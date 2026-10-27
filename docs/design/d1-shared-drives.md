# D1 — Shared Drive support

The existing full Drive OAuth scope is retained. Shared Drives do not imply a
permission or quota bypass. The adapter uses the applicable supports-all-drives,
include-all-drives and corpus options, preserving `driveId` metadata.

## Permission boundaries

- Inspect rejects a single file with `canCopy=false` as `copy_not_permitted`.
- Tree discovery skips restricted children with stable `cannot_copy` reasons.
- Destination inspection requires add-child capability and returns
  `destination_not_writable` when unavailable.
- Permission/not-found responses are distinct from transport/throttle failures.

The fake provider supports Shared Drive roots, inherited Drive IDs and synthetic
viewer/contributor/organizer roles. These fixtures check request/permission paths,
not Google's live role matrix or every cross-drive restriction.

See [copy fidelity](../features/clone-jobs.md),
[Drive adapter](../../internal/drive/client.go) and [verification](../verification.md).
