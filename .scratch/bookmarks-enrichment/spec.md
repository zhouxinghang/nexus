# Bookmarks Enrichment Spec

Status: ready-for-agent

## Problem Statement

The Bookmarks board (shipped in `.scratch/bookmarks/`) stores an Owner-given
name, an http(s) URL, and an optional icon, and periodically health-checks each
site. Two things make it more tedious and less useful than comparable
self-hosted navigation boards (linkding, Linkwarden, Flame):

1. **Everything is manual.** Adding a Bookmark means typing a name by hand, and
   the icon is either an emoji/image URL the Owner supplies or a favicon fetched
   client-side from Google's favicon service (`faviconFor` in
   `web/src/pages/Bookmarks.tsx`). The site's own `<title>` and its real favicon
   are never used, so a quick "paste a URL" flow isn't possible and the board
   depends on a third-party favicon proxy.
2. **There's no sense of what the Owner actually uses.** Bookmarks are a flat
   list ordered by creation time with no signal of which links get opened. A
   navigation board's whole point is fast access to frequently-visited sites,
   but Nexus can't surface them because it doesn't count opens.

## Solution

Enrich Bookmarks on the existing architecture (single Go process, chi + sqlc +
golang-migrate, injected network seam; ADR-0005 — Bookmarks is the one place
external URLs live — unchanged):

- **Auto-fetch title + favicon.** A metadata-fetch seam (mirroring the existing
  `checker.Prober`) fetches a URL's HTML and extracts its `<title>` and declared
  favicon. On create, a blank name is auto-filled from the fetched title; the
  fetched favicon URL becomes part of the icon fallback chain. Owner-entered
  name/icon always win and are never clobbered by a re-fetch.
- **Click-count tracking.** Each time the Owner opens a Bookmark, Nexus records
  the open and keeps a running count plus a last-opened timestamp, enabling a
  "most-visited" sort and a visible open count.

