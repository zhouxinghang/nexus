# Nexus v1 Spec

Status: ready-for-agent

## Problem Statement

I run a personal host machine (my own server/workstation) and juggle four
recurring chores with scattered, disconnected tools: I lose track of where files
live on that host, I have no quick read on the machine's health, my to-dos live
in my head, and I have nowhere consistent to jot down what happened. I want one
private workbench, served from that same host, that gives me all four in one
place — without stitching together a filesystem browser, a monitoring dashboard,
a task app, and a notes app.

## Solution

Nexus: a single-user personal workbench. One native Go process runs on the host,
serves a React UI plus a JSON API, and exposes four tools over that host:

- **Files** — index and manage real files living on the host's filesystem.
- **Servers** — monitor the host's own vitals over time.
- **Tasks** — a three-column board of to-dos.
- **Journal** — an append-only plain-text log.

I log in with a single password, and from any browser (local or remote) I get a
live view of my host and my work. Only Postgres runs in a container; everything
else is the one Go binary.

## User Stories

### Auth & access

1. As the Owner, I want to log in with a single password, so that my workbench isn't exposed to anyone who finds the URL.
2. As the Owner, I want my session carried in a secure cookie, so that I stay logged in across page loads without re-entering the password.
3. As the Owner, I want my session to expire after about a week, so that an abandoned browser doesn't stay authenticated forever.
4. As the Owner, I want to be redirected to a login screen when my session is invalid or expired, so that I always know my auth state.
5. As the Owner, I want to log out explicitly, so that I can end my session on a shared or public machine.
6. As the Owner, I want every API call to require a valid session, so that no module is reachable unauthenticated.

### Files — configuration

7. As the Owner, I want to configure one or more Managed Roots (host directories), so that Nexus knows which parts of my filesystem to manage.
8. As the Owner, I want to remove a Managed Root, so that I can stop managing a directory I no longer care about.
9. As the Owner, I want Nexus to reject a Managed Root path that doesn't exist or isn't a directory, so that I don't misconfigure it silently.

### Files — scanning & indexing

10. As the Owner, I want Nexus to scan a Managed Root and index the files it finds as Resources, so that I can browse them in the UI.
11. As the Owner, I want to trigger a rescan on demand, so that I can pick up changes made outside Nexus.
12. As the Owner, I want a newly appeared file on disk to show up as a new Resource after a scan, so that the index reflects reality.
13. As the Owner, I want a file that changed on disk (size/mtime) to have its Resource metadata updated on scan, so that the index stays accurate.
14. As the Owner, I want a file deleted on disk by something other than Nexus to be marked Missing (not auto-removed), so that the tags and notes I attached to it survive.
15. As the Owner, I want Missing Resources shown greyed out, so that I can tell at a glance which files are gone.
16. As the Owner, I want to manually clear a Missing Resource, so that I can tidy the index once I accept the file is gone.
17. As the Owner, I want to manually register a single file by its path, so that I can index something without scanning a whole root.

### Files — browsing & metadata

18. As the Owner, I want to browse Resources by Managed Root and subdirectory, so that I can navigate my files.
19. As the Owner, I want to see each Resource's name, path, size, type, and modified time, so that I have the metadata I need.
20. As the Owner, I want to add tags to a Resource, so that I can organize files across directories.
21. As the Owner, I want to filter Resources by tag, so that I can find related files quickly.
22. As the Owner, I want to write a free-text note on a Resource, so that I can record context about a file.
23. As the Owner, I want to download or open a Resource's underlying file, so that I can use its contents.
24. As the Owner, I want my tags and notes stored by Nexus (not written into the file), so that the original file is never altered by metadata.

### Files — mutating the underlying file

25. As the Owner, I want to rename a Resource's underlying file on disk, so that I can fix file names from the UI.
26. As the Owner, I want to move a Resource's underlying file to another location, so that I can reorganize my filesystem.
27. As the Owner, I want to delete a Resource's underlying file, so that I can remove files I no longer need.
28. As the Owner, I want a rename/move whose target falls outside every Managed Root to be rejected, so that Nexus can't scatter files into unmanaged territory.
29. As the Owner, I want a delete to permanently remove the file (no trash), so that deletion is final and predictable.
30. As the Owner, I want every file mutation (rename/move/delete) recorded in an audit log, so that I can see what Nexus did to my filesystem.
31. As the Owner, I want tags and notes to follow a file through a rename or move, so that reorganizing doesn't lose my metadata.
32. As the Owner, I want deleting a file's underlying bytes to also remove or mark its Resource, so that the index doesn't keep a dangling entry.

### Servers — monitoring

