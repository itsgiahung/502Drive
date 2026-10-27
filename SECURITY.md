# Security

## Report a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/itsgiahung/502Drive/security/advisories/new).
Do not publish credentials or exploitable details in a public issue. Include the
affected version, reproduction steps and practical impact.

Security fixes target the current Go release line. Check the release tag and
`502drive version`; old binaries may refuse databases upgraded by newer migrations.

## Keep private data out of Git

Never commit configuration, OAuth clients, API/bot tokens, `.env`, `master.key`,
`google-token.enc`, databases/WAL files, backups, logs or reports with private
metadata. Use synthetic fixtures. Ignore rules are a safeguard, not a substitute
for reviewing what is staged.

## Rotate credentials

1. Google: run `502drive account revoke`, log in again and check `doctor --online`.
   You can also revoke the application's access through your Google account.
2. API bearer token: stop the daemon, replace the private single-line `token_file`,
   restart and point clients at that same file.
3. Telegram: revoke the bot token with BotFather, update its private token file
   and restart. Discard obsolete callbacks.
4. Master key: reauthorize the Google account to establish new token material;
   keep old backups private if they are still needed.

## Boundaries

The API binds to loopback and requires a bearer token. Container deployments bind
inside the container but publish only to host loopback. Use SSH forwarding for
remote control. Google tokens are encrypted with AES-GCM; local compromise of the
same user account can still expose the key and token together.

Telegram requires private chat, an allowlist and server-side owner/operator scope.
Mutation callbacks are one-shot, expiring and revision-bound. Chat content is never
executed by a shell. Remote cleanup only trashes owned objects; there is no
permanent Drive delete route.

The review scope includes the daemon, clients, Telegram adapter, maintenance and
packaging. Host root/user compromise and Google's permission/quota decisions are
outside the protection provided by this application. See the
[threat model](docs/threat-model.md) and [third-party notices](THIRD_PARTY_NOTICES.md).
