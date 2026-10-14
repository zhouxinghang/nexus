# 01 — Tags-only Resource metadata

Status: done

Remove note controls, API fields and write routes, generated queries and the
database column. Preserve tags through scans, rename, move and text edits.

## Comments

- 2026-10-14: implemented with migration 17. Migration-survival and HTTP tests
  verify that tags and Resource identity remain unchanged and notes disappear.
  Go full-suite tests and browser metadata/preview tests passed.