33. As the Owner, I want to see current CPU usage of the host, so that I know how busy it is.
34. As the Owner, I want to see current memory usage, so that I know how much RAM is free.
35. As the Owner, I want to see current disk usage, so that I know how full my disks are.
36. As the Owner, I want to see current network throughput, so that I know how much traffic is flowing.
37. As the Owner, I want to see the system load average, so that I can judge sustained pressure.
38. As the Owner, I want to see the host's uptime, so that I know how long it's been running.
39. As the Owner, I want to see the top processes by CPU and memory, so that I can spot what's consuming resources.
40. As the Owner, I want to see the state of Docker containers on the host, so that I can tell what's running.
41. As the Owner, I want the Docker panel to show "not enabled" if no Docker is present, so that the absence of Docker doesn't break the view.
42. As the Owner, I want Nexus to sample these vitals every 30 seconds into history, so that I can see trends, not just the current instant.
43. As the Owner, I want to see trend charts over time for CPU, memory, disk, and network, so that I can spot patterns and spikes.
44. As the Owner, I want samples older than 30 days automatically removed, so that history doesn't grow without bound.
45. As the Owner, I want live values to refresh in the UI, so that the dashboard feels current.

### Tasks

46. As the Owner, I want to create a Task with a title, so that I can capture a to-do quickly.
47. As the Owner, I want each Task to have a status of Todo, Doing, or Done, so that I can track its progress.
48. As the Owner, I want a Kanban board with three fixed columns (Todo/Doing/Done), so that I can see my work at a glance.
49. As the Owner, I want to move a Task between columns, so that I can update its status.
50. As the Owner, I want to set a priority on a Task, so that I can tell which to-dos matter most.
51. As the Owner, I want to set a due date on a Task, so that I can track deadlines.
52. As the Owner, I want Tasks approaching or past their due date highlighted, so that I notice deadlines without a reminder system.
53. As the Owner, I want to group Tasks into projects/lists, so that I can separate different areas of work.
54. As the Owner, I want to edit a Task's details, so that I can correct or refine it.
55. As the Owner, I want to delete a Task, so that I can remove ones that no longer apply.

### Journal

56. As the Owner, I want to append a plain-text Journal Entry, so that I can quickly record something that happened.
57. As the Owner, I want each entry automatically stamped with its creation time, so that I don't have to timestamp manually.
58. As the Owner, I want entries shown newest-first as a stream, so that recent activity is front and center.
59. As the Owner, I want to full-text search my Journal, so that I can find a past entry by its words.
60. As the Owner, I want entries to be immutable — no editing, no deleting — so that the Journal stays a trustworthy record of what happened.

## Implementation Decisions

### Architecture & deployment

- **Native host process.** The backend is one native Go process running directly on the Host (not containerized), so it has first-class access to the filesystem and system metrics. See ADR-0001.
- **Single Host.** v1 monitors and manages exactly the one machine running Nexus. No multi-machine agents. See ADR-0002.
- **Postgres only in Docker.** A single-service `docker-compose.yml` runs Postgres. The Go binary connects to it.
- **Frontend embedded.** React + shadcn (built with Vite) is compiled and embedded into the Go binary via `go:embed`; the same binary serves the SPA and the JSON API. Deployment is one binary + one Postgres container.
- **Monorepo layout.** Single repo: `web/` (React), `server/` (Go), root `docker-compose.yml`.

### Backend stack

- HTTP routing via standard library `net/http` with `chi`.
- Database access via `sqlc` (type-safe Go generated from hand-written SQL); no ORM.
- Schema migrations via `golang-migrate` versioned `.sql` files.

### The Host port (key seam)

- All interaction with the live machine goes through a single `Host` port (interface), with three capability groups:
  - **Filesystem**: list/stat within a Managed Root, rename, move, delete.
  - **Metrics**: a `Sample()` returning the vitals struct (CPU, memory, disk, network, load, uptime, top-N processes).
  - **Docker**: list containers (id/name/image/state) by reading `/var/run/docker.sock`, or report unavailable.
- A real implementation runs on the host; a fake implementation is injected in tests.

### Auth

- Single password, hashed with bcrypt; the hash and the JWT signing key live in backend config/env, not the database.
- Login issues a JWT carried in an **HttpOnly, SameSite=Strict** cookie; no `refresh` token.
- Token lifetime ~7 days; on expiry the Owner re-logs-in.
- All API endpoints except login require a valid session.

### Files module

- A **Managed Root** is a configured host directory. All file operations are confined inside Managed Roots; a rename/move targeting a path outside every Managed Root is rejected.
- **Disk is the source of truth for existence.** On scan: new file → create Resource; vanished file → mark Resource `missing` (keep it, keep tags/notes, grey it out); changed file (size/mtime) → update metadata.
- **No external-URL resources.** Every Resource points to a real file on the Host. (The earlier "Link" concept was dropped.)
- **Delete is permanent** — a direct removal, no trash directory.
- Tags and notes are stored by Nexus in Postgres and keyed so they follow a file through rename/move; the original file is never modified by metadata.
- File mutations (rename/move/delete) are recorded in an audit log.

