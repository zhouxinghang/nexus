# 02 — Compact folder/type browsing interactions

Status: done

Separate browser modes, type grouping/pagination, URL-backed filters, Finder-style
selection/navigation and on-demand management/details/batch controls.

## Comments

- 2026-10-14: implemented. All 33 browser tests and the production build passed.
  Verified complete 1,208-Resource folder lists, type grouping, combination
  filters, offset pages/history, selection/range order, late-response guards,
  native dialogs and existing preview/edit behavior.
- 2026-10-14: visually checked folder/type modes at desktop and 375px dark mobile,
  plus landscape interaction. Fixed mobile toolbar overlap and selection-bar
  layout movement; file paths appear only in recursive type browsing.
