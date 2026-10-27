# T2 — Client lifetime and single instance

Three distinct actions have explicit meanings:

| Action | Effect |
| --- | --- |
| Close TUI | Client exits; daemon remains |
| Quit tray | Tray exits; daemon normally remains |
| Stop 502Drive | Explicit graceful HTTP shutdown |

`tray_quit_stops_daemon` is an off-by-default compatibility preference. CLI stop
with active work prompts on a TTY and requires `--yes` in scripts. Tray stop uses
a second click within ten seconds when work remains. Stop waits boundedly for the
daemon to exit and reports failure rather than killing processes by name.

Daemon/tray locks are process-held and independent. Startup checks liveness/locks
before spawning. Tray polling backs off offline; it does not supervise/restart the
daemon repeatedly. A bounded systemd user-unit policy handles optional crash restart.

Linux without StatusNotifierWatcher can continue without tray and report an
actionable Doctor result. The native library re-registers when the host returns;
Windows re-adds its icon after Explorer restart. These hooks do not replace native
acceptance of actual desktop/session behavior.

See [desktop](../features/desktop.md), [tray tests](../../internal/tray/tray_test.go),
[CLI lifecycle](../../internal/cli).
