# Files: tags-only metadata, directory hierarchy and sorting

## Requirements

- Remove Resource notes from the UI, HTTP API and database; retain tags.
- Browse a Managed Root one directory at a time, including empty directories,
  with breadcrumbs and browser back/forward. Never flatten its descendants.
- Keep Missing Resources reachable through indexed ancestor directories.
- Support ascending/descending Size and Modified sorting before pagination;
  retain Name sorting and place navigation directories above Resources.
- Confine directory reads to the Managed Root, including symlink traversal.
- Preserve the existing Resource preview, text editing and batch operations.

## API

`GET /api/files/resources?root_id=ID&parent=ABSOLUTE_DIRECTORY&sort=size|modified|name&order=asc|desc`

`parent` returns direct Resource children and a `directories` array. Type/tag
filters and kind totals are scoped to direct Resources; directories remain
navigable regardless of those filters. Without `parent`, the existing recursive
API and `dir` prefix filter remain compatible.

## Data migration

Migration 17 removes `resources.note`. Existing note text is intentionally
discarded; tags, Resource identity and Host content remain unchanged.
