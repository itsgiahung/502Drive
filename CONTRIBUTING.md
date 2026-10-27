# Contributing

502Drive is a single Go module: `cmd/` holds the entry points, `internal/` the code.
Read [AGENTS.md](AGENTS.md) for the invariants and [docs/README.md](docs/README.md)
for the map before changing behaviour.

## Setup

```bash
go build ./cmd/502drive
gofmt -l cmd internal && go vet ./... && go test -race ./...
# or: make check   (docs: make docs, full local release build: make release-local)
```

Go 1.27 is required. The SQLite driver is pure Go, so `CGO_ENABLED=0` builds work
(the tray needs a desktop session at runtime, not CGO on Linux/Windows).

## Rules of the road

- Adapters (HTTP, CLI, Telegram, TUI, tray) call `internal/app` or the HTTP API.
  They never write SQL and never contain a second copy engine.
- A change to HTTP behaviour updates [docs/contracts/openapi.json](docs/contracts/openapi.json)
  and the conformance tests in `internal/httpapi`.
- A change to a remote-write path needs a failure test with the fake provider
  (`internal/drive/drivetest`): lost response, timeout, crash between write and record.
- Use synthetic fixtures. Never commit credentials, tokens, databases, reports,
  resource keys or `master.key`.
- Keep `gofmt` clean, one statement per line, comments only where the reason is
  not obvious.
- User-visible text is Vietnamese and English; do not hard-code one language in a
  new adapter surface.
- Public Markdown is English; `README_VI.md` is the Vietnamese introduction.
  Keep commands aligned with code and distinguish local fixtures from live evidence.
  Private handoffs, worker assignments, runtime logs and credentials stay out of Git.
  Update the generator as well when changing generated documentation.

## Pull requests

Describe what changed, which checks you ran (`go vet`, `go test -race`, any manual
smoke) and what you could not verify (live Google Drive, real Telegram, real OS
desktop). Do not claim live behaviour from fake-provider tests.
