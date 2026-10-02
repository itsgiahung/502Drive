#!/usr/bin/env bash
# install-systemd-user.sh — install gdclone-bot as a systemd user service.
# Re-run to upgrade the binary in place.
#
# Usage:
#   bash scripts/install-systemd-user.sh
#
# Environment overrides:
#   BINARY_PATH   — path to install binary (default: ~/.local/bin/gdclone-bot)
#   CONFIG_PATH   — path to config.toml (default: ~/.config/gdclone-bot/config.toml)

set -euo pipefail
BOLD='\033[1m'; GREEN='\033[32m'; RED='\033[31m'; RESET='\033[0m'
say()  { printf "${BOLD}==> %s${RESET}\n" "$*"; }
ok()   { printf "${GREEN}    ok:${RESET} %s\n" "$*"; }
fail() { printf "${RED}    ERROR:${RESET} %s\n" "$*" >&2; exit 1; }

SERVICE_NAME="gdclone-bot"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/gdclone-bot"
BINARY_PATH="${BINARY_PATH:-$HOME/.local/bin/gdclone-bot}"
CONFIG_PATH="${CONFIG_PATH:-$CONFIG_DIR/config.toml}"

# ── 1. Binary ────────────────────────────────────────────────────────────────
say "Checking binary"
SOURCE_BINARY=""
if [[ -f "target/release/502drive" ]]; then
    SOURCE_BINARY="target/release/502drive"
elif [[ -f "target/release/$SERVICE_NAME" ]]; then
    SOURCE_BINARY="target/release/$SERVICE_NAME"
fi

if [[ -n "$SOURCE_BINARY" ]]; then
    say "Installing release binary → $BINARY_PATH"
    mkdir -p "$(dirname "$BINARY_PATH")"
    install -m 755 "$SOURCE_BINARY" "$BINARY_PATH"
    ok "$BINARY_PATH installed"
elif [[ -f "$BINARY_PATH" ]]; then
    ok "Using existing binary at $BINARY_PATH"
else
    fail "Binary not found. Run: cargo build --release --bin 502drive"
fi

# ── 2. Config ────────────────────────────────────────────────────────────────
say "Checking config"
if [[ ! -f "$CONFIG_PATH" ]]; then
    if [[ -f "config.sample.toml" ]]; then
        say "Copying sample config → $CONFIG_PATH"
        mkdir -p "$CONFIG_DIR"
        install -m 600 config.sample.toml "$CONFIG_PATH"
        ok "Edit $CONFIG_PATH and set bot_token and owner_telegram_id"
    else
        fail "Config not found at $CONFIG_PATH — create it first."
    fi
else
    ok "Config found at $CONFIG_PATH"
    # Warn if permissions are too open.
    perm=$(stat -c '%a' "$CONFIG_PATH" 2>/dev/null || stat -f '%Lp' "$CONFIG_PATH" 2>/dev/null || echo "???")
    if [[ "$perm" != "600" && "$perm" != "400" ]]; then
        printf "    \033[33mWARN:\033[0m config.toml permissions are %s — consider chmod 600\n" "$perm"
    fi
fi

# ── 3. Private config directory ──────────────────────────────────────────────
mkdir -p "$CONFIG_DIR"
chmod 700 "$CONFIG_DIR"
chmod 600 "$CONFIG_PATH"
ok "Config permissions ready"

# ── 4. Generate unit from effective config ──────────────────────────────────
# Delegate to the Rust platform implementation so service hardening and
# ReadWritePaths stay in sync with custom storage.db_path/log_dir/report_dir.
say "Generating hardened systemd unit from effective config"
"$BINARY_PATH" --config "$CONFIG_PATH" service-install
ok "Unit file generated"

# ── 5. Stop any foreground process ─────────────────────────────────────────
if pgrep -u "$USER" -f "$SERVICE_NAME .* run" > /dev/null 2>&1; then
    say "Stopping foreground gdclone-bot process"
    pkill -u "$USER" -f "$SERVICE_NAME .* run" || true
    sleep 1
fi

# ── 6. Enable lingering so service survives logout ─────────────────────────
say "Enabling user lingering (loginctl)"
loginctl enable-linger "$USER" 2>/dev/null || true

# ── 7. Enable and start ────────────────────────────────────────────────────
say "Reloading systemd user daemon"
systemctl --user daemon-reload

say "Enabling $SERVICE_NAME"
systemctl --user enable "$SERVICE_NAME.service"

say "Starting $SERVICE_NAME"
if systemctl --user start "$SERVICE_NAME.service"; then
    ok "Service started"
else
    printf "    \033[33mWARN:\033[0m start may have failed — check status below\n"
fi

echo ""
say "Status:"
systemctl --user status "$SERVICE_NAME.service" --no-pager -l || true

echo ""
say "Useful commands:"
echo "  journalctl --user -u $SERVICE_NAME -f          # follow logs"
echo "  systemctl --user status $SERVICE_NAME          # status"
echo "  systemctl --user restart $SERVICE_NAME         # restart"
echo "  systemctl --user stop $SERVICE_NAME            # stop"
echo "  $BINARY_PATH auth login                        # Google auth"
