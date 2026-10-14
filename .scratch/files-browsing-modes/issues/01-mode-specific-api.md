# 01 — Unpaged folders and filtered Resource pages

Status: done

Add explicit mode contracts, complete direct-child folder responses and literal
name filtering in SQL. Preserve the recursive page cap and existing API clients.

## Comments

- 2026-10-14: implemented. New HTTP integration tests cover 1,205 direct
  Resources, directory exclusion, sorted results, paging limits, literal
  wildcard/CJK names, combined filters/counts and invalid mode parameters.
- 2026-10-14: full `go test ./... -count=1` passed, including the existing
  directory confinement, Missing Resource, scan, preview/edit and mutation tests.
