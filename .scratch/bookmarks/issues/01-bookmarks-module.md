# Bookmarks module

Status: done

## Problem

The four v1 tools (Files, Servers, Tasks, Journal) cover local files, host
vitals, to-dos, and notes, but there's nowhere to collect the external websites
the Owner visits often. Files is deliberately local-only (no external URLs, see
CONTEXT.md / ADR-0005), so a navigation board of links needs its own home.

## What to build

A **Bookmarks** tool: a navigation board of named, clickable links to external
sites, each with an optional icon, plus a periodic health check that records
whether each site is reachable.

- The Owner can add a Bookmark with a **name**, a **link** (http(s) URL), and an
  optional **icon** (emoji, image URL, or a favicon fallback from the link host).
- Clicking a Bookmark opens its site in a new tab.
- The Owner can edit and delete a Bookmark.
- Nexus **periodically health-checks** each Bookmark (every 5 minutes) and shows
  `up` / `down` / `unknown`, with last status code, latency, and check time.
- The Owner can force a recheck of one Bookmark or of all of them.
- A bare host like `example.com` is normalized to `https://example.com`; only
  http(s) links are accepted.

## How it was built

- **DB:** migration `000008_bookmarks` adds a `bookmarks` table and a
  `bookmark_health` enum (`unknown`/`up`/`down`), with `last_status`,
  `last_latency_ms`, `last_checked_at`. Queries in
  `internal/db/queries/bookmarks.sql` (sqlc-generated).
- **Checker seam:** `internal/checker` defines a `Prober` port
  (`HTTPProber` in production, `FakeProber` in tests) and a background `Checker`
  that mirrors the Servers sampler — a 5-minute ticker launched from `main`,
  plus `CheckOne`/`CheckAll` for on-demand checks. A probe with HTTP status
  `< 500` is `up`; a transport failure or 5xx is `down`.
- **API** (all under the authenticated `/api`):
  - `POST /api/bookmarks` — create (kicks off an immediate background check)
  - `GET /api/bookmarks` — list, creation order
  - `PATCH /api/bookmarks/{id}` — edit name/url/icon
  - `DELETE /api/bookmarks/{id}` — delete
  - `POST /api/bookmarks/{id}/check` — recheck one, returns the updated row
  - `POST /api/bookmarks/check` — recheck all, returns the refreshed list
- **UI:** `web/src/pages/Bookmarks.tsx` — a card grid with icon, name, host,
  health dot, click-to-open, per-card recheck/edit, and a "Check all" action.
  Nav entry added in `Shell.tsx`, route in `App.tsx`.
- **Tests:** `internal/httpapi/bookmarks_test.go` drives everything through the
  HTTP seam with the injected `FakeProber`: create (name/icon/link), URL
  normalization, validation (empty name, non-http(s) URL), list ordering,
  edit/delete, up/down health recording, manual recheck transitions, check-all,
  and a 404 recheck.

## Comments

- Scoped as its own module rather than a Resource "kind" to preserve the Files
  local-only invariant (ADR-0005).
- Health is monitoring-only; no active control of the remote sites (consistent
  with ADR-0002's monitoring-not-controlling posture).
