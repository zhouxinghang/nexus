# 03: Process / container details

Status: done

**What to build:** Make the top-process rows and Docker rows clickable, opening a
read-only detail panel. Today both tables in `web/src/pages/Servers.tsx` are
static. Add detail endpoints backed by richer `Host` port methods and a drill-in
UI. Monitoring-only — no actions on processes/containers.

**Blocked by:** None

**Spec:** `.scratch/servers-enhancements/spec.md`

## Acceptance criteria

- [x] The `Host` port gains read-only detail methods:
      - `ProcessDetail(pid int32) (ProcessDetail, error)` — full command line,
        user, status, create time, num threads, CPU%, memory (RSS + percent),
        name/pid. Real impl via gopsutil `process.NewProcess(pid)`; a distinct
        not-found error when the pid is gone.
      - `ContainerDetail(id string) (ContainerDetail, error)` — full image,
        state + status string, created time, command, published ports (and
        optionally mounts). Real impl via the existing Docker-socket client
        (`GET /containers/{id}/json`); graceful error when Docker is off or the
        id is gone.
- [x] Endpoints (authenticated): `GET /api/servers/processes/{pid}` and
      `GET /api/servers/containers/{id}`. A missing/gone target returns `404`
      with the standard `{"error": "..."}` body; Docker-disabled container
      detail returns a clear error, not a panic.
- [x] Frontend: clicking a top-process row or a container row opens a detail
      panel (reuse the `Modal`) that fetches and shows the detail; a gone target
      shows a clean "no longer present" state.
- [x] `FakeHost` lets tests set canned process/container detail (incl. a
      not-found path and a Docker-disabled path).
- [x] **No** process/container actions (kill/stop/restart/exec) are added.
- [x] Tests (`internal/httpapi`): with canned detail data assert both endpoints
      return the rich fields; a missing pid/id returns `404`; container detail
      with Docker disabled degrades to a clear error.

## Notes

- These are read-only, per-ADR-0002's monitoring-not-controlling posture and the
  v1 "active host control out of scope" line.
- Extend `FakeHost` to carry maps of canned detail keyed by pid/id so tests stay
  deterministic and no live process/socket is touched.
- Keep the detail payloads flat and typed; only include fields the real sources
  (gopsutil / Docker socket) actually expose.

## Comments

- Implemented: `Metrics.ProcessDetail(pid)` and `Docker.ContainerDetail(id)`
  on the Host port, with `host.ErrNotFound` / `host.ErrDockerUnavailable`.
  - Real process detail uses gopsutil (ppid, name, exe, cmdline, user,
    status, create time, threads, CPU%, RSS + %). Fields the OS hides are left
    empty instead of failing.
  - Real container detail uses `GET /containers/{id}/json` (image, state,
    created, started, command = Path + Args, published ports, mounts), plus the
    list endpoint filtered by id for docker-ps's human "Up 2 hours" status.
    Container refs are validated against `[A-Za-z0-9][A-Za-z0-9_.-]*` so
    they can't inject path segments into the socket URL.
- `GET /api/servers/processes/{pid}` → 200 / 404 (gone) / 400 (bad pid).
  `GET /api/servers/containers/{id}` → 200 / 404 (gone) / **503** (Docker
  unavailable, with a clear message).
- `FakeHost` has `SetProcessDetail` / `SetContainerDetail`. Container
  lookups return `ErrDockerUnavailable` while `Docker.Enabled` is false.
- UI: process and container rows are clickable and keyboard-focusable, and
  open a read-only `Modal`. A 404 shows "no longer running / present".
  No kill/stop/restart/exec was added.
