# U13 — Open the selected job

Tray action IDs remain job:0..2; resolve the ID from the current snapshot under
lock, never encode a Drive name into an action. menu gains alias tui and additive
--job. The CLI resolves/auth-checks the job before opening; snapshots fetch that
job explicitly when it is outside the first page. The model keeps job identity
across list refreshes. Plain TTY fallback prints the target's job details.
Terminal launch preserves explicit config/endpoint/token-file flags. Windows uses
wt or a native new console (no cmd shell for names/paths/IDs). IDs passed to a
terminal are constrained to generated job identifiers. The U12 Open hook uses
this same path. No engine/DB writes or new HTTP routes.
