# 02: Disk by partition

Status: done

**What to build:** Report usage per mounted filesystem instead of only the root
`/`. Today `RealHost.Sample()` calls `disk.Usage("/")` and `Vitals` carries a
single `DiskTotal/DiskUsed` (`server/internal/host/real.go`,
`server/internal/host/host.go`). Add a per-partition breakdown on the current
view; keep the aggregate for the historical chart.

**Blocked by:** None

**Spec:** `.scratch/servers-enhancements/spec.md`

## Acceptance criteria

- [x] `Vitals` gains `Disks []DiskUsage` where
      `DiskUsage = { Path, Fstype string; Total, Used uint64; UsedPercent float64 }`.
- [x] `RealHost` enumerates mounts via `disk.Partitions(false)` + `disk.Usage(mountpoint)`,
      skipping entries that error or are clearly pseudo/special filesystems
      (small fstype allow/deny list). The legacy `DiskTotal/DiskUsed` (root `/`)
      is **kept** for backward compatibility and the historical chart.
- [x] `GET /api/servers/current` includes the `disks` array; the UI renders a
      per-partition list (mount point, type, used/total with a bar).
- [x] The `metric_samples` schema is **not** changed — per-partition values are a
      current snapshot only, no per-partition time series this increment.
- [x] `FakeHost` lets tests set canned `Disks`.
- [x] Tests (`internal/httpapi`): set canned `Disks`; assert `/servers/current`
      returns the per-partition array with correct fields and that legacy
      `disk_total/disk_used` still reflect root; assert an empty disks list
      returns/renders cleanly.

## Notes

- `UsedPercent` can come straight from gopsutil's `UsedPercent`; keep it so the
  UI doesn't recompute.
- Deduplicate mounts that point at the same device if gopsutil reports bind
  mounts twice; prefer the real mount point.
- Keep the existing disk % trend chart working off the aggregate root figure —
  per-partition is additive, not a replacement.

## Comments

- Implemented: `host.DiskUsage` and `Vitals.Disks`.
  `RealHost` enumerates `disk.Partitions(false)` → `disk.Usage` through a
  pure `filterDisks` (unit-tested). It skips a pseudo/virtual fstype deny-list
  (tmpfs, overlay, squashfs, proc, devfs, autofs, …), unreadable or zero-size
  mounts, and macOS `nobrowse` system volumes (they share the boot APFS
  container's numbers). It dedupes bind mounts by device, keeping the
  shortest mount point. On this Mac that leaves just `/`; on Linux, real data
  disks remain.
- `/servers/current` includes `disks` (always an array). Legacy
  `disk_total/disk_used` still come from `/` and still feed the history
  chart. `metric_samples` is unchanged (snapshot only).
- UI: a "Filesystems" card with mount, fstype, used/total, and a bar that
  turns amber at 75% and red at 90%. It is hidden when the list is empty.
