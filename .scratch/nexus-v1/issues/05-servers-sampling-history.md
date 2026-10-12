# 05: Servers — sampling history & trends

**What to build:** The Owner sees trends, not just the current instant. A
background sampler captures the Host's vitals every 30 seconds and persists them
as Metric Samples. The UI shows trend charts over time for CPU, memory, disk,
and network, backed by time-range queries. Samples older than 30 days are
automatically pruned so history stays bounded. No downsampling/rollups in v1 —
raw samples only.

**Blocked by:** 04 (servers — live snapshot)

**Status:** ready-for-agent

- [ ] A background sampler captures the Host's vitals every 30 seconds and persists them as Metric Samples.
- [ ] The process list stored per sample is limited to top-N by CPU/memory, not the full list.
- [ ] Time-range queries return a series of Metric Samples.
- [ ] The UI shows trend charts over time for CPU, memory, disk, and network.
- [ ] Metric Samples older than 30 days are automatically removed.
- [ ] No downsampling or aggregation — only raw 30-day retention.
- [ ] Tests (through the HTTP API seam, with a fake `Host`) assert: samples accumulate, time-range queries return the expected series, and the 30-day retention prune removes old samples.
