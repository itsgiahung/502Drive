# Threat model

502Drive has one daemon and local clients. There is no web frontend or browser
session authentication. Google and Telegram remain external trust boundaries.

## Assets

| Asset | Location |
| --- | --- |
| Google access/refresh tokens | AES-GCM encrypted file; separate private master key |
| API bearer token and Telegram bot token | Private local files |
| Drive names, IDs and resource keys | Private SQLite database and authorized responses |
| Jobs, intents, mappings and callbacks | SQLite database |
| Backups | Private directories containing database and credential material |
| Retained Drive copies | The user's destination/archive folders |

## Controls

| Boundary | Main risks | Controls |
| --- | --- | --- |
| HTTP | Unauthorized access and forged actors | Loopback, bearer authentication, constant-time comparison, server-side principal |
| Telegram | Strangers, group chats, stale callbacks | Private chat, allowlist, owner/operator scope, expiring one-shot revision-bound callbacks |
| Drive | Malicious names, provider failures, ambiguous writes | Sanitized presentation, classified errors, durable intents and reconciliation |
| Terminal/tray | Escape sequences and command injection | Sanitization, fixed argument vectors, parsed Drive references |
| Maintenance | Overwrite, traversal, incomplete backups | Writer lock, empty target, integrity/hash/FK checks, symlink restrictions |
| Configuration | Secrets in logs or responses | Private files, redaction, secret-free DTOs |
| Installer | Unknown publisher or tampered binaries | Published checksums; explicit unsigned-installer notice |
| Mirror/restore | Removing unrelated data | Exact ownership, complete scan/preflight, ordered durable actions, empty-folder checks |

## Limits

- A compromised OS user or root account can access local credentials and session APIs.
- Google decides permissions, copy eligibility, quotas and object availability.
- A user may share or edit destination files outside 502Drive. Snapshots retain
  physical copies and do not make independent immutable byte backups.
- Pinned dependencies reduce drift but do not establish supply-chain attestation.
- Checksums do not provide code signing or proof of a trusted publisher.

If a credential is exposed, follow [Security](../SECURITY.md), create a fresh backup
and treat existing backups as secrets. See [Privacy](../PRIVACY.md).
