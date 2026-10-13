# 03: Process / container details

Status: ready-for-agent

**What to build:** Make the top-process rows and Docker rows clickable, opening a
read-only detail panel. Today both tables in `web/src/pages/Servers.tsx` are
static. Add detail endpoints backed by richer `Host` port methods and a drill-in
UI. Monitoring-only — no actions on processes/containers.

**Blocked by:** None

**Spec:** `.scratch/servers-enhancements/spec.md`

## Acceptance criteria

- [ ] The `Host` port gains read-only detail methods:
      - `ProcessDetail(pid int32) (ProcessDetail, error)` — full command line,
        user, status, create time, num threads, CPU%, memory (RSS + percent),
        name/pid. Real impl via gopsutil `process.NewProcess(pid)`; a distinct
        not-found error when the pid is gone.
      - `ContainerDetail(id string) (ContainerDetail, error)` — full image,
        state + status string, created time, command, published ports (and
        optionally mounts). Real impl via the existing Docker-socket client
        (`GET /containers/{id}/json`); graceful error when Docker is off or the
        id is gone.
- [ ] Endpoints (authenticated): `GET /api/servers/processes/{pid}` and
      `GET /api/servers/containers/{id}`. A missing/gone target returns `404`
      with the standard `{"error": "..."}` body; Docker-disabled container
      detail returns a clear error, not a panic.
- [ ] Frontend: clicking a top-process row or a container row opens a detail
      panel (reuse the `Modal`) that fetches and shows the detail; a gone target
      shows a clean "no longer present" state.
- [ ] `FakeHost` lets tests set canned process/container detail (incl. a
      not-found path and a Docker-disabled path).
- [ ] **No** process/container actions (kill/stop/restart/exec) are added.
- [ ] Tests (`internal/httpapi`): with canned detail data assert both endpoints
      return the rich fields; a missing pid/id returns `404`; container detail
      with Docker disabled degrades to a clear error.

## Notes

- These are read-only, per-ADR-0002's monitoring-not-controlling posture and the
  v1 "active host control out of scope" line.
- Extend `FakeHost` to carry maps of canned detail keyed by pid/id so tests stay
  deterministic and no live process/socket is touched.
- Keep the detail payloads flat and typed; only include fields the real sources
  (gopsutil / Docker socket) actually expose.
