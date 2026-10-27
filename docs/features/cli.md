# Command-line reference

Operational commands are HTTP clients. Only offline maintenance opens the database,
with the writer lock enforced. The terminal UI/tray share these contracts.
Use `502drive COMMAND --help` for the complete flag set.

## Setup and daemon

```text
502drive setup [--oauth-client FILE] [--env-file FILE] [--no-env]
               [--telegram-token-file FILE] [--telegram-owner-id N]
               [--oauth-callback-port N]
502drive onboard [--no-input]
502drive up
502drive serve [--maintenance]
502drive stop [--yes]
502drive status
502drive account status|login|revoke
502drive doctor [--online] [--fix]
502drive logs [-n N] [-f] [--tray]
502drive version
```

Bare `502drive` launches the interactive experience on a TTY. `up` starts the
background service without opening a UI. `serve` is foreground. Closing a TUI/tray
normally keeps a detached daemon alive; stop is explicit. Stop with running jobs
prompts in a TTY and requires `--yes` without one.

Setup precedence is flags, environment, explicit/current-directory `.env`, then
configuration-directory `.env`. Values are validated before private files are
created; source credential files are not modified or printed. Interactive setup
may ask for missing values. Autostart/quiet and script modes do not prompt.
See [OAuth setup](../oauth-setup.md).

`onboard` guides an interactive terminal through prerequisites. With `--no-input`,
`--json` or no TTY it reads no input and starts no processes: it emits a readiness
checklist, with exit 9 when incomplete. Older servers fall back to Doctor.

## Copy and inspect

```text
502drive inspect SOURCE [--to ID] [--name NAME] [--deep] [FILTER]
502drive copy [SOURCE...|--stdin] [--to ID] [--name NAME|--auto-name]
              [--wait] [--wait-timeout D] [--idempotency-key K]
              [--shortcuts follow|skip] [--on-conflict rename|skip|update]
              [--update|--new] [--sync off|realtime|hourly|six_hourly|daily] [FILTER]
502drive LINK [LINK...] [flags]
502drive destinations list|add|default
```

Bare links invoke copy. Repeated sources/standard-input lines form a batch with
one job per unique nonblank link; one invalid link does not undo other admissions.
Batch output reports accepted/failed items, and exits with the first relevant error.
Explicit idempotency keys allow safe replay. Scripts default existing-copy handling
to update; interactive single-copy flows can choose update/new/cancel.

`--auto-name` uses the source name plus a copy date and conflicts with `--name`.
Inspect is read-only. Deep inspect waits for background discovery; interrupted or
cancelled results never claim a total. Stopping the local wait does not delete output
or cancel the admitted job/scan.

Filters are shared by inspect, copy and sync:

- Repeated `--include` / `--exclude`; exclude wins. Path/name globs support `**`.
- `--ext pdf,docx`, `ext:.pdf`, `class:doc`; classes include image, video, audio,
  doc, archive, native and other.
- `--min-size`, `--max-size`, `--modified-after`, `--keep-empty-folders`.
- `--filter-file FILE` accepts documented rules with comment lines beginning `#`.

A source that is one file is not filtered. See [copy behavior](clone-jobs.md).

## Jobs, queue and reports

```text
502drive jobs list [--state queued,blocked,failed]
502drive jobs get|wait|events|pause|resume|cancel|retry ID
502drive wait ID
502drive watch [ID] [--interval 1s]
502drive jobs move ID [--before ID|--after ID] [--priority N]
502drive pause
502drive resume
502drive jobs report ID [--format table|json|csv] [--max-rows N]
502drive history export [--since 30d] [--format csv|json|table] [--all] [--max-rows N]
```

Job references accept full IDs, unambiguous prefixes, `@last` and `@running`.
State filters accept comma-separated sets; invalid/empty members fail validation.
Moving requires queued state and priority -5..5. Global pause is durable; resume
keeps individual user pauses. Queue hints are estimates, not promised start times.

Reports page through problems and cap output at 5,000 rows by default. Human tables
show state, stable code, explanation, relative path and attempts. CSV adds source/
destination IDs and escapes spreadsheet formula prefixes. JSON is one envelope;
combining `--json` with CSV is a usage error. Truncation is reported on stderr.
History defaults to CSV over 30 days, ordered by update time; relative durations,
RFC3339 and calendar dates are accepted for `--since`.

## Collections, sync and audit

