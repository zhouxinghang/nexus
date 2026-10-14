# Workbench Polish Spec (post-v1 increment)

Status: done

## Problem Statement

Nexus v1 shipped the five tools (Files, Servers, Tasks, Journal, Bookmarks),
but three gaps keep it from being comfortable to run as a real, remotely-reached
personal workbench:

1. **Login is a brute-force target.** `POST /api/auth/login` verifies the single
   Owner password against a bcrypt hash with no failure throttling, and the
   session cookie is never marked `Secure`. The spec explicitly expects access
   "from any browser (local or remote)", so an exposed URL invites unlimited
   password guessing, and over plain HTTP the session token travels in the
   clear.
2. **List endpoints return everything.** `ListResources`, `ListJournalEntries`,
   and the tag path (`ListResourceIDsByTag` → load-all → filter in Go) have no
   `LIMIT`. A busy Managed Root or a long-lived Journal makes every page load
   fetch the whole table and render it in one DOM pass. This degrades silently
   as the Owner actually uses the tool.
3. **There is no front door.** The root route redirects straight to `/files`;
   the five tools are five islands. There's no single place that answers "is my
   Host healthy, what's due, what's broken, what did I just note?"

## Solution

A focused "polish" increment, built on the existing architecture (single native
Go process, chi + sqlc + golang-migrate, embedded React SPA; ADR-0001..0005
unchanged):

- **Login hardening** — per-client failure throttling with lockout, plus a
  configurable `Secure` cookie flag and correct client-IP resolution behind a
  reverse proxy.
- **Pagination** — a consistent page envelope for the Journal stream and the
  Files resource list (and its tag-filtered path), keyset where the ordering is
  stable, offset where filter combinatorics make keyset awkward.
- **Overview (home Dashboard)** — a new landing page backed by one aggregation
  endpoint that summarizes Host vitals, Tasks (overdue/due-today/counts), the
  latest Journal entries, Bookmark health, and Files totals, with purely visual
  threshold flags (no notification system).

Everything stays single-Owner, single-Host, and local-first. No new runtime
dependencies beyond what the repo already uses.

## User Stories

### Login hardening

1. As the Owner, I want repeated wrong-password attempts from one client to be
   throttled, so that an exposed URL can't be brute-forced.
2. As the Owner, I want a clear "too many attempts, try again later" response
   (HTTP 429) rather than a silent failure, so that I understand why login is
   refused.
3. As the Owner, I want a correct password to still work the moment the lockout
   window elapses, so that throttling never permanently locks me out.
4. As the Owner, I want my session cookie marked `Secure` when I serve Nexus
   over HTTPS, so that the token is never sent over plain HTTP — while still
   being able to disable that for local `http://localhost` development.
5. As the Owner running behind a reverse proxy, I want throttling keyed on my
   real client IP (from a trusted forwarded header), so that one proxy address
   isn't treated as a single global bucket.

### Pagination

6. As the Owner, I want the Journal to load newest entries in pages and let me
   fetch older ones on demand, so that years of entries don't all load at once.
7. As the Owner, I want Journal search results paginated the same way, so that a
   common term doesn't dump the whole matching set.
8. As the Owner, I want the Files resource list to load in pages (within the
   current root / tag / directory filters), so that a large Managed Root stays
   responsive.
9. As the Owner, I want each paged response to tell me whether more remains, so
   that the UI can show a correct "load more" / "end of list" state.

### Overview (home Dashboard)

10. As the Owner, I want Nexus to open on an Overview page, so that I get the
    state of my workbench at a glance instead of landing in one tool.
11. As the Owner, I want the Overview to show current Host vitals (CPU, memory,
    disk, load, uptime) with the over-threshold ones visually flagged, so that I
    spot pressure without opening Servers.
12. As the Owner, I want the Overview to show how many Tasks are overdue, due
    today, and open per column, so that I know what needs attention.
13. As the Owner, I want the Overview to show my most recent Journal entries, so
    that recent context is in front of me.
14. As the Owner, I want the Overview to list any Bookmarks currently `down`,
    so that I notice a site I rely on is unreachable.
15. As the Owner, I want the Overview to show Files totals including how many
    Resources are `Missing`, so that I know the index needs tidying.
16. As the Owner, I want each Overview panel to link into its tool, so that the
    home page is a launchpad, not a dead end.

## Implementation Decisions

### Login hardening

- A small in-memory limiter in `internal/auth` (or a sibling package) tracks
  recent failures per client key with an injectable clock (mirroring the
  existing `Authenticator.now` seam) so tests stay deterministic. State is
  process-local and non-persistent — acceptable for a single-process,
  single-Owner app; a restart clears it.
- Policy (all env-configurable, with sane defaults): after N consecutive
  failures within a window, return `429 Too Many Requests` with a
  `Retry-After` header until a cooldown elapses. A successful login clears the
  client's failure count.
- Client key = resolved client IP. Resolution trusts `X-Forwarded-For` /
  `X-Real-IP` **only** when a `NEXUS_TRUSTED_PROXY` toggle is set; otherwise it
  uses `RemoteAddr`. This avoids header-spoofed throttle evasion by default.
- `SetSessionCookie` / `ClearSessionCookie` gain a `Secure` flag sourced from
  config (`NEXUS_COOKIE_SECURE`, default `true`; set `false` for local HTTP
  dev). `HttpOnly` and `SameSite=Strict` are unchanged.

