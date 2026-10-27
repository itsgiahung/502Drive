# Identity and access control

Principals are derived from the authenticated transport, never from an actor field
in a request body. Store operations enforce scope again before mutations.

| Principal | Authentication | Scope |
| --- | --- | --- |
| Owner (`owner`) | Private HTTP bearer token; configured Telegram owner ID | Administration and all collections/jobs |
| Operator | Enabled Telegram principal | Its own jobs, collections, sync and audits; existing destinations only |

Owner-only administration includes destinations, Drive browse/search/mkdir,
operators, Doctor/fixes, global queue pause/resume and `all=true` reads. Collection
lifecycle checks ownership; another operator's object is returned as `404`.
Owner control of an operator's job retains the owner's actor identity for
idempotency rather than impersonating the operator.

HTTP bearer tokens are read from private files and compared in constant time.
There is currently one owner token, no `api_tokens` table or separately revocable
operator HTTP tokens. Operators access the service through Telegram.

Telegram requires private chat and an allowed enabled principal. Mutation
callbacks validate principal, chat, expiry, revision and one-shot consumption.
An opaque ID does not grant access by itself.

Google uses a Desktop OAuth client with PKCE. Tokens are encrypted with AES-GCM
using a private master key. Account states include `disconnected`, `connected`,
`reconnect_required`, `revoking` and `revoked`. Tokens, codes and verifiers must not
appear in logs or DTOs.

There is no application SSO, folder-level ACL editor or separate administrative
audit-log product. Jobs have durable events. See [Security](../../SECURITY.md),
[state/principals.go](../../internal/state/principals.go),
[HTTP authentication](../../internal/httpapi/server.go) and
[Telegram authorization](../../internal/telegram/telegram.go).
