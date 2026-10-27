# H1 — Additive client APIs

Expose existing service capabilities without weakening scope or old response fields.
Collection `include_retired` is separate from owner `all`; cursor identity includes
that filter. Latest-audit lookup returns not-found when absent and counts remaining
repairable findings through an index.

Owner Drive search is bounded, excludes trash, escapes query syntax and respects
Google visibility. Operator `/find` later gained a separate library-only path;
it does not make whole-Drive search available to operators.

Job listing combines state and collection filters with indexed keysets. Cursor
reuse under another filter is invalid. Daily-report settings use the shared
preferences rather than a second chat-only schedule. Configurable job slots remain
separate from shared worker limits.

Verify principal isolation, cursor binding, missing audits, bounded search and query
plans. Current HTTP shapes live in [OpenAPI](../contracts/openapi.json); UI handoff
history is not a public API contract.
