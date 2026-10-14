# 01 — Host disk I/O sampling and history

Status: done

Capture per-device disk counter deltas as read/write throughput and IOPS.
Handle unavailable baselines, device changes and resets without false spikes;
persist nullable fields and average them for display-time downsampling.

## Comments

- 2026-10-14: implemented with migration 18, the Host disk-I/O collector, sampler
  and current/raw/bucketed HTTP responses. Overview and filesystem capacity
  fields remain compatible.
- Verification: full `go test ./... -count=1` passed against isolated Postgres
  databases. Host tests passed with `-race`; `go vet ./...` passed. Two real
  Host disk-counter observations produced valid rates in the live smoke test.
