# U4 — Honest progress

present owns the bounded 30-second sample window and formatter. At least three
samples (distinct increasing timestamps) are needed. Rate is completed items per
second; byte counts always come from logical_bytes. Counters moving backwards
reset the window. Zero/stalled or >2x-varying intervals suppress ETA. Unknown
total always says so and never has ETA. Any ETA is explicitly estimated.
Existing Progress remains the formatting entry point; RecentProgress supplies
only measured rates. No persistence, routes, engine or provider changes.
