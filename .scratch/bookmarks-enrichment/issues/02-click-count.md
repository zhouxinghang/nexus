# 02: Click-count tracking

Status: done

**What to build:** Count how often the Owner opens each Bookmark so the board
can surface frequently-visited sites. Today Bookmarks are a flat list ordered by
creation time (`ListBookmarks`), with no usage signal.

**Blocked by:** None

**Spec:** `.scratch/bookmarks-enrichment/spec.md`

## Acceptance criteria

- [x] Migration adds `click_count BIGINT NOT NULL DEFAULT 0` and
      `last_clicked_at TIMESTAMPTZ` to `bookmarks`.
- [x] `POST /api/bookmarks/{id}/click` atomically increments `click_count` and
      sets `last_clicked_at = now()`, returns **`204 No Content`**, and `404`s
      for a missing id. A dedicated sqlc query does the increment (don't reuse
      the Owner-field update path).
- [x] `GET /api/bookmarks` accepts an optional `sort` param: `created` (default,
      current creation-order behavior) or `clicks` (most-visited first, ties
      broken by `last_clicked_at` recency). Invalid `sort` → `400`.
- [x] The bookmark response includes `click_count` and `last_clicked_at`
      (non-breaking additions).
- [x] Frontend: opening a Bookmark fires the click endpoint. Since the link
      opens in a new tab the page survives, but use `fetch(..., {keepalive:true})`
      (or `navigator.sendBeacon`) so a same-tab navigation still records. The
      card shows the open count subtly; add a "Most visited / Recent" sort
      toggle.
- [x] Recording a click **never blocks or delays** opening the site.
- [x] Tests through the HTTP seam: click increments count and stamps
      `last_clicked_at` and returns `204`; `404` for a missing id;
      `?sort=clicks` orders by count desc with a recency tiebreak; default sort
      unchanged (creation order).

## Notes

- Keep the click endpoint cheap and side-effect-only; no body, no returned row.
- `sort=clicks` tiebreak: `ORDER BY click_count DESC, last_clicked_at DESC NULLS LAST,
  created_at ASC, id ASC` keeps ordering stable for never-clicked bookmarks.
- This is a running counter only — no per-click history table, no charts (out of
  scope per the spec).

## Comments

- Implemented: migration `000014_bookmark_clicks`. `RecordBookmarkClick`
  (dedicated, atomic `+1`, doesn't bump `updated_at`).
  `POST /api/bookmarks/{id}/click` → 204 / 404.
- `GET /api/bookmarks?sort=created|clicks`, invalid → 400.
  `ListBookmarksByClicks` orders by `click_count DESC, last_clicked_at DESC
  NULLS LAST, created_at, id`.
- UI: opening a card (click or middle-click) fires a `fetch(…, {keepalive:
  true})` that is never awaited, so it can't delay the navigation. The count
  updates optimistically. There's a subtle open-count on each card, an
  "Opened N times · last …" line in the editor, and a Recent / Most visited
  toggle persisted under `nexus-bookmarks-sort`.
