# H5 — Locale, user service and prune references

Human CLI values use shared date/time, byte and reason formatting. Preferences
are cached per command; JSON retains protocol timestamps and stable fields.
Formatting failure falls back without dropping the original value.

`autostart systemd install` writes a private user unit, reloads and enables it;
it does not start the service automatically. Status/remove are idempotent and
respect XDG paths. Doctor distinguishes absent, enabled and installed-but-disabled
units. Desktop autostart and service restart are alternative mechanisms.

Prune keeps jobs referenced by mappings, plans, sync, audits, pending outbox and
snapshots; foreign-key integrity is required. Offline lock checks remain mandatory.
The CLI's `--dry-run` must be supplied to preview pruning.

Tests exercise localized values without JSON changes, user-unit status and
referential retention. Actual systemd/desktop behavior needs native acceptance.
See [operations](../runbooks.md).