Everything stays single-Owner and monitoring-only (no control of remote sites,
per ADR-0002's posture). No new runtime dependencies: HTML title/favicon parsing
uses the standard library (`net/http` + lightweight tokenizing); if a tolerant
HTML parser is wanted, `golang.org/x/net/html` is the one acceptable addition.

## User Stories

### Auto-fetch title + favicon

1. As the Owner, I want to paste just a URL and have Nexus fill in the site's
   title as the name, so that adding a Bookmark is one step.
2. As the Owner, I want Nexus to use the site's real favicon as the default
   icon, so that the board looks right without me supplying an image.
3. As the Owner, I want my own name and icon to always take precedence over the
   fetched ones, so that a re-fetch never overwrites what I set deliberately.
4. As the Owner, I want title/favicon fetching to fail gracefully (keep whatever
   I typed, fall back to the existing favicon behavior), so that an unreachable
   or odd site never blocks creating the Bookmark.
5. As the Owner, I want to re-fetch metadata for an existing Bookmark on demand,
   so that I can refresh a site's title/favicon after it changes.

### Click-count tracking

6. As the Owner, I want Nexus to count each time I open a Bookmark, so that it
   knows which sites I use most.
7. As the Owner, I want to see a Bookmark's open count and when I last opened it,
   so that I have a sense of usage.
8. As the Owner, I want to sort the board by most-visited, so that my frequent
   sites rise to the top.
9. As the Owner, I want recording an open to never block or delay the site from
   opening, so that clicking stays instant.

## Implementation Decisions

### Metadata fetch

- A new seam `Fetcher` (in `internal/checker` alongside `Prober`, or a sibling
  package) with `FetchMeta(ctx, url) (Meta, error)` returning
  `{ Title string; FaviconURL string }`. A real HTTP implementation is wired in
  `main`; a **fake is injected in tests** so no live network calls happen —
  exactly mirroring how `Prober`/`FakeProber` work today.
- The real fetcher issues a bounded GET (reuse the probe timeout posture),
  reads a capped number of bytes of the response head, extracts `<title>` and
  the best `<link rel="icon"|"shortcut icon"|"apple-touch-icon">` href,
  resolving it to an absolute URL (falling back to `<host>/favicon.ico`). Non-200,
  timeouts, and non-HTML content types degrade to empty metadata, never an error
  that blocks creation.
- **Owner fields are authoritative.** New nullable columns
  `fetched_title TEXT` and `fetched_favicon_url TEXT` store fetched values
  separately from the Owner's `name`/`icon`. On create, if the request `name` is
  blank, the handler fills `name` from `fetched_title` (falling back to the host
  if still blank). The Owner's `icon` is never auto-populated; instead the UI's
  fallback chain becomes: Owner `icon` → `fetched_favicon_url` → the existing
  Google favicon service → letter avatar.
- Fetching happens off the request path where possible: like the immediate
  health check on create, kick off the metadata fetch in a bounded background
  goroutine and persist via a query that only writes the `fetched_*` columns
  (never Owner fields), mirroring `RecordBookmarkCheck`.
- A manual re-fetch endpoint (`POST /api/bookmarks/{id}/refetch`) updates the
  `fetched_*` columns and returns the updated row.

### Click-count tracking

- New columns `click_count BIGINT NOT NULL DEFAULT 0` and
  `last_clicked_at TIMESTAMPTZ`.
- `POST /api/bookmarks/{id}/click` atomically increments the count and stamps
  `last_clicked_at`, returning `204 No Content` (cheap, fire-and-forget). The
  frontend calls it when the Owner opens a Bookmark; since the link opens in a
  new tab the current page survives, but use `keepalive`/`sendBeacon` so a same-
  tab navigation still records.
- `GET /api/bookmarks` gains an optional `sort` param: `created` (default,
  current behavior) or `clicks` (most-visited first, ties broken by recency).
- The response includes `click_count` and `last_clicked_at`; the card shows the
  count subtly.

### API contract (shape, not paths)

- All endpoints stay under the authenticated `/api` group with the existing JSON
  error convention and cookie session.
- `click_count`, `last_clicked_at`, `fetched_title`, `fetched_favicon_url` are
  added to the bookmark response object (non-breaking additions).

## Testing Decisions

Same seams as the Bookmarks module: drive the real chi router via `httptest`
against ephemeral Postgres, authenticate, and inject the fakes.

- **Metadata fetch:** inject a `FakeFetcher` returning canned title/favicon.
  Assert: creating with a blank name fills the name from the fetched title;
  Owner-supplied name/icon are preserved and not overwritten by a fetch; a
  fetcher error/empty result still creates the Bookmark (graceful fallback);
  `refetch` updates only the `fetched_*` columns and leaves Owner fields intact.
- **Click tracking:** assert `POST /api/bookmarks/{id}/click` increments
  `click_count` and sets `last_clicked_at`, returns `204`, and 404s for a
  missing id; assert `GET /api/bookmarks?sort=clicks` orders by count desc with
  a recency tiebreak; assert the default sort is unchanged (creation order).

## Out of Scope

- Scraping or storing page content/description/Open Graph beyond title + favicon.
- Caching/downloading favicon bytes locally; store the resolved URL only.
- Per-click analytics beyond a running count + last-opened timestamp (no history
  table, no charts).
- Any change to the Files local-only invariant (ADR-0005) — Bookmarks remains
  the sole home of external URLs.
- Active control of remote sites.

## Further Notes

- Vocabulary follows `CONTEXT.md`: Bookmark, Health. Fetched title/favicon are
  internal enrichment of an existing Bookmark, not new domain nouns. "Open
  count" / "clicks" is UI copy, not a stored glossary term.
- Relevant ADRs: 0005 (Bookmarks is the one place external URLs live — the
  fetcher reaches external sites, consistent with that), 0002 (monitoring, not
  controlling — fetching metadata is a read).
- Tickets: `issues/01-auto-fetch-title-favicon.md`, `issues/02-click-count.md`.
  They are independent and can be done in either order.
