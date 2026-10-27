# S11 — Linux reliability

Use a bounded systemd user-service restart policy and documented resource/hardening
settings. Doctor checks free disk space, descriptor limits, WAL/checkpoint health
and private-secret permissions without treating a process's existence as readiness.
OS readers can be injected for deterministic local tests.

SQLite supports passive and truncate checkpoints plus optimization. Offline
vacuum holds the data-directory lock and reports before/after storage. No client
or maintenance command may bypass an active writer.

Long-running soak/load checks are separate from ordinary unit tests. Record CPU/RSS,
API counts and exact fixture size only when measured; fake-tree performance is not
Google throughput. See [verification](../verification.md) and
[runbook](../runbooks.md).
