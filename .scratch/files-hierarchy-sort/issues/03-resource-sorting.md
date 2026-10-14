# 03 — Resource Size and Modified sorting

Status: done

Add bidirectional Name/Size/Modified sorting before pagination, stable path
tie-breaks, null modification times last, folders first, accessible headers and
mobile controls. Ignore obsolete responses after rapid navigation/sort changes.

## Comments

- 2026-10-14: implemented and verified with SQL-backed paginated sorting tests
  and browser sort/reload/load-more/stale-response tests.
- Verification: `go test ./... -count=1` passed against isolated Postgres
  databases; `npm test -- --workers=4` passed all 21 browser tests;
  `npm run build` passed. Existing preview/editor tests remain green.
