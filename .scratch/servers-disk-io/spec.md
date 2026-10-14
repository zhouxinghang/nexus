# Servers — Disk I/O statistics

Confirmed request: 2026-10-14 (Asia/Shanghai)

## Acceptance

- Replace Servers' Disk capacity stat and Disk % chart with Host-wide read/write
  throughput (bytes/sec) and read/write operations/sec (IOPS).
- Keep filesystem capacity under Filesystems and preserve Overview/API capacity
  fields for compatibility.
- Compute rates from per-device counter deltas and elapsed time. Do not count
  Linux partitions, stacked device-mapper/RAID layers or memory/loop devices twice.
- First reading, errors, unknown baselines and counter resets are unavailable
  (`null`); measured idle storage is a real zero.
- Persist both throughput and IOPS, average available samples for display-time
  downsampling, and leave pre-migration samples null instead of fabricating zeros.
- Distinguish Read/Write curves with labels and line styles; support an
  accessible data table and desktop/mobile layouts.

## Verification

- Host tests for deltas, multi-device aggregation, appearance/disappearance,
  resets/replacement, failures/recovery, idle zero and Linux device selection.
- Live Host smoke test for two real disk-counter observations.
- HTTP seam tests for live stats, persistence, NULL vs zero, downsampling and
  migration survival.
- Browser tests for the read/write stat, curves, data table, polling, unavailable
  and idle states, capacity preservation, range changes and responsive UI.
