# 03: Home Overview page

Status: ready-for-agent

**What to build:** Give Nexus a front door. Today the index route redirects to
`/files` (`web/src/App.tsx`) and the five tools are disconnected. Add an
**Overview** landing page backed by a single aggregation endpoint that
summarizes Host vitals, Tasks, Journal, Bookmarks, and Files, with purely visual
threshold flags, and deep-links into each tool.

**Blocked by:** None (benefits from `02-list-pagination`'s envelope but does not
require it)

**Spec:** `.scratch/workbench-polish/spec.md`

## Acceptance criteria

### Backend

- [ ] `GET /api/overview` (authenticated) returns one composed summary in a
      single round trip:
      - **Host vitals**: current CPU %, memory used/total, disk used/total,
        load, uptime — from the live `Host.Sample()` (+ `Host.Status()` if
        Docker count is wanted). Include boolean **threshold flags** (e.g.
        CPU/mem/disk ≥ 90%).
      - **Tasks**: counts per status (todo/doing/done), plus **overdue** and
        **due-today** counts computed with the DB clock.
      - **Journal**: the latest N entries (id, body, created_at).
      - **Bookmarks**: counts by health (up/down/unknown) and the list of
        currently **`down`** bookmarks (id, name, url).
      - **Files**: total resource count and **missing** count.
- [ ] The summary uses dedicated **count/summary SQL** (new sqlc queries), not
      re-listing full tables, so it stays cheap as data grows.
- [ ] Threshold flags are **visual only** — no email/push/webhook of any kind
      (consistent with the v1 "frontend highlight only" posture).

### Frontend

- [ ] A new `Overview` page (`web/src/pages/Overview.tsx`) becomes the **index
      route**, replacing the `/files` redirect; a nav entry is added at the top
      of `Shell.tsx`'s `nav` array; `LoginRoute` post-login redirect points at
      Overview.
- [ ] Panels: Host vitals (over-threshold values flagged red), Tasks summary
      (overdue/due-today/open), latest Journal entries, down Bookmarks, Files
      totals (with Missing called out).
- [ ] **Each panel deep-links into its tool** (e.g. vitals → `/servers`, tasks →
      `/tasks`, etc.) so Overview is a launchpad.
- [ ] Empty/zero states render cleanly (no tasks, no down bookmarks, etc.).

### Tests

- [ ] `internal/httpapi` test seeds tasks (overdue / due-today / various
      statuses), bookmarks (up/down/unknown via the fake `Prober`), journal
      entries, and resources (present/missing); feeds canned vitals through the
      fake `Host`; asserts `GET /api/overview` returns correct counts, the
      `down` bookmark list, latest journal entries, missing-resource count, and
      the threshold flags.

## Notes

- Terminology: the page is **Overview**. Avoid "home"/"dashboard"/"index" as
  stored terms. If this should be canonical, add **Overview** to `CONTEXT.md`;
  otherwise flag the term rather than silently diverging.
- Vitals come from the live `Host` sample (same source as `GET /api/servers/current`),
  so Overview reflects "now", not the last persisted Metric Sample. That's fine
  and avoids a dependency on the sampler having run.
- Keep the composed response flat and typed; prefer one handler that fans out to
  small query helpers over a mega-query.
