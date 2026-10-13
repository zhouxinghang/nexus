# 02: Click-count tracking

Status: ready-for-agent

**What to build:** Count how often the Owner opens each Bookmark so the board
can surface frequently-visited sites. Today Bookmarks are a flat list ordered by
creation time (`ListBookmarks`), with no usage signal.

**Blocked by:** None

**Spec:** `.scratch/bookmarks-enrichment/spec.md`

## Acceptance criteria

- [ ] Migration adds `click_count BIGINT NOT NULL DEFAULT 0` and
      `last_clicked_at TIMESTAMPTZ` to `bookmarks`.
- [ ] `POST /api/bookmarks/{id}/click` atomically increments `click_count` and
      sets `last_clicked_at = now()`, returns **`204 No Content`**, and `404`s
      for a missing id. A dedicated sqlc query does the increment (don't reuse
      the Owner-field update path).
- [ ] `GET /api/bookmarks` accepts an optional `sort` param: `created` (default,
      current creation-order behavior) or `clicks` (most-visited first, ties
      broken by `last_clicked_at` recency). Invalid `sort` → `400`.
- [ ] The bookmark response includes `click_count` and `last_clicked_at`
      (non-breaking additions).
- [ ] Frontend: opening a Bookmark fires the click endpoint. Since the link
      opens in a new tab the page survives, but use `fetch(..., {keepalive:true})`
      (or `navigator.sendBeacon`) so a same-tab navigation still records. The
      card shows the open count subtly; add a "Most visited / Recent" sort
      toggle.
- [ ] Recording a click **never blocks or delays** opening the site.
- [ ] Tests through the HTTP seam: click increments count and stamps
      `last_clicked_at` and returns `204`; `404` for a missing id;
      `?sort=clicks` orders by count desc with a recency tiebreak; default sort
      unchanged (creation order).

## Notes

- Keep the click endpoint cheap and side-effect-only; no body, no returned row.
- `sort=clicks` tiebreak: `ORDER BY click_count DESC, last_clicked_at DESC NULLS LAST,
  created_at ASC, id ASC` keeps ordering stable for never-clicked bookmarks.
- This is a running counter only — no per-click history table, no charts (out of
  scope per the spec).
