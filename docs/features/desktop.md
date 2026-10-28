# Terminal UI, tray and desktop lifecycle

The terminal UI and tray are clients of the daemon. They never own a second engine,
write the database or stop the daemon merely by closing.

## Keyboard navigation

`1`–`7`, Tab/Shift+Tab, `[`/`]` and tab-bar clicks select the same top-level tabs.
On a list, `↑↓` selects its rows and Enter opens a detail view. Top-level horizontal
arrows switch tabs where the page does not own those arrows.

Inside a collection, `↑↓` moves focus through **Copy**, **Sync**, **Output**,
**Filter**, **Integrity check** and **Protection** as available. It keeps the same
collection open. Select Sync and press Enter/Right for a vertical schedule/action
menu; arrows choose, Enter applies, Left/Esc closes the menu. Select Output to open
its URL, Filter for the editor, Integrity for findings/start scan or Protection for
the [mirror/snapshot screen](protection.md). Existing letter shortcuts remain.

Left/Esc returns from a job, collection, report or audit to its parent. Left also
cancels a non-typing copy dialog or state picker. In text fields, arrows move the
text cursor; Esc cancels the field/dialog. The Drive browser uses Left for its
parent folder. Up/Down in job details scroll rather than select another job.

A live refresh preserves detail identity across list reordering. If the open
object disappears, return to the parent list rather than target its replacement.
`q` exits the TUI; the detached daemon keeps running.

## Readiness, progress and editors

Home shows setup checks until Doctor reports database, OAuth client, account and
default destination ready. Deep preflight is driven by SSE while connected and
falls back to polling. Partial discovery never claims a total.

Rate/ETA uses a shared observation window and enough samples to establish a trend.
Unknown totals remain unknown; stalled/unstable estimates are hidden. Queue hints
are snapshots and may change.

Collection shortcuts:

- `A`: paginated audit browser; `t` filters, `n/p` navigate pages, `f` previews
  repair, `F` requests confirmation after a successful preview.
- `B`: full filter editor. Ctrl+P previews top-level counts; Ctrl+S saves with
  collection revision. Include/exclude accept semicolon-separated rules.
- `b`: filter presets; lifecycle and sync letter shortcuts remain in the on-screen help.

Settings ends with **Clear history…**: pick a range (older than 7, 30 or 90 days, or every
finished job), read the preview, confirm. `x` on the Jobs screen clears the selected
finished jobs after one confirmation. Neither touches your copies or Drive.

Protection offers mirror on/off, automatic snapshots and fixed retention choices,
capture now, pin/unpin and restore. Enabling mirror asks first and names the Drive
trash consequence. Restore always runs a read-only preview first and shows the
action count; apply is a second, separate confirmation that remarks sync pauses.
See the [protection guide](protection.md).

## Tray

The tray is a separate HTTP client on Linux/Windows. It offers status, Open 502Drive,
clipboard copy, recent jobs, latest result, queue pause/resume and optional startup.
Clipboard content is read only on click, parsed as one Drive reference, and never
shell-executed. Recent-job actions open the specific job in a terminal with fixed
arguments and the same config/endpoint/token settings.

**Quit tray** normally closes only the tray. **Stop 502Drive…** calls
`POST /v1/shutdown`; running jobs require a second confirmation within ten seconds.
The optional `tray_quit_stops_daemon` preference changes tray exit behavior.
Windows does not terminate the daemon by process name.

Seven icon states represent offline, error, needs-attention, reconnecting, paused,
running and idle. Motion can be disabled. Polling is faster while busy and backs
off while offline. Linux requires D-Bus and a tray host; when absent, the launcher
can run without a tray and Doctor explains why. Explorer restarts are handled by
the Windows backend. macOS has CLI/daemon support, not a native tray.

Desktop notifications currently target Linux. They respect preferences and quiet
hours and coalesce repeated causes. A supported Open action captures a fixed job ID;
actual desktop clicks and Windows native delivery still need acceptance.

Update checks stay opt-in and run at most daily. When one finds a newer release,
the tray shows `Update to vX…` and the tooltip adds `New release available: vX`.
The row is hidden otherwise. A second click inside ten seconds confirms; it opens
a terminal running `502drive update --install --restart --yes` with the running
binary's path and fixed arguments. The tray never installs software itself, and a
package-managed install (rpm/deb, Windows installer) reports `update_package_managed`
in that terminal so the package manager keeps ownership. **Settings › Check for
updates daily** turns the check on or off and saves `check_updates`.

## Startup and shutdown

Daemon and tray use separate process-held locks. Detached daemon startup belongs
to `up` / the launcher; a second writer is refused. Tray exit/restart does not own
daemon lifetime. A systemd user unit provides restart-on-failure with a bounded
restart policy; the tray is not a watchdog.

Choose desktop autostart (`autostart enable`) or systemd user startup
(`autostart systemd install`). Do not rely on closing terminals for graceful stop;
use `502drive stop`. Foreground `serve` follows its own process lifetime.

## Sleep and network recovery

Lifecycle states include running, suspending, sleeping, resuming and reconnect-required.
Before sleep, admission is suspended and WAL checkpointed. After wake, the daemon
checks state, resets idle connections and waits for connectivity before new work.
Wall/monotonic clock gaps provide portable sleep detection; OS hooks add precision.
Network failure does not spend item attempts. Unknown writes still reconcile.

Keep-awake is opt-in and may increase battery/thermal load. Native laptop acceptance
is separate from logical tests. See the [runbook](../runbooks.md#native-laptop-checks).

## Packaging and evidence

RPM/DEB include the user unit, desktop entry, icons and completions. Windows ZIP
includes CLI/tray binaries; Inno Setup offers optional PATH/startup/desktop tasks.
The installer is unsigned. Distroless containers run the same daemon without a desktop.

Sources: [TUI](../../internal/tui), [tray](../../internal/tray),
[packaging](../../packaging), [release workflow](../../.github/workflows/go-release.yml).
See [verification](../verification.md) for recorded evidence and platform gaps.
