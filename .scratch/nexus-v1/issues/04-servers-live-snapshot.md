# 04: Servers — live snapshot

**What to build:** The Owner sees a live view of the Host's vitals. This ticket
introduces the `Host` port with its metrics and Docker capability groups: a real
implementation that reads the host (system metrics and the Docker socket) and an
injectable fake for tests. The dashboard shows current CPU, memory, disk,
network, load average, uptime, and the top processes by CPU and memory, plus the
state of Docker containers — degrading gracefully to "not enabled" when no
Docker is present. Values refresh so the view feels current. Scope is the single
Host running Nexus (ADR-0002); active control is out of scope.

**Blocked by:** 01 (walking skeleton + auth)

**Status:** ready-for-agent

- [ ] A `Host` port exposes a metrics capability (`Sample()` returning the vitals struct) and a Docker capability (list containers), with a real host implementation and an injectable fake.
- [ ] The UI shows current CPU usage, memory usage, disk usage, network throughput, load average, and uptime.
- [ ] The UI shows the top processes by CPU and by memory (top-N).
- [ ] The UI shows Docker container states (id/name/image/state) read from `/var/run/docker.sock`.
- [ ] When no Docker is present, the Docker panel shows "not enabled" and nothing errors.
- [ ] Live values refresh in the UI.
- [ ] Only the single Host running Nexus is monitored; no multi-host/agent anything.
- [ ] Tests (through the HTTP API seam, with a fake `Host` feeding canned vitals and a fake container list) assert: current values are served correctly and Docker-absent degrades to "not enabled".
