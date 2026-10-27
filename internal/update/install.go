package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Installing is only ever started by the user (`502drive update --install`).
// The archive comes from the project's GitHub release over HTTPS and must
// match the release's checksums.txt before any file on disk is touched.

// maxArchive bounds a download; a release archive is about 15 MB.
const maxArchive = 200 << 20

// Asset is one release archive and its expected SHA-256.
type Asset struct {
	Name   string
	SHA256 string
}

// ReleaseBase is where a tag's files are downloaded from ("" = GitHub).
func ReleaseBase(base, tag string) string {
	if base != "" {
		return strings.TrimRight(base, "/")
	}
	return "https://github.com/" + Repo + "/releases/download/" + tag
}

// PickAsset finds the archive for goos/goarch in a checksums.txt body.
func PickAsset(checksums []byte, goos, goarch string) (Asset, error) {
	suffix := "_" + goos + "_" + goarch + ".tar.gz"
	if goos == "windows" {
		suffix = "_" + goos + "_" + goarch + ".zip"
	}
	sc := bufio.NewScanner(bytes.NewReader(checksums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 || !strings.HasSuffix(f[1], suffix) || strings.ContainsAny(f[1], `/\`) {
			continue
		}
		if len(f[0]) != 64 {
			continue
		}
		if _, err := hex.DecodeString(f[0]); err != nil {
			continue
		}
		return Asset{Name: f[1], SHA256: strings.ToLower(f[0])}, nil
	}
	return Asset{}, fmt.Errorf("this release has no %s/%s archive", goos, goarch)
}

// Fetch downloads url into memory, at most limit bytes.
func Fetch(ctx context.Context, hc *http.Client, url string, limit int64) ([]byte, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "502drive-update")
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot download %s: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s answered HTTP %d", url, res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("the download is larger than expected; nothing was installed")
	}
	return b, nil
}

// Verify checks data against the expected SHA-256 hex digest.
func Verify(data []byte, want string) error {
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != strings.ToLower(want) {
		return fmt.Errorf("checksum mismatch (expected %s, got %s); nothing was installed", want, got)
	}
	return nil
}

// Extract returns the named files from a release archive (tar.gz, or zip on
// Windows). Only top-level entries with exactly those names are read; nothing
// is written to disk here, so a hostile path in the archive has no effect.
func Extract(archive []byte, zipped bool, names ...string) (map[string][]byte, error) {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	out := map[string][]byte{}
	take := func(name string, r io.Reader) error {
		if !want[name] {
			return nil
		}
		b, err := io.ReadAll(io.LimitReader(r, maxArchive))
		if err != nil {
			return err
		}
		out[name] = b
		return nil
	}
	if zipped {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = take(f.Name, rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return nil, err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if h.Typeflag != tar.TypeReg {
				continue
			}
			if err = take(h.Name, tr); err != nil {
				return nil, err
			}
		}
	}
	for _, n := range names {
		if len(out[n]) == 0 {
			return nil, fmt.Errorf("the archive does not contain %s; nothing was installed", n)
		}
	}
	return out, nil
}

// Replace swaps the file at target for data. The new file is written next to
// it first; the current one is kept as target+".prev" for a manual rollback.
// Renaming a running executable works on Linux, macOS and Windows.
func Replace(target string, data []byte) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".502drive-update-*")
	if err != nil {
		return fmt.Errorf("cannot write next to %s: %w", target, err)
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	prev := target + ".prev"
	_ = os.Remove(prev)
	if err = os.Rename(target, prev); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot move the current binary aside: %w", err)
	}
	if err = os.Rename(tmp.Name(), target); err != nil {
		_ = os.Rename(prev, target) // put the working binary back
		return fmt.Errorf("cannot install the new binary: %w", err)
	}
	return nil
}

// Writable reports whether the current user can replace files in dir.
func Writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".502drive-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}
