# 05: Servers — sampling history & trends

**What to build:** The Owner sees trends, not just the current instant. A
background sampler captures the Host's vitals every 30 seconds and persists them
as Metric Samples. The UI shows trend charts over time for CPU, memory, disk,
and network, backed by time-range queries. Samples older than 30 days are
automatically pruned so history stays bounded. No downsampling/rollups in v1 —
raw samples only.

**Blocked by:** 04 (servers — live snapshot)

**Status:** done

- [x] A background sampler captures the Host's vitals every 30 seconds and persists them as Metric Samples.
- [x] The process list stored per sample is limited to top-N by CPU/memory, not the full list.
- [x] Time-range queries return a series of Metric Samples.
- [x] The UI shows trend charts over time for CPU, memory, disk, and network.
- [x] Metric Samples older than 30 days are automatically removed.
- [x] No downsampling or aggregation — only raw 30-day retention.
- [x] Tests (through the HTTP API seam, with a fake `Host`) assert: samples accumulate, time-range queries return the expected series, and the 30-day retention prune removes old samples.

## Comments

- `internal/sampler` runs on a 30s ticker (`Interval`), persisting a `metric_samples` row per tick (vitals + top-N processes + containers as JSONB) and pruning anything older than 30 days (`Retention`). Launched in a goroutine from `main`. No downsampling — raw rows only.
- API: `GET /api/servers/samples?from=&to=` (RFC3339, defaults to last 24h) returns the raw series. UI charts (CPU/Memory/Disk/Network) in `web/src/pages/Servers.tsx` via Recharts, refreshing every 30s.
- Tests in `internal/httpapi/sampler_test.go` drive sampling synchronously through exported test hooks (`CaptureSampleForTest`, `PruneSamplesForTest`, `InsertAgedSampleForTest`) and assert accumulation, time-range filtering, and that a 40-day-old sample is pruned while a fresh one survives.
