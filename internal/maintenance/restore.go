package maintenance

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// tokenFormat and token AAD mirror internal/drive/auth.go. They are duplicated
// on purpose: internal/drive is not open for changes, and the check here is
// read-only verification, not decryption of a live credential.
const tokenFormat = "502drive-token-v1"

// RestoreOptions points at a backup directory and a new, empty data directory.
type RestoreOptions struct {
	Backup string
	Target string
}

type RestoreReport struct {
	Manifest      Manifest
	Target        string
	SchemaVersion int
	ConfigWritten bool
}

// Restore validates a backup set and only then renames a staging directory
// into place. Any failure removes the staging directory, so a half-restored
// data directory never appears.
func Restore(ctx context.Context, o RestoreOptions) (RestoreReport, error) {
	var report RestoreReport
	if o.Backup == "" || o.Target == "" {
		return report, errors.New("a backup path and a target data directory are required")
	}
	backup, err := filepath.Abs(o.Backup)
	if err != nil {
		return report, err
	}
	target, err := filepath.Abs(o.Target)
	if err != nil {
		return report, err
	}
	if inside(target, backup) || inside(backup, target) {
		return report, fmt.Errorf("%w: the backup and the target must be separate directories", ErrUnsafePath)
	}
	m, err := ReadManifest(backup)
	if err != nil {
		return report, err
	}
	if m.SchemaVersion > maxSchema() {
		return report, fmt.Errorf("%w: backup schema %d, binary schema %d", ErrSchema, m.SchemaVersion, maxSchema())
	}
	if err = requireEmptyTarget(target); err != nil {
		return report, err
	}

	parent := filepath.Dir(target)
	staging, err := os.MkdirTemp(parent, ".restore-"+filepath.Base(target)+"-*")
	if err != nil {
		return report, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(staging)
		}
	}()

	for _, f := range m.Files {
		if !safeRel(f.Path) {
			return report, fmt.Errorf("%w: %s", ErrUnsafePath, f.Path)
		}
		if err := verifyAndCopy(filepath.Join(backup, f.Path), filepath.Join(staging, f.Path), f); err != nil {
			return report, err
		}
	}
	if err = verifyDatabase(ctx, filepath.Join(staging, DatabaseName), m.SchemaVersion); err != nil {
		return report, err
	}
	if err = verifySecrets(staging, m); err != nil {
		return report, err
	}
	if m.Files != nil {
		if err = rewriteConfig(filepath.Join(staging, ConfigName), target); err != nil {
			return report, err
		}
		_, statErr := os.Lstat(filepath.Join(staging, ConfigName))
		report.ConfigWritten = statErr == nil
	}
	if err = os.Chmod(staging, 0o700); err != nil {
		return report, err
	}
	// The target is empty (or absent); replace it with the validated snapshot.
	if err = os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	if err = os.Rename(staging, target); err != nil {
		return report, err
	}
	ok = true
	report.Manifest, report.Target, report.SchemaVersion = m, target, m.SchemaVersion
	return report, nil
}

// requireEmptyTarget refuses a directory that holds anything: the live data
// directory, a previous restore or an unrelated folder.
func requireEmptyTarget(target string) error {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", ErrTargetExists, target)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("%w: %s is not empty; restore into a new directory", ErrTargetExists, target)
	}
	return nil
}

// safeRel rejects absolute paths and any path that escapes the directory.
func safeRel(p string) bool {
	if p == "" || filepath.IsAbs(p) {
		return false
	}
	clean := filepath.Clean(p)
	return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func verifyAndCopy(src, dst string, want FileEntry) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("%w: %s is missing", ErrManifest, want.Path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, want.Path)
	}
	sum, size, err := fileSHA256(src)
	if err != nil {
		return err
	}
	if size != want.Size || sum != want.SHA256 {
		return fmt.Errorf("%w: %s does not match the manifest", ErrIntegrity, want.Path)
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return copyFile(src, dst)
}

// verifyDatabase runs the SQLite checks on the restored copy before it is
// allowed to become the live database.
func verifyDatabase(ctx context.Context, path string, want int) error {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("mode", "ro")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var check string
	if err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
		return fmt.Errorf("%w: integrity_check said %q", ErrIntegrity, check)
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("%w: foreign_key_check reported a violation", ErrIntegrity)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	var version, appID int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > want || version > maxSchema() {
		return fmt.Errorf("%w: database schema %d", ErrSchema, version)
	}
	if err = db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID); err != nil {
		return err
	}
	if appID != 0 && appID != stateApplicationID {
		return fmt.Errorf("%w: application_id %d", ErrSchema, appID)
	}
	var tables int
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='jobs'").Scan(&tables); err != nil {
		return err
	}
	if tables != 1 {
		return fmt.Errorf("%w: the database has no jobs table", ErrIntegrity)
	}
	return nil
}

// stateApplicationID marks a 502Drive v2 database (see internal/state).
const stateApplicationID = 502002

// verifySecrets proves the backed-up key can decrypt the backed-up token. A
// backup that carries a token without a key is unusable for a restore.
func verifySecrets(dir string, m Manifest) error {
	keyPath := filepath.Join(dir, SecretsDir, KeyName)
	tokenPath := filepath.Join(dir, SecretsDir, TokenName)
	token, tokenErr := os.Lstat(tokenPath)
	key, keyErr := os.Lstat(keyPath)
	switch {
	case errors.Is(tokenErr, os.ErrNotExist):
		return nil // nothing was ever logged in
	case tokenErr != nil:
		return tokenErr
	case errors.Is(keyErr, os.ErrNotExist):
		return fmt.Errorf("%w: the backup has a token but no master.key", ErrSecret)
	case keyErr != nil:
		return keyErr
	}
	if !key.Mode().IsRegular() || !token.Mode().IsRegular() {
		return fmt.Errorf("%w: secrets must be regular files", ErrUnsafePath)
	}
	rawKey, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	secret, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(rawKey)))
	if err != nil || len(secret) != 32 {
		return fmt.Errorf("%w: master.key is malformed", ErrSecret)
	}
	rawToken, err := os.ReadFile(tokenPath)
	if err != nil {
		return err
	}
	var env struct {
		Format string `json:"format"`
		Nonce  []byte `json:"nonce"`
		Data   []byte `json:"data"`
	}
	if json.Unmarshal(rawToken, &env) != nil || env.Format != tokenFormat {
		return fmt.Errorf("%w: the saved token has an unsupported format", ErrSecret)
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(env.Nonce) != gcm.NonceSize() {
		return fmt.Errorf("%w: the token nonce is malformed", ErrSecret)
	}
	if _, err = gcm.Open(nil, env.Nonce, env.Data, []byte(tokenFormat)); err != nil {
		return fmt.Errorf("%w: master.key does not decrypt the token", ErrSecret)
	}
	return nil
}

// rewriteConfig points the restored config at the restored data directory. A
// backup without a config file is still restorable by hand.
func rewriteConfig(path, target string) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("%w: the backed-up config is not JSON", ErrManifest)
	}
	cfg["data_dir"] = target
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil {
		return err
	}
	return writeFile(path, append(b, '\n'))
}
