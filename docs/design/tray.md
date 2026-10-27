# T4 — Tray presentation and actions

Retain the existing systray backend: Linux/Windows already support the needed
status, menu and tooltip behavior without a new dependency. A tray is an HTTP
client, not a watchdog or a second daemon.

Keep stable menu/action IDs and update labels/enabled state only when changed.
Offer status, explicit daemon start/stop, Open 502Drive, clipboard copy, recent jobs,
latest result, queue pause/resume, optional startup and separate Quit tray.
The black logo remains; state overlays distinguish offline, error, needs-attention,
reconnecting, paused, running and idle. Busy motion is bounded and optional.

Read clipboard only after a user action, parse exactly one Drive reference and
never construct shell commands from clipboard/name text. Recent-job actions capture
IDs with fixed arguments. Stop uses HTTP and confirmation when work remains.

Tooltip/menu text is bounded for platform limits, uses real state/counts and offers
a recovery action. Linux clicks depend on the tray host; all functions remain in
the menu. Windows double-click can open the terminal. Missing native events are
not simulated as universally supported behavior.

Poll faster while active and back off offline. Suppress duplicate native updates
by view signatures. Idle CPU, actual desktop notification clicks and host/session
restart behavior require native acceptance. See [desktop](../features/desktop.md),
[verification](../verification.md).
