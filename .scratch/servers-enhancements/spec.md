# Servers Enhancements Spec

Status: ready-for-agent

## Problem Statement

The Servers module shows the Host's live vitals and trend charts, but three gaps
keep it behind comparable host monitors (Netdata, Glances, btop):

1. **Charts have no time range.** `web/src/pages/Servers.tsx` calls
   `GET /api/servers/samples` with no params and plots whatever comes back. The
   backend already accepts `?from=&to=` (defaulting to the last 24h), but the UI
   can't switch between "last hour / 24h / 7d / 30d", and at 30-day scale
   (~2 samples/min ≈ 86k points) a raw plot is unreadable and heavy.
2. **Disk is a single number.** `Vitals.DiskTotal/DiskUsed` come from
   `disk.Usage("/")` only (`server/internal/host/real.go`). A Host with several
   mounted filesystems (data disk, external drive, Docker volume mount) shows
   only the root filesystem, hiding where space is actually going.
3. **Processes and containers are dead-ends.** The top-process table and the
   Docker table list rows but nothing is clickable. There's no way to see a
   process's full command line / user / status, or a container's ports / image
   / created time / command.

## Solution

Enhance Servers on the existing architecture (single native Go process reading
the live machine through the `Host` port; chi + sqlc; recharts in the SPA;
ADR-0001/0002 unchanged — still read-only monitoring of exactly one Host):

- **Time range selector** — a UI control (1h / 24h / 7d / 30d) that drives the
  existing `?from=&to=` query, plus display-time downsampling so long ranges
  stay readable and light. This is render-time aggregation, **not** stored
  rollups (the 30-day raw retention is unchanged).
- **Disk by partition** — extend the metrics capability to report per-mount
  usage (path, fstype, total, used), surfaced on the current view as a
  per-partition breakdown. The aggregated root-filesystem figure stays for the
  historical charts.
- **Process / container details** — read-only detail endpoints and a drill-in UI
  (click a row → a detail panel) showing the extra fields gopsutil / the Docker
  socket already expose. Still monitoring-only: no kill/stop/restart.

Single-Owner, single-Host, no new runtime dependencies (gopsutil already
provides partition and richer process data; container detail uses the existing
Docker socket client).

## User Stories

### Time range selector

1. As the Owner, I want to switch the charts between the last hour, 24 hours, 7
   days, and 30 days, so that I can see both recent spikes and long trends.
2. As the Owner, I want long ranges to stay readable and fast, so that 30 days
   of data doesn't produce an unreadable, heavy chart.
3. As the Owner, I want the selected range to drive what the trend charts query,
   so that the window I pick is the window I see.
4. As the Owner, I want my chosen range to persist across refreshes, so that I
   don't reselect it every visit.

### Disk by partition

5. As the Owner, I want to see each mounted filesystem's usage (mount point,
   type, used / total), so that I know which disk is filling up.
6. As the Owner, I want a filesystem that is unreadable or special (pseudo/overlay)
   to be skipped gracefully, so that the panel shows real disks, not noise.
7. As the Owner, I want the existing disk % trend chart to keep working, so that
   per-partition detail is additive, not a regression.

### Process / container details

8. As the Owner, I want to click a top process and see its details (full command
   line, user, status, started time, thread count, CPU% and memory), so that I
   can understand what a heavy process is.
9. As the Owner, I want to click a container and see its details (full image,
   state/status, created time, command, published ports), so that I can tell
   what it's doing.
10. As the Owner, I want a detail lookup for a process/container that has since
    disappeared to fail gracefully (clear "no longer present"), so that a stale
    click doesn't error the page.

## Implementation Decisions

### Time range selector

- Frontend adds a range control with presets **1h / 24h / 7d / 30d**. It
  computes `from`/`to` and passes them to `GET /api/servers/samples` (already
  supported). The selected preset persists in `localStorage` (like the sidebar
  collapse state).
- **Downsampling for readability** is applied so a chart never renders tens of
  thousands of points. Preferred: a server-side optional `max_points`
  (or `bucket`) param on `/servers/samples` that averages rows into time buckets
  via SQL (e.g. `width_bucket` / `date_bin`), returning at most ~N points for
  the range; the handler falls back to raw rows when the param is absent (back-
  compatible). Document this as **query-time aggregation for display**, explicitly
  distinct from the spec's "no downsampling/rollups" storage decision — the
  stored samples and 30-day retention are untouched. Flag this tension in the
  ticket rather than silently diverging.
- The live-refresh behavior (current values every ~5s) is unchanged; only the
  historical charts gain the range.

