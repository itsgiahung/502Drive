# Working in 502Drive

Go, HTTP-first, with CLI/TUI, tray launcher and Telegram (the earlier Rust/Tauri
implementation was removed on 2026-10-26). Start at the [map](docs/README.md), read the
[architecture](docs/architecture.md), then the feature or contract you touch.

## Before editing

- `git status --short`; keep changes you did not make. Do not commit, push or
  deploy unless asked.
- Use `rg` within scope; do not read every doc each session.
- Never put real config, credentials, DBs, logs, reports, resource keys or
  `master.key` in examples or handoffs. Synthetic fixtures only.

## Invariants

- One Go daemon owns the DB, workers and tokens. The CLI/TUI/tray are HTTP clients.
  Telegram uses the same `internal/app` service and authorization. No second
  engine, no client writes to the DB. HTTP does not hold a request until a copy ends.
- Server-side copy only, source → destination, preserving names and tree within
  [copy fidelity](docs/features/clone-jobs.md). No relaying file bytes, no quota or
  permission bypass, no automatic public links, no promise of identical IDs,
  owners, permissions or revisions.
- The daemon runs detached; only `502drive stop` or the tray's *Stop 502Drive…*
  (`POST /v1/shutdown`) stops it. The TUI or tray exiting, closing a terminal
  tab or an SSE client leaving never does. Realtime is `GET /v1/stream`
  (change counter only; clients re-read).
- Admission (job + idempotency + first event) commits before "accepted". Job IDs
  are durable; accepted ≠ succeeded; retry and restart keep operation identity.
- A write with an unknown outcome is reconciled, never blindly resent.
- Cleanup and repair default to a dry run and only move objects carrying 502Drive's
  intent to the Drive trash; there is no permanent Drive delete.
- A global queue pause is durable (`settings.queue_paused`); resuming only restarts
  jobs the queue paused, keeping user pauses. Shared Drives use the same copy rules
  and scopes.
- Telegram checks allowlist, private chat, owner/operator scope and one-shot,
  expiring callbacks tied to a revision. Never shell-execute chat content.
- CLI stdout/stderr/exit code/JSON are a contract ([CLI](docs/features/cli.md)):
  colour/mascot only on a TTY, no banner in pipes, never prompt in scripts.
- Scheduled and realtime sync keep the collection, root and mapping across runs
  ([automation](docs/features/automation.md)); stop/cancel keep what was created.
- The HTTP contract is [control-contracts](docs/control-contracts.md) +
  [OpenAPI](docs/contracts/openapi.json); schema is [data-contracts](docs/data-contracts.md).
  Migrations are forward-only (`internal/state/migrations`); never edit an applied one.

## Verification and handoff

Pick checks from [runbooks](docs/runbooks.md) and [acceptance](docs/acceptance.md).
Run `gofmt -l cmd internal && go vet ./... && go test -race ./...`. Fake-provider
tests do not prove live Google behaviour; say what was and was not run, as in
[verification](docs/verification.md). Keep this file under ~80 lines and the doc
map under 100; details live in feature/contract docs.
