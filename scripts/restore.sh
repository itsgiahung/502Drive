#!/usr/bin/env bash
# restore.sh — safely restore a backup archive produced by scripts/backup.sh.
#
# Usage: ./scripts/restore.sh <backup.tar.gz>
#
# Optional target overrides:
#   GDCLONE_RESTORE_CONFIG_PATH  — defaults to ~/.config/gdclone-bot/config.toml
#   GDCLONE_RESTORE_DB_PATH      — defaults to ~/.local/share/gdclone-bot/state.db
#
# The service is stopped before publishing restored files and is deliberately
# left stopped so the user can run `502drive doctor` before restarting it.

set -euo pipefail
umask 077

BOLD='\033[1m'; RED='\033[31m'; YELLOW='\033[33m'; RESET='\033[0m'
say()  { printf "${BOLD}==> %s${RESET}\n" "$*"; }
warn() { printf "${YELLOW}WARN: %s${RESET}\n" "$*" >&2; }
die()  { printf "${RED}ERROR: %s${RESET}\n" "$*" >&2; exit 1; }

ARCHIVE="${1:-}"
[[ -n "$ARCHIVE" ]] || die "Usage: $0 <backup.tar.gz>"
[[ -f "$ARCHIVE" ]] || die "Archive not found: $ARCHIVE"

CONFIG_PATH="${GDCLONE_RESTORE_CONFIG_PATH:-${XDG_CONFIG_HOME:-$HOME/.config}/gdclone-bot/config.toml}"
DB_PATH="${GDCLONE_RESTORE_DB_PATH:-${XDG_DATA_HOME:-$HOME/.local/share}/gdclone-bot/state.db}"
KEY_PATH="$(dirname "$CONFIG_PATH")/master.key"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/502drive-restore.XXXXXX")"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

say "Validating archive structure"
mapfile -t MEMBERS < <(tar -tzf "$ARCHIVE") || die "Cannot read archive"
[[ ${#MEMBERS[@]} -gt 0 ]] || die "Archive is empty"

for member in "${MEMBERS[@]}"; do
  case "$member" in
    config.toml|state.db|master.key|RESTORE.vi.txt) ;;
    *) die "Unexpected archive entry: $member" ;;
  esac
done

# Only regular files are allowed. This rejects symlinks/hardlinks/devices before
# extraction so a crafted archive cannot redirect a later restore copy.
while IFS= read -r line; do
  [[ "${line:0:1}" == "-" ]] || die "Archive contains a non-regular entry"
done < <(tar -tvzf "$ARCHIVE")

tar --no-same-owner --no-same-permissions -xzf "$ARCHIVE" -C "$TMP_DIR"

for name in config.toml state.db master.key RESTORE.vi.txt; do
  if [[ -e "$TMP_DIR/$name" ]]; then
    [[ -f "$TMP_DIR/$name" && ! -L "$TMP_DIR/$name" ]]       || die "Restored entry is not a regular file: $name"
  fi
done

[[ -f "$TMP_DIR/config.toml" ]] || die "Archive is missing config.toml"

if [[ -f "$TMP_DIR/state.db" ]]; then
  if command -v sqlite3 >/dev/null 2>&1; then
    QUICK_CHECK="$(sqlite3 -readonly "$TMP_DIR/state.db" 'PRAGMA quick_check;' 2>/dev/null)"       || die "Backup state.db cannot be opened by SQLite"
    [[ "$QUICK_CHECK" == "ok" ]] || die "Backup state.db failed SQLite quick_check: $QUICK_CHECK"
  else
    warn "sqlite3 CLI not found; skipping pre-restore SQLite quick_check"
  fi
fi

SERVICE_WAS_ACTIVE=0
if command -v systemctl >/dev/null 2>&1   && systemctl --user is-active --quiet gdclone-bot.service 2>/dev/null; then
  SERVICE_WAS_ACTIVE=1
  say "Stopping gdclone-bot service"
  systemctl --user stop gdclone-bot.service     || die "Could not stop gdclone-bot.service; restore aborted"
fi

atomic_install() {
  local source="$1"
  local destination="$2"
  local parent temp

  parent="$(dirname "$destination")"
  mkdir -p "$parent"
  temp="$destination.restore.$$"
  rm -f "$temp"
  install -m 600 "$source" "$temp"
  mv -f "$temp" "$destination"
}

say "Restoring private config"
atomic_install "$TMP_DIR/config.toml" "$CONFIG_PATH"

if [[ -f "$TMP_DIR/master.key" ]]; then
  atomic_install "$TMP_DIR/master.key" "$KEY_PATH"
fi

if [[ -f "$TMP_DIR/state.db" ]]; then
  say "Restoring SQLite state"
  atomic_install "$TMP_DIR/state.db" "$DB_PATH"
  rm -f "$DB_PATH-wal" "$DB_PATH-shm"
fi

chmod 700 "$(dirname "$CONFIG_PATH")"
chmod 600 "$CONFIG_PATH"
[[ -f "$KEY_PATH" ]] && chmod 600 "$KEY_PATH"
[[ -f "$DB_PATH" ]] && chmod 600 "$DB_PATH"

say "Restore complete; service remains stopped for verification."
echo "  502drive --config \"$CONFIG_PATH\" doctor"
echo "  systemctl --user start gdclone-bot.service"
if [[ "$SERVICE_WAS_ACTIVE" -eq 1 ]]; then
  say "The service was running before restore. Restart it only after doctor passes."
fi
