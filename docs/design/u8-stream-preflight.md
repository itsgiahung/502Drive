# U8 — Stream-driven deep preview

Successful snapshots after an SSE change re-read the visible running plan.
Dedicated 700ms polling stops while stream-connected; reconnect loss resumes the
existing fallback. A loading flag coalesces requests. Finished/cancelled plans
and late replies never restart polling. Plan writes already bump the state change
counter, so no server/engine/schema change is needed. Partial scans explicitly
say total unknown. No new write route or denser polling.

A regression test caught a refresh-feedback risk: the queue-position snapshot used
Store.tx, whose commit emits a change even for reads. It now uses a read-only
transaction without a write signal; GET queue reads cannot sustain an SSE loop.
