# S5 — Selective copy filters

`internal/filter.Spec` is persisted from plan to job to collection. A compiled
matcher runs during discovery before committing write work. The same rule parser
serves CLI/TUI/Telegram rather than adapter-specific matching implementations.

Rules include name/path globs (`**` for descendants), extensions, MIME classes,
size bounds and modified-after. Exclusions win. Excluded folders are marked listed
without traversing their subtrees. Omit filtered empty folders unless configured
to retain them; the source root is treated separately.

Migration 011 adds relative paths and indexed empty-folder queries. Folder moves
update descendant paths. Matching is per page and adds no metadata-list calls.
The shallow preview describes a top-level page, never the full tree.

A changed filter affects later runs, not the current admitted rule snapshot and
not deletion of existing output. Previously excluded items can become pending;
in-flight intents are preserved. Mirror treats exclusion as preservation, not
source disappearance. **Single-file roots are not filtered.**

See [copy jobs](../features/clone-jobs.md),
[filter package](../../internal/filter), [filter editor](../features/desktop.md).