### Pagination

- A shared JSON envelope: list endpoints return `{ "items": [...],
  "next_cursor": "<opaque|null>" }` for keyset endpoints, or `{ "items": [...],
  "total": <n>, "limit": <n>, "offset": <n> }` for offset endpoints. Pick one
  shape per endpoint and document it; don't mix within an endpoint.
- **Journal** uses **keyset** pagination on `(created_at DESC, id DESC)` — the
  stream is append-only and newest-first, so a `(created_at,id)` cursor is
  stable and never skips/dupes across appends. `limit` defaults to a sensible
  page size and is capped. Both list and search take the same `limit` + cursor.
- **Files** uses **offset + limit with a total count**. The resource list is
  ordered by `path ASC` but is sliced by several optional filters (root,
  directory prefix, tag), so offset keeps the first pass simple. The tag path
  must stop loading the whole table into Go and filtering: push the tag filter
  and the limit/offset into SQL (join `resource_tags`), returning both the page
  and a `COUNT(*)` for the same filter set.
- Defaults are backward compatible for callers that pass no paging params
  (serve the first page), but the response shape changes — update the few
  existing callers/tests accordingly rather than branching on a query flag.

### Overview (home Dashboard)

- One aggregation endpoint, `GET /api/overview`, composes the summary server-side
  so the page is one round trip, not five. It reuses the live `Host.Sample()` +
  `Host.Status()` for vitals and adds dedicated **count/summary SQL** (tasks
  counts by status + overdue/due-today using the DB clock; bookmark counts by
  health + the `down` list; resource total + missing count) rather than
  re-listing full tables.
- Threshold flags (e.g. CPU/mem/disk ≥ 90%) are computed and surfaced as booleans
  in the response (or applied in the UI from the raw numbers); they are **purely
  visual**. No emails, push, or any notification mechanism — consistent with the
  v1 "frontend highlight only" posture.
- Frontend: a new `Overview` page becomes the index route (replacing the
  `/files` redirect) with a nav entry in `Shell.tsx`; each panel deep-links into
  its tool. The term for this page is **Overview** (see Further Notes).

### API contract (shape, not paths)

- New/changed endpoints stay under the authenticated `/api` group and keep the
  existing JSON error convention (`{"error": "..."}`), status codes, and cookie
  session.
- Paginated endpoints accept `limit` and either `cursor` (Journal) or `offset`
  (Files); invalid paging params return `400`.

## Testing Decisions

Same two seams as v1: drive the real chi router via `httptest` against an
ephemeral Postgres, authenticate for the cookie, and inject the fake `Host` /
fake `Prober` where live state is involved.

- **Login hardening:** with an injectable clock, assert N wrong attempts yield
  `429` with `Retry-After`; a correct password before the threshold still
  succeeds; after the cooldown elapses the same client logs in; a successful
  login resets the counter; the `Secure` flag toggles with config; forwarded
  headers are honored only when the trusted-proxy toggle is on.
- **Pagination:** seed more than one page of Journal entries and Resources;
  assert the first page respects `limit`, the cursor/offset fetches the next
  page with no gaps or dupes, `next_cursor`/`total` is correct, and the final
  page signals "no more". Assert the tag-filtered Files page is counted and
  sliced in SQL (correct `total` for the filter).
- **Overview:** seed tasks (overdue/due-today/various statuses), bookmarks
  (up/down/unknown via the fake Prober), journal entries, and resources
  (present/missing); feed canned vitals through the fake Host; assert the single
  `GET /api/overview` returns correct counts, the `down` bookmark list, latest
  journal entries, missing-resource count, and the threshold flags.

## Out of Scope

- Persistent/distributed rate limiting, CAPTCHA, account lockout emails, or any
  multi-user auth. Single-Owner, in-memory throttling only.
- Full-text relevance ranking or search-result scoring in the Journal (still
  trigram substring; see ADR-0004).
- Any notification/alerting transport for Overview thresholds (no email, push,
  webhooks) — visual flags only.
- Configurable Overview layout/widgets; a fixed set of panels in this increment.
- Changing the Files local-only invariant, the append-only Journal, or the
  single-Host monitoring posture (ADR-0002..0005 unchanged).

## Further Notes

- Vocabulary follows `CONTEXT.md`: Host, Owner, Resource, Managed Root, Missing,
  Metric Sample, Task, Journal Entry, Bookmark, Health. The home page introduces
  one new UI term, **Overview** (the landing summary of the whole workbench).
  If this term should be fixed in the glossary, add it to `CONTEXT.md`; until
  then prefer "Overview" and avoid "home", "dashboard" (as a stored term), and
  "index" in copy. Flag the conflict rather than silently diverging.
- Relevant ADRs: 0001 (native host process — the limiter and client-IP logic run
  in-process), 0002 (single Host — Overview summarizes exactly one machine).
  Consider a short ADR for "in-memory, non-persistent login throttling" since it
  is a deliberate durability trade-off.
- Tickets: `issues/01-login-rate-limiting.md`, `issues/02-list-pagination.md`,
  `issues/03-home-overview.md`. They are independent and can be done in any
  order; 03 benefits from 02's envelope but does not require it.
