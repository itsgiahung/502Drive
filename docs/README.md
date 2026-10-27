# Documentation

502Drive is one Go 1.27 module. The application supports English and Vietnamese;
public documentation is English except for the [Vietnamese README](../README_VI.md).

## Start here

| Goal | Document |
| --- | --- |
| Install and make a first copy | [Project README](../README.md) |
| Linux packages, systemd and tray | [Linux](install-linux.md) |
| Windows ZIP and installer | [Windows](install-windows.md) |
| Container deployment | [Docker / Podman](install-docker.md) |
| Configure Google access | [OAuth setup](oauth-setup.md) |
| Diagnose an issue | [Troubleshooting](troubleshooting.md) |
| Backup, restore or upgrade | [Operations runbook](runbooks.md) |

## Features

| Document | Scope |
| --- | --- |
| [Copy jobs](features/clone-jobs.md) | Fidelity, conflicts, filters, queue, audit and lifecycle |
| [Automation](features/automation.md) | Scheduled/realtime sync, replay, quota and throttling |
| [Protection](features/protection.md) | Opt-in mirror, snapshots, pinning and restore |
| [CLI](features/cli.md) | Commands, JSON, standard streams and exit codes |
| [Desktop](features/desktop.md) | TUI navigation, tray, notifications and autostart |
| [Telegram](features/telegram.md) | Bot setup, private chats, callbacks and notifications |
| [Identity](features/identity-control.md) | Owner/operator boundaries and credentials |

## Maintainers

Read [AGENTS.md](../AGENTS.md), then architecture and the contract you are changing.

| Document | Scope |
| --- | --- |
| [Architecture](architecture.md) | Packages, worker flow and invariants |
| [HTTP contract](control-contracts.md) | Routes, authorization, idempotency and SSE |
| [OpenAPI](contracts/openapi.json) | Machine-readable API schema; 70 operations |
| [Data contracts](data-contracts.md) | SQLite migrations 001–011 and 020–027 |
| [Threat model](threat-model.md) | Assets, trust boundaries and limitations |
| [Acceptance](acceptance.md) | Coverage and remaining release checks |
| [Verification](verification.md) | Evidence, commands and live-test boundaries |
| [Design notes](design/) | Technical decisions for individual features |

Local handoffs, credentials, runtime data, test reports and build outputs are not
public product documentation and must remain outside Git.

## Checks

```sh
gofmt -l cmd internal
go vet ./...
staticcheck ./...
go test -race -count=1 -timeout 900s ./...
python3 scripts/check-docs-links.py .
python3 -c "import json; json.load(open('docs/contracts/openapi.json'))"
git diff --check
```

Fake-provider tests establish local behavior; they do not establish Google,
Telegram or native desktop behavior. See [verification](verification.md).
