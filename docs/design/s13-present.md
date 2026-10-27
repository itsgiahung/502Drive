# S13 — Shared client presentation

`internal/present` centralizes bilingual state labels, compact progress, human byte
sizes, durations and job summaries. Clients use shared stable-code explanations
rather than guessing provider causes independently.

Formatting handles zero/unknown/negative values and unknown states predictably.
Unknown total is not zero; no rate/ETA is invented without enough observations.
Machine JSON and exit codes remain independent from localized presentation.

Table-driven tests cover English/Vietnamese and boundary values. Follow-up U3/U4
notes describe the explanation catalogue and sampled progress. See
[presentation code](../../internal/present), [U3](u3-plain-errors.md),
[U4](u4-honest-progress.md).
