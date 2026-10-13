# 02: List pagination (Journal + Files)

Status: ready-for-agent

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

- [ ] `GET /api/journal/entries` accepts `limit` (default page size, capped) and
      an opaque `cursor`; responds `{ "items": [...], "next_cursor": "<opaque|null>" }`.
- [ ] Keyset is on `(created_at DESC, id DESC)` so appends never cause a page to
      skip or duplicate an entry. `next_cursor` is `null` on the last page.
- [ ] The same `limit` + `cursor` work for the search path (`?q=`), paginating
      matches with the trigram `ILIKE` query (ADR-0004 unchanged).
- [ ] New sqlc queries add the `WHERE (created_at,id) < (cursor)` bound and
      `LIMIT`; the handler encodes/decodes the cursor and validates paging params
      (`400` on invalid `limit`/`cursor`).

### Files (offset + total)

- [ ] `GET /api/files/resources` accepts `limit` + `offset`; responds
      `{ "items": [...], "total": <n>, "limit": <n>, "offset": <n> }` for the
      current filter set (`root_id`, `dir` prefix, `tag`).
- [ ] The **tag path stops loading the whole table into Go**: push the tag
      filter, directory prefix, limit, and offset into SQL (join
      `resource_tags`) and return a `COUNT(*)` for the same filter so `total` is
      correct.
- [ ] `path ASC` ordering is preserved within a page.

### Shared

- [ ] A documented envelope convention (one shape per endpoint); no-paging-param
      callers get the first page. Update the existing frontend callers
      (`web/src/pages/Journal.tsx`, `web/src/pages/Files.tsx`) and tests to the
      new response shape — a "load more" / "end of list" affordance in the UI.
- [ ] Tests (`internal/httpapi`): seed > 1 page of entries and resources; assert
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
