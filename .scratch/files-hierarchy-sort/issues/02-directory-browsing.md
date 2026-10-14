# 02 — Hierarchical Managed Root browsing

Status: done

Show direct directory children instead of flattened descendants. Include empty
directories, breadcrumbs, browser history, Missing ancestor navigation and
Managed Root confinement.

## Comments

- 2026-10-14: implemented through the parent-scoped resource page and root-backed
  Host directory reads. Tests cover nested, empty, Missing, wildcard/multibyte
  directory paths, multiple roots and symlink escapes. Browser tests and
  desktop/dark-mobile visual verification passed.
