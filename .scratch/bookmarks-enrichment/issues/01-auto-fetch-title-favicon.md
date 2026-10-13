# 01: Auto-fetch title + favicon

Status: ready-for-agent

**What to build:** Let the Owner add a Bookmark by pasting just a URL. Add a
metadata-fetch seam that reads a site's `<title>` and declared favicon; on
create, auto-fill a blank name from the title and make the fetched favicon part
of the icon fallback chain. Owner-entered name/icon always win. Today
`handleCreateBookmark` requires a name and the icon fallback is a client-side
call to Google's favicon service (`faviconFor` in `web/src/pages/Bookmarks.tsx`).

**Blocked by:** None

**Spec:** `.scratch/bookmarks-enrichment/spec.md`

## Acceptance criteria

- [ ] A new `Fetcher` seam (in `internal/checker` beside `Prober`, or a sibling
      package) with `FetchMeta(ctx, url) (Meta, error)` where
      `Meta = { Title string; FaviconURL string }`. Real HTTP impl wired in
      `main`; a **`FakeFetcher` injected in tests** (no live network), mirroring
      `Prober`/`FakeProber`.
- [ ] The real fetcher does a bounded GET (reuse the probe timeout posture),
      reads a capped number of head bytes, extracts `<title>` and the best
      `<link rel="icon|shortcut icon|apple-touch-icon">` href resolved to an
      absolute URL (fallback `<host>/favicon.ico`). Non-200 / timeout /
      non-HTML degrade to empty `Meta`, never a blocking error.
- [ ] Migration adds nullable `fetched_title TEXT` and
      `fetched_favicon_url TEXT` to `bookmarks`, kept **separate** from the
      Owner's `name`/`icon`.
- [ ] On `POST /api/bookmarks`: if the request `name` is blank, fill `name` from
      the fetched title (fallback to the URL host if still blank) so a URL-only
      create succeeds. The Owner's `icon` is **never** auto-populated.
- [ ] Fetch runs in a bounded background goroutine (like the create-time health
      check) and persists via a query that writes **only** the `fetched_*`
      columns — never Owner `name`/`icon` (mirror `RecordBookmarkCheck`).
- [ ] `POST /api/bookmarks/{id}/refetch` refreshes the `fetched_*` columns and
      returns the updated row; 404 for a missing id.
- [ ] Response gains `fetched_title` and `fetched_favicon_url` (non-breaking).
      Frontend icon fallback chain becomes: Owner `icon` → `fetched_favicon_url`
      → existing Google favicon service → letter avatar; the add form allows a
      URL-only submit (name optional).
- [ ] Tests through the HTTP seam with `FakeFetcher`: blank name filled from
      fetched title; Owner name/icon preserved and not overwritten by a fetch;
      fetcher error/empty still creates the Bookmark; `refetch` updates only
      `fetched_*` and leaves Owner fields intact.

## Notes

- Prefer the standard library for parsing; `golang.org/x/net/html` is the only
  acceptable new dependency if a tolerant parser is needed.
- Validation unchanged: still require a valid http(s) URL (`normalizeURL`); only
  the **name** requirement relaxes when a title can be fetched.
- Do not download/store favicon bytes — store the resolved URL only.
