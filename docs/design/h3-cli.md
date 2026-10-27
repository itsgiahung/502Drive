# H3 — Fast CLI paths

Bare Drive references invoke copy after global-flag parsing. Unknown commands
remain usage errors. Job references support full IDs, unique prefixes, `@last`
and `@running`; ambiguous prefixes are never guessed.

`wait` shares the existing waited-job contract. `watch` emits periodic human lines
or one JSON envelope per line and uses the same terminal exit mapping. Dynamic
completion queries bounded recent jobs/collections and stays silent when configuration
or daemon access is unavailable.

Doctor fixes only safe local permission/directory issues via the owner service.
It does not change Drive files or job state. All operational commands preserve
stdout/stderr separation, JSON envelopes, noninteractive behavior and exit codes.

See [CLI reference](../features/cli.md) and [CLI tests](../../internal/cli).
