# 02 — Servers read/write statistics and curves

Status: done

Replace Disk capacity with a read/write throughput and IOPS card. Replace Disk %
with labelled read/write curves, differentiate line styles, preserve unavailable
gaps and real idle zero, and provide a keyboard-accessible data table.

## Comments

- 2026-10-14: implemented; Filesystems retains the current capacity display.
  Desktop, dark mobile, reduced-motion and landscape layouts were checked.
- Verification: all 27 browser tests passed, including 6 disk-I/O tests. The 6
  disk-I/O tests also passed after the final theme/axis polish.
  `npm run build` passed (existing bundle-size advisory remains).
