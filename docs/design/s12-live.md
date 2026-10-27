# S12 — Real-provider harness and fault injection

`drive.FaultInjector` wraps an HTTP RoundTripper to simulate rate limits,
connection failures, lost responses, delays and server errors. Ordinary injector
tests run locally without Google credentials; faults are injected at the transport
boundary rather than replacing engine semantics.

The real-provider harness is separately gated by build tag `live` and explicit
`DRIVE502_LIVE=1`. It performs actual writes and requires an authorized test account.
Each run creates an isolated application test root; cleanup trashes that root,
never permanently deletes it. No unrelated user data is a valid test target.

The current supported credential mechanisms are defined by
[internal/live](../../internal/live), not by old draft environment names. Supply
credentials privately; never paste them into docs, reports or command-history examples.

Live scenarios can cover small-tree copy, OAuth/account access, uncertain-write
reconciliation and Changes handling, with optional Telegram probes. Check the actual
implemented tests and [verification](../verification.md) before claiming a scenario
ran. Normal `go test ./...` does not enable the live tag or authorize provider writes.
