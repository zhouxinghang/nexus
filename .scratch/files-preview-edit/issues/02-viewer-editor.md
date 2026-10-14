# 02 — Resource preview dialog and text editor

Status: done

Implement a responsive type-aware viewer using the current design tokens,
explicit Preview/Edit modes, lazily bundled CodeMirror, zoom/full-window
controls, adjacent-file navigation, save feedback, draft protection and
conflict recovery.

## Verification

Playwright tests use mocked API responses and real Chromium to cover entry
points, readonly editing, shortcut saves, BOM/CRLF, empty content, in-flight
locks, discard/close/navigation guards, conflicts and draft download, load
failures, unavailable/missing files, image/media controls, focus cycling,
Escape/return focus, small screen, dark theme and reduced motion.

## Comments

- 2026-10-14: all 13 Chromium interaction tests passed. Desktop (1440×1000),
  dark mobile (375×812), and landscape (812×375) screenshots were visually
  inspected. Syntax highlighting, source contrast, focus behavior and wrapping
  were verified. Save retains undo history; BOM/CRLF and empty-file saves pass.
- `npm run build` passed. Vite still reports a main-bundle size warning;
  the viewer, editor and language modules are independently lazy-loaded.
- Browser tests exercise UI behavior with mock responses, not actual native
  media codec/PDF rendering or real disk mutations; the HTTP tests exercise
  actual filesystem writes, streaming headers and Postgres metadata.
