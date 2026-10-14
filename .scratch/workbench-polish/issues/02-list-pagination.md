# 02: List pagination (Journal + Files)

Status: done

**What to build:** Stop the list endpoints from returning entire tables. The
Journal stream/search (`handleListJournalEntries`, queries `ListJournalEntries`
/ `SearchJournalEntries`) and the Files resource list (`handleListResources`,
queries `ListResources` / `ListResourcesByRoot` / the tag path that loads all
rows and filters in Go) all lack a `LIMIT`. Add a consistent paging envelope:
keyset for the Journal (stable append-only ordering), offset+total for Files
(several optional filters), and push the tag filter into SQL.

**Blocked by:** None

**Spec:** `.scratch/workbench-polish/spec.md`

## Acceptance criteria

### Journal (keyset)

- [x] `GET /api/journal/entries` accepts `limit` (default page size, capped) and
      an opaque `cursor`; responds `{ "items": [...], "next_cursor": "<opaque|null>" }`.
- [x] Keyset is on `(created_at DESC, id DESC)` so appends never cause a page to
      skip or duplicate an entry. `next_cursor` is `null` on the last page.
- [x] The same `limit` + `cursor` work for the search path (`?q=`), paginating
      matches with the trigram `ILIKE` query (ADR-0004 unchanged).
- [x] New sqlc queries add the `WHERE (created_at,id) < (cursor)` bound and
      `LIMIT`; the handler encodes/decodes the cursor and validates paging params
      (`400` on invalid `limit`/`cursor`).

### Files (offset + total)

- [x] `GET /api/files/resources` accepts `limit` + `offset`; responds
      `{ "items": [...], "total": <n>, "limit": <n>, "offset": <n> }` for the
      current filter set (`root_id`, `dir` prefix, `tag`).
- [x] The **tag path stops loading the whole table into Go**: push the tag
      filter, directory prefix, limit, and offset into SQL (join
      `resource_tags`) and return a `COUNT(*)` for the same filter so `total` is
      correct.
- [x] `path ASC` ordering is preserved within a page.

### Shared

- [x] A documented envelope convention (one shape per endpoint); no-paging-param
      callers get the first page. Update the existing frontend callers
      (`web/src/pages/Journal.tsx`, `web/src/pages/Files.tsx`) and tests to the
      new response shape — a "load more" / "end of list" affordance in the UI.
- [x] Tests (`internal/httpapi`): seed > 1 page of entries and resources; assert
      first page respects `limit`, cursor/offset fetches the next page with no
      gaps/dupes, `next_cursor`/`total` are correct, the final page signals no
      more, and the tag-filtered Files `total` matches the filter.

## Notes

- Encode the Journal cursor opaquely (e.g. base64 of `created_at` RFC3339Nano +
  `id`) so the client treats it as a token, not a field to construct.
- Keep the Files first pass as offset+limit even though keyset would scale
  better; the optional directory-prefix + tag combinatorics make a stable keyset
  cursor awkward, and offset is adequate for a single Host's file counts. Note
  this trade-off in the handler so a later pass can revisit if needed.
- Don't change the ordering semantics the UI already relies on
  (Journal newest-first; Files `path ASC`).

## Comments

- Envelope conventions are documented in `server/internal/httpapi/pagination.go`.
  Journal uses keyset: `{items, next_cursor}`, default 50 / cap 200. The
  cursor is base64url of `created_at RFC3339Nano|id`. Files uses offset:
  `{items, total, limit, offset}`, default 200 / cap 1000. A `limit` over the
  cap is clamped, not rejected. Bad `limit`/`offset`/`cursor` → 400.
- Journal list and search share the `(created_at, id) <` bound. One extra row
  is fetched to decide `next_cursor`.
- Files: `ListResourcesPage` + `CountResources` push root, dir prefix
  (LIKE-escaped), tag (`EXISTS` on `resource_tags`), limit, and offset into
  SQL. The load-all-then-filter tag path and `ListResourceIDsByTag` are gone.
  Tags for a page load in one batched query. The offset-vs-keyset trade-off is
  noted in the handler.
- `POST /files/roots/{id}/scan` now returns the same envelope for the scanned
  root (it used to return a bare array).
- UI: Journal has "Load older entries" / end-of-list. Files has a "Showing N
  of M" footer with "Load more". After a mutation, Files refetches as many
  rows as were already loaded. The client-side file-type chips apply to loaded
  rows only, and the footer says so.
