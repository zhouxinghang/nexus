# 01: Auto-fetch title + favicon

Status: done

**What to build:** Let the Owner add a Bookmark by pasting just a URL. Add a
metadata-fetch seam that reads a site's `<title>` and declared favicon; on
create, auto-fill a blank name from the title and make the fetched favicon part
of the icon fallback chain. Owner-entered name/icon always win. Today
`handleCreateBookmark` requires a name and the icon fallback is a client-side
call to Google's favicon service (`faviconFor` in `web/src/pages/Bookmarks.tsx`).

**Blocked by:** None

**Spec:** `.scratch/bookmarks-enrichment/spec.md`

## Acceptance criteria

- [x] A new `Fetcher` seam (in `internal/checker` beside `Prober`, or a sibling
      package) with `FetchMeta(ctx, url) (Meta, error)` where
      `Meta = { Title string; FaviconURL string }`. Real HTTP impl wired in
      `main`; a **`FakeFetcher` injected in tests** (no live network), mirroring
      `Prober`/`FakeProber`.
- [x] The real fetcher does a bounded GET (reuse the probe timeout posture),
      reads a capped number of head bytes, extracts `<title>` and the best
      `<link rel="icon|shortcut icon|apple-touch-icon">` href resolved to an
      absolute URL (fallback `<host>/favicon.ico`). Non-200 / timeout /
      non-HTML degrade to empty `Meta`, never a blocking error.
- [x] Migration adds nullable `fetched_title TEXT` and
      `fetched_favicon_url TEXT` to `bookmarks`, kept **separate** from the
      Owner's `name`/`icon`.
- [x] On `POST /api/bookmarks`: if the request `name` is blank, fill `name` from
      the fetched title (fallback to the URL host if still blank) so a URL-only
      create succeeds. The Owner's `icon` is **never** auto-populated.
- [x] Fetch runs in a bounded background goroutine (like the create-time health
      check) and persists via a query that writes **only** the `fetched_*`
      columns — never Owner `name`/`icon` (mirror `RecordBookmarkCheck`).
- [x] `POST /api/bookmarks/{id}/refetch` refreshes the `fetched_*` columns and
      returns the updated row; 404 for a missing id.
- [x] Response gains `fetched_title` and `fetched_favicon_url` (non-breaking).
      Frontend icon fallback chain becomes: Owner `icon` → `fetched_favicon_url`
      → existing Google favicon service → letter avatar; the add form allows a
      URL-only submit (name optional).
- [x] Tests through the HTTP seam with `FakeFetcher`: blank name filled from
      fetched title; Owner name/icon preserved and not overwritten by a fetch;
      fetcher error/empty still creates the Bookmark; `refetch` updates only
      `fetched_*` and leaves Owner fields intact.

## Notes

- Prefer the standard library for parsing; `golang.org/x/net/html` is the only
  acceptable new dependency if a tolerant parser is needed.
- Validation unchanged: still require a valid http(s) URL (`normalizeURL`); only
  the **name** requirement relaxes when a title can be fetched.
- Do not download/store favicon bytes — store the resolved URL only.

## Comments

- Implemented: `checker.Fetcher` + `HTTPFetcher` (`internal/checker/fetcher.go`)
  and `FakeFetcher`, wired through `httpapi.NewServer(…, fetcher)` and
  `main`. Parsing is stdlib only (a small tolerant scanner, no x/net/html). It
  reads ≤512 KiB of the page, strips comments, picks rel=icon / shortcut icon
  ahead of apple-touch-icon, resolves against the post-redirect URL, and falls
  back to `<origin>/favicon.ico`. Non-2xx and transport errors → `ErrFetch`;
  non-HTML → empty `Meta`. Titles that aren't valid UTF-8 (e.g. GBK pages)
  are dropped rather than stored garbled.
- Migration `000013_bookmark_meta`. Fetched values are written only by
  `RecordBookmarkMeta`, which never touches `name`/`icon`.
- **Deviation from the ticket (flagged):** when the name is blank, the title
  fetch runs *synchronously* (bounded to 5s), because the name must exist when
  the row is inserted and the response should show it. With a name given,
  the fetch runs in the background as specified. The fallback name is the URL
  host (keeping any non-default port).
- Also re-fetches in the background when an edit changes the URL, since the
  stored title/favicon would otherwise describe the old site.
- `POST /api/bookmarks/{id}/refetch` returns the updated row. A failed fetch
  → 502, and the stored values are kept.
- Background work is tracked by a `sync.WaitGroup` on the Server
  (`WaitBackgroundForTest`), so tests stay deterministic and the harness
  can't drop the DB under an in-flight goroutine.
- UI: name is optional on add. The icon chain is Owner icon → fetched favicon
  → Google favicon service → letter. The editor has a "From the site" panel
  with Refetch and "Use as name".