### Servers module

- Vitals are sampled every **30 seconds** and persisted as **Metric Samples**.
- Raw samples are retained for **30 days**, then auto-deleted. No downsampling/aggregation in v1.
- Process list stores only **top-N** by CPU/memory, not the full list.
- Docker container state read via the local Docker socket; absence degrades gracefully to "not enabled".
- Active control (start/stop/restart containers, run scripts, manage services) is **out of scope for v1**.

### Tasks module

- A **Task** has a fixed status enum: `todo` / `doing` / `done`, plus optional priority and due date, and belongs to an optional project/list.
- The board has three fixed columns; no custom columns.
- Due-date proximity/overdue is a **frontend visual highlight** only — no email/push reminders, no recurring tasks.

### Journal module

- A **Journal Entry** is append-only and immutable: plain text + automatic creation timestamp. No edit, no delete. See ADR-0003.
- No tags on entries.
- Full-text search over entry text (Postgres full-text search).
- Streamed newest-first.

### API contract (shape, not paths)

- JSON over HTTP, grouped by module (auth, files/roots, files/resources, servers/metrics, tasks, journal).
- List endpoints support the filters the user stories imply (Resources by root/subdirectory/tag; Journal by search query; Metrics by time range).
- Mutations return the updated resource and appropriate status codes; rejected operations (e.g. out-of-root move) return a clear client error.

## Testing Decisions

### What makes a good test here

- Test **external behavior through the HTTP API**, never internal function calls or private helpers. A test should read like a sequence of real client interactions and assertions on responses + persisted state.
- Prefer the **highest seam**: drive the real `chi` router via `httptest` against an **ephemeral Postgres**, authenticate to obtain the JWT cookie, then exercise each module end-to-end. This covers routing, auth, SQL, and business logic in one seam.

### The two seams

1. **HTTP API (dominant seam).** The vast majority of behavior is tested here:
   - Auth: login succeeds with the right password and fails otherwise; protected endpoints reject missing/expired/invalid sessions; logout invalidates.
   - Files: configure a Managed Root; scan and assert Resources appear; make an out-of-band change and assert new/updated/`missing` transitions; add tags/notes and assert they survive a rename/move; assert an out-of-root move is rejected; assert delete is permanent; assert the audit log records mutations.
   - Servers: with a fake Host feeding canned vitals, assert current values are served, samples accumulate, time-range queries return series, and the 30-day retention prune removes old samples; assert Docker-absent degrades to "not enabled".
   - Tasks: create, move across statuses, set priority/due date, group into a project, edit, delete.
   - Journal: append entries, assert newest-first ordering and automatic timestamps, assert edit/delete are not possible, assert full-text search finds entries by content.

2. **Host port (injected).** In API tests, inject a **fake `Host`**: an in-memory/temp-dir filesystem, canned metrics, and fake container list. Back the filesystem fake with a **real temp directory** so rename/move/delete and out-of-root rejection run against real syscalls. The **real** `Host` implementation (reading `/proc`-style metrics and the Docker socket) gets its own narrow integration test, kept separate and minimal.

### Implicitly covered

- `sqlc` queries, `chi` routing, JWT issuance/verification, and migrations are all exercised through the API seam rather than in isolation.

## Out of Scope

- Multi-user / multi-tenant: Nexus is strictly single-Owner.
- Multi-host monitoring and any agent-on-remote-machine architecture.
- External-URL / bookmark resources in the Files module.
- A trash/recycle bin for deleted files; deletes are permanent.
- Active host control: starting/stopping/restarting Docker containers, running registered scripts, managing systemd services, killing arbitrary processes.
- SSH-based server management.
- Journal editing/deleting, Journal tags.
- Recurring tasks, custom board columns, and reminders/notifications beyond frontend due-date highlighting.
- Metric downsampling/rollups; only raw 30-day retention.
- Refresh tokens / OAuth / third-party identity providers.

## Further Notes

- Vocabulary follows `CONTEXT.md`: Host, Owner, Resource, Managed Root, Missing, Metric Sample, Task, Journal Entry. Specs, issues, tests, and UI copy should use these terms and avoid their listed synonyms.
- Relevant ADRs: 0001 (native host process), 0002 (single-host scope), 0003 (append-only Journal). Any implementation that contradicts these should surface the conflict rather than silently diverging.
- Note that Postgres itself, running in Docker, will appear in the host's Docker container list — expected, not a bug.