### Disk by partition

- Extend the `Metrics` capability: `Vitals` gains a `Disks []DiskUsage` where
  `DiskUsage = { Path, Fstype string; Total, Used uint64; UsedPercent float64 }`.
  The real host enumerates mounts via `disk.Partitions(false)` and `disk.Usage(mountpoint)`,
  skipping entries that error or are clearly pseudo filesystems (keep a small
  allowlist/denylist of fstypes). The existing `DiskTotal/DiskUsed` (root `/`)
  is kept for backward compatibility and the historical chart.
- `GET /api/servers/current` includes the `disks` array; the UI renders a
  per-partition list (mount, type, a used/total bar).
- **History stays aggregate.** Per-partition values are a *current* snapshot
  only — the `metric_samples` schema (single `disk_total/disk_used`) is **not**
  changed, so there's no per-partition time series. Note this scope line in the
  ticket; a later increment can add per-disk history if wanted.
- The `FakeHost` lets tests set canned `Disks`.

### Process / container details

- Extend the `Host` port with read-only detail methods:
  - `ProcessDetail(pid int32) (ProcessDetail, error)` → full command line, user,
    status, create time, num threads, CPU%, memory (RSS + percent), plus the
    basics. Real impl via gopsutil `process.NewProcess(pid)`; `ErrNotFound`-style
    error when the pid is gone.
  - `ContainerDetail(id string) (ContainerDetail, error)` → full image, state +
    status string, created time, command, published ports, maybe mounts. Real
    impl via the existing Docker-socket client (`GET /containers/{id}/json`);
    graceful error when Docker is off or the id is gone.
- Endpoints (authenticated): `GET /api/servers/processes/{pid}` and
  `GET /api/servers/containers/{id}`. A missing/gone target returns `404` with
  the standard `{"error": "..."}` body.
- Frontend: clicking a top-process row or a container row opens a detail panel
  (reuse the existing `Modal`) that fetches and shows the detail; a gone target
  shows a clean "no longer present" state.
- **Monitoring-only.** No actions on processes/containers (no kill, stop,
  restart, exec) — consistent with ADR-0002's monitoring-not-controlling posture
  and the v1 "active host control out of scope" line.

### API contract (shape, not paths)

- New/changed endpoints stay under the authenticated `/api` group with the
  existing JSON error convention and cookie session.
- `/servers/current` response gains `disks` (additive); `/servers/samples`
  gains an optional downsampling param (additive, back-compatible); two new
  read-only detail endpoints are added.

## Testing Decisions

Same seams as the rest of Servers: drive the real chi router via `httptest`
against ephemeral Postgres, authenticate, and inject the `FakeHost` for canned
host state.

- **Time range / downsampling:** seed samples across a span; assert `?from=&to=`
  returns only in-range rows (already behavior) and that the downsampling param
  caps the returned point count while preserving range coverage and ordering;
  assert absence of the param returns raw rows (back-compat).
- **Disk by partition:** set canned `Disks` on the FakeHost; assert
  `/servers/current` returns the per-partition array with correct fields, and
  that the legacy `disk_total/disk_used` still reflect root; assert an empty
  disks list renders/returns cleanly.
- **Process / container details:** with canned detail data on the FakeHost,
  assert `/servers/processes/{pid}` and `/servers/containers/{id}` return the
  rich fields; assert a missing pid/id returns `404`; assert container detail
  with Docker disabled degrades to a clear error, not a panic.

## Out of Scope

- Any active control of processes or containers (kill/stop/restart/exec) — still
  monitoring-only (ADR-0002).
- Per-partition or per-interface **historical** time series / schema changes —
  current snapshot only this increment.
- Stored downsampling / rollups / retention changes — the 30-day raw retention
  is unchanged; downsampling here is render-time only.
- Network-per-interface breakdown (only disk-per-partition was requested).
- Multi-host (ADR-0002) — exactly one Host.

## Further Notes

- Vocabulary follows `CONTEXT.md`: Host, Metric Sample. "Partition / mount",
  "process detail", and "container detail" are UI/implementation terms, not new
  domain nouns, so no glossary change is required; if "per-partition disk usage"
  should be fixed as a term, add it to `CONTEXT.md` and flag it.
- Relevant ADRs: 0001 (native host process — partition enumeration and process
  detail need first-class host access, which the native process has), 0002
  (single Host, monitoring not controlling — all additions are read-only).
- Tickets: `issues/01-time-range-selector.md`, `issues/02-disk-by-partition.md`,
  `issues/03-process-container-details.md`. All three are independent and can be
  done in any order.
