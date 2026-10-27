# Privacy policy

Last updated: 2026-10-28.

502Drive is self-hosted software. You run its daemon on your machine or server;
there is no hosted 502Drive service, analytics pipeline or telemetry endpoint.

## Data stored locally

| Data | Storage and purpose |
| --- | --- |
| Configuration | Private `config.json`; paths and preferences |
| API bearer token | Private `token_file`, read by the daemon and local clients |
| Google OAuth client | Private copy created by setup under the configuration directory |
| Google access/refresh tokens | AES-GCM encrypted `secrets/google-token.enc`; key in `secrets/master.key` |
| Telegram bot token | Private file referenced by configuration, when enabled |
| Drive and job metadata | SQLite `state-v2.db`: names, IDs, resource keys, mappings, events and sync state |
| Telegram state | Local inbox, callbacks and outbox for replay and delivery |
| Snapshots | Metadata and references to retained physical files in your Google Drive |
| Backups | User-selected directories containing database, configuration and token/key material |

Private files use mode 0600 and directories 0700, or the corresponding user
access restrictions on Windows. Existing installations may retain the earlier
`502drive-next` configuration directory. File contents are never downloaded to
relay a copy, and Google passwords are not stored.

## Network destinations

- Google: OAuth authorization, metadata, server-side copies and other explicitly
  requested Drive operations.
- Telegram: only when configured; commands, cards, file names and job notifications.
- GitHub: release checks when you run `502drive update` or enable tray checks.
- Loopback: the CLI, TUI and tray communicate with your daemon over local HTTP.

Tokens and resource keys are not intentionally exposed in logs or notification
URLs. Logs may contain operational metadata and file names; treat them as private.
There is no project-operated recipient for your job data.

## Retention and removal

Job state and mappings support retries and reconciliation. Eligible historical
records can be pruned by maintenance, including the daemon's default daily policy.
Current retention-settings boundaries are documented in the runbook. Use documented
maintenance and retention commands rather than removing database files while the
daemon runs. Snapshot retention removes metadata; it does not automatically remove
archived Drive copies. See [protection](docs/features/protection.md).

Stop the daemon before removing local state. Removing `master.key` prevents the
stored token from being decrypted. Backups contain decryption material and must be
protected like credentials. Revoking Google access is separate from deleting local
files: use `502drive account revoke` or your Google account permissions page.

## Contact

For privacy questions, open a repository issue without credentials or private
metadata. Report security concerns privately as described in [Security](SECURITY.md).
See the [threat model](docs/threat-model.md) for trust boundaries.
