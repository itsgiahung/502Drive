#!/usr/bin/env bash
# backup.sh — create a portable 502Drive backup archive from a consistent
# SQLite snapshot produced by the canonical CLI backup engine.
#
# Usage: ./scripts/backup.sh [output_dir]
#
# Optional:
#   DRIVE502_BIN         — path to the 502drive binary
#   GDCLONE_CONFIG_PATH  — config path (defaults to native user config)

set -euo pipefail
umask 077

BOLD='\033[1m'; RED='\033[31m'; RESET='\033[0m'
say() { printf "${BOLD}==> %s${RESET}\n" "$*"; }
die() { printf "${RED}ERROR: %s${RESET}\n" "$*" >&2; exit 1; }

BACKUP_DIR="${1:-${BACKUP_DIR:-$PWD}}"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/gdclone-bot"
CONFIG_PATH="${GDCLONE_CONFIG_PATH:-$CONFIG_DIR/config.toml}"

resolve_binary() {
  if [[ -n "${DRIVE502_BIN:-}" ]]; then
    [[ -x "$DRIVE502_BIN" ]] || die "DRIVE502_BIN is not executable: $DRIVE502_BIN"
    printf '%s\n' "$DRIVE502_BIN"
    return
  fi
  if command -v 502drive >/dev/null 2>&1; then
    command -v 502drive
    return
  fi
  if [[ -x "target/release/502drive" ]]; then
    printf '%s\n' "$PWD/target/release/502drive"
    return
  fi
  die "502drive binary not found. Install it or set DRIVE502_BIN."
}

BIN="$(resolve_binary)"
[[ -f "$CONFIG_PATH" ]] || die "Config not found: $CONFIG_PATH"

mkdir -p "$BACKUP_DIR"
TIMESTAMP="$(date +%Y%m%dT%H%M%S)"
ARCHIVE="$BACKUP_DIR/gdclone-bot-backup-$TIMESTAMP.tar.gz"
TMP_ARCHIVE="$ARCHIVE.tmp.$$"
SNAPSHOT_DIR="$(mktemp -d "$BACKUP_DIR/.502drive-snapshot-$TIMESTAMP.XXXXXX")"

cleanup() {
  rm -rf "$SNAPSHOT_DIR"
  rm -f "$TMP_ARCHIVE"
}
trap cleanup EXIT INT TERM

say "Creating consistent snapshot"
"$BIN" --config "$CONFIG_PATH" backup "$SNAPSHOT_DIR"

FILES=()
for name in config.toml state.db master.key RESTORE.vi.txt; do
  [[ -f "$SNAPSHOT_DIR/$name" ]] && FILES+=("$name")
done
[[ ${#FILES[@]} -gt 0 ]] || die "Backup engine produced no files"

say "Creating archive: $ARCHIVE"
tar -C "$SNAPSHOT_DIR" -czf "$TMP_ARCHIVE" "${FILES[@]}"
chmod 600 "$TMP_ARCHIVE"
mv "$TMP_ARCHIVE" "$ARCHIVE"

BYTES="$(stat -c%s "$ARCHIVE" 2>/dev/null || stat -f%z "$ARCHIVE")"
say "Backup complete: $ARCHIVE ($BYTES bytes)"
say "Restore with: scripts/restore.sh $ARCHIVE"
