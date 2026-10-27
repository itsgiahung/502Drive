# T1 — Power and network lifecycle

The daemon coordinates sleep/wake, connectivity and in-flight write safety. It
never turns an uncertain network result into permission to repeat a write.

Portable wall/monotonic clock-gap detection provides a fallback when native hooks
are missing. Linux logind and Windows power/network notifications improve timing;
callbacks remain bounded and do not run worker logic directly.

## Transitions

- Startup reads the durable heartbeat; a long gap enters resuming/probe mode.
- Suspend sets admission unavailable, closes the network claim gate and checkpoints
  WAL under a bounded budget before releasing any delay inhibitor.
- Resume checks local state, resets idle connections and probes with backoff before
  opening the gate and waking workers.
- Authentication failure exposes reconnect-required instead of a rapid retry loop.

Definitely unsent/unreachable requests release work without spending attempts.
Listing retries the same page after connectivity returns, never commits a fabricated
empty page. Timeout/EOF after a possible write enters intent reconciliation.
Restart recovers executing work through the existing reconciliation path.

Keep-awake is opt-in, held only while work requires it and released afterwards.
It is a convenience, not a guarantee that the OS cannot suspend. Clients display
lifecycle status but never implement a second engine or database writer.

Tests use fake clocks, provider faults and native-source adapters to cover gap,
flapping connectivity, single-flight probe, shutdown/cancellation and no-penalty
network waits. Real suspend/hibernate/VPN/OS-restart acceptance remains separate:
[runbook checklist](../runbooks.md#native-laptop-checks).
