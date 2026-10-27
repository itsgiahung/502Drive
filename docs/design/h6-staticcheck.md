# H6 — Static analysis hygiene

Run `staticcheck ./...` alongside vet and race tests. Remove genuinely unused
functions rather than suppressing diagnostics. Preserve validation, error handling
and published CLI behavior while simplifying dead paths.

Past cleanup covered impossible-interface comparisons, error capitalization,
unnecessary formatting/append loops and unused test values. These are historical
categories, not a claim that analysis has run on every later commit.

See current commands and evidence in [verification](../verification.md).
