# T3 — Consistent optional autostart

Use `internal/autostart` for CLI/tray startup settings rather than duplicate OS
logic. Status reads the actual user setting; enable/disable are idempotent.
No administrator privilege is needed for per-user startup.

- Linux: XDG autostart invokes the fixed executable with `up --quiet`.
- Windows: the current user's Run key invokes the same detached-start path.
  Inno Setup uses the matching key/value behavior.
- macOS: this manager has no native autostart implementation.

Quiet startup never opens a terminal or asks for credentials. Directories and
configuration use private permissions. Missing/corrupt entries produce useful
status/errors rather than implying a successful enabled state.

Systemd user startup is a separate daemon-only option with restart policy; choose
one startup mechanism. See [Linux installation](../install-linux.md),
[Windows installation](../install-windows.md).
