# Files: distinct folder/type modes and a quieter page

Confirmed request: 2026-10-14 (Asia/Shanghai)

## Requirements

- Folder mode follows traditional Finder-style hierarchy: complete direct
  children, no pagination, single-click selection, double-click/Enter opening,
  breadcrumbs, browser back/forward and a parent-folder button/shortcut.
- Type mode browses recursively and groups each server page by Resource kind.
  Combine kind, Managed Root, directory subtree, exact tag and literal,
  case-insensitive name-substring filters. Counts use the same filters; kind
  counts deliberately ignore only the active kind.
- Type mode uses real offset pagination, not client filtering or accumulating
  “load more” pages. Folder mode must not silently stop at the normal page cap.
- Keep empty and Missing directories navigable, preserve Resource metadata,
  preview/editing and individual/batch mutations.
- Reduce permanent UI: one browser surface, compact navigation, root management
  in a dialog, advanced filters behind a disclosure, an explicit detail toggle,
  batch forms in dialogs. Selection must not move the list during double-click.
- URL-backed mode/scope/filter/sort/page state; stale responses cannot replace
  the current view. Responsive layout, visible keyboard focus and modal focus
  containment.

## API

- `GET /api/files/resources?mode=folder&root_id=ID&parent=ABSOLUTE_DIRECTORY`
  returns all direct indexed Resources plus navigation directories. `limit: 0`
  denotes the unpaged response. Optional `sort` / `order` remain supported.
  Paging and type/tag/name/prefix filters are rejected in this mode.
- `GET /api/files/resources?mode=type&root_id=ID&dir=ABSOLUTE_DIRECTORY&kind=KIND&tag=TAG&name=TEXT&limit=200&offset=0`
  supports optional filters, including omitted root_id for all Managed Roots.
  Folder prefixes match directory boundaries; names escape SQL wildcards.
- Omitting `mode` retains the previous paginated API and parent/prefix behavior.
  Only a validated direct-directory query can disable the page cap.

## Scope

No schema migration, dependencies or separate file service. This is not full
Finder parity: native desktop integration, drag-to-move and file creation are
not added.
