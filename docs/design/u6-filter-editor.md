# U6 — Collection filter editor

Collection B opens six fields: include/exclude rules (semicolon separated), minimum
and maximum size, modified-after, keep-empty-folders. Path/name globs, ext: and
class: use internal/filter directly; no second rule parser or matcher. Existing
b presets remain shortcuts. Validate on every edit, beside the input; invalid
rules cannot preview/save. Ctrl+P calls the existing plan API with the collection
source/destination and shows the top-level scope explicitly; it is not a whole-tree
count. Ctrl+S saves through the revision-checked filter route. No engine/schema
change. The complete stored Spec is retained, including fields older TUI omitted.