```text
502drive collections list|get
502drive collections run|pause|audit COLLECTION
502drive collections retire|resume COLLECTION
502drive collections forget COLLECTION --yes
502drive collections cleanup COLLECTION [--dry-run=false --yes --confirm-name NAME]
502drive collections open COLLECTION [--dest]
502drive sync enable COLLECTION --daily 22:00 [--timezone IANA] [--dry-run]
502drive sync enable COLLECTION --weekly mon,wed@07:30 [--timezone IANA]
502drive sync enable COLLECTION --every 30m
502drive sync enable COLLECTION --mode realtime
502drive sync status|run|pause|resume|stop COLLECTION
502drive sync filter show COLLECTION
502drive sync filter set COLLECTION [FILTER] [--clear]
502drive audit COLLECTION [--repair [--apply|--dry-run]] [--wait=false] [--wait-timeout 10m]
502drive audit show AUDIT_ID [--kind KIND] [--all]
```

`sync enable` also accepts `--policy versioned_copy|replace_copy`; dry-run previews
three future slots. Explicit timezone overrides the saved preference. A stale sync
revision can be refreshed once by the CLI.

Retire/resume/forget affect local tracking, not file contents. Forget requires
retirement. Collection cleanup previews by default and requires the **actual Drive
root name** to apply; it only trashes owned output. Offline temporary-file cleanup
is a separate command. Audit is read-only; `--repair` previews unless `--apply`.
`audit --wait` is a boolean switch; use `--wait-timeout 5m`, not `--wait 5m`.
Each new audit command requests a fresh report. Copy/repair cancellation retains
completed output.

## Protection options

```text
502drive sync options COLLECTION [--mirror=true|false] [--snapshots=true|false]
                                [--keep-days 30] [--keep-count 10] [--dry-run]
502drive snapshots list|create COLLECTION
502drive snapshots show|pin|unpin SNAPSHOT
502drive snapshots restore SNAPSHOT [--apply|--dry-run] [--wait=false] [--wait-timeout D]
502drive snapshots plan JOB
```

List/plan accept `--cursor`. Unspecified options preserve current values. Mirror
and automatic capture default off; restore defaults to preview and apply pauses
sync. See [protection](protection.md) for storage and recovery limits.

## Drive browsing

```text
502drive ls [FOLDER] [--all]
502drive info LINK_OR_ID
502drive mkdir NAME [--in PARENT] [--idempotency-key K]
```

`root` means My Drive. Listings show folders first, sort names within each group
and include size/modified time. `--all` merges pages, bounded at 200 pages; JSON
returns one combined envelope. Resource keys are passed internally, not printed.
Mkdir returns its ID/link; quiet mode prints the ID.

## Terminal, tray and maintenance

```text
502drive menu
502drive tui --job ID
502drive tray [--write-icons DIR]
502drive autostart enable|disable|status
502drive autostart systemd status|install|remove
502drive update [--enable-check|--disable-check]
502drive backup PATH
502drive restore PATH --to DIR
502drive cleanup [--dry-run|--apply] [--older-than D]
502drive prune --older-than 90d [--dry-run]
502drive vacuum
502drive completion bash|zsh|fish|powershell
```

Menu uses a TUI only with suitable TTY/style/input settings; otherwise a plain
menu or noninteractive result is used. `--no-input` never opens an input UI.
`menu --job` / `tui --job` opens one job, including beyond the first page.
See [desktop navigation](desktop.md#keyboard-navigation),
[operations](../runbooks.md) and [Windows installation](../install-windows.md).
Update only checks; it does not download or install software.

## Output contract

Global flags include `--config`, `--endpoint`, `--token-file`, `--json`, `--quiet`,
`--no-input`, `--timeout`, `--ascii`, `--plain`, `--color`, `--no-motion`.
Precedence is flags > `DRIVE502_*` environment > configuration > defaults.

| Mode | Standard output | Standard error |
| --- | --- | --- |
| Human | Results/tables | Progress, warnings and errors |
| JSON | `{"data":...,"request_id":...}`; event/watch streams use NDJSON | JSON error envelope |
| Quiet | Relevant ID/result, where supported | Errors |

No banners, color, animation or cursor controls in pipes/JSON/plain output. Drive
names are sanitized for terminal controls/bidi characters. Human operational CLI
text is English; interactive clients support English/Vietnamese. Human dates use
saved locale/timezone; JSON preserves protocol timestamps and stable codes.

| Exit | Meaning |
| --- | --- |
| 0 | Success; a waited job succeeded |
| 1 | Internal/provider error |
| 2 | Usage, input or configuration error |
| 3 | Authentication/reconnection required |
| 4 | Not found |
| 5 | Conflict or stale revision |
| 6 | Daemon/network unavailable or wait timeout |
| 7 | Wait ended partial/failed/blocked |
| 8 | Wait ended cancelled |
| 9 | Onboarding prerequisites incomplete |
| 130 | Interrupted |

Waiting is not ownership of the job: timeout/Ctrl-C can end a client while durable
work continues. Progress/ETA is sampled and withheld when totals or rates are unknown.
See [CLI tests](../../internal/cli) for request, output and exit-code coverage.
