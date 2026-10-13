# UI Toast Spec

Status: ready-for-agent

## Problem Statement

The SPA has no unified way to tell the Owner that an action succeeded or failed.
Feedback is scattered and inconsistent:

- **Files** keeps a local `error` string and renders it in one spot
  (`web/src/pages/Files.tsx`), and uses the browser's blocking `confirm()` for
  deletes.
- **Login** has its own inline `error` paragraph.
- **Journal** rolls its own transient "Copied" state with a manual `setTimeout`.
- **Bookmarks** and **Tasks** mostly swallow errors in `catch {}` or rely on
  `confirm()`, so a failed create/delete/move is often silent.

There is no success feedback anywhere (a save, a rescan, a "checked all"
completes with no acknowledgement), and errors either hijack a corner of the
page or vanish. A single toast system gives every module one consistent,
non-blocking, accessible way to confirm outcomes.

## Solution

Add a dependency-free **Toast** system to the existing component kit
(`web/src/components/ui.tsx`) and a context provider, matching the app's current
conventions: `class-variance-authority` variants, `lucide-react` icons, the HSL
design tokens in `index.css`, and the provider pattern already used by
`ThemeProvider` / `AuthProvider`. Then adopt it across the pages, replacing the
ad-hoc `setError` spots and the hand-rolled "Copied" state.

No new runtime dependencies (no `sonner`/`react-hot-toast`); the app already
bans extra libraries where a small local component suffices (see `Modal`, which
is deliberately portal-free and self-contained).

## User Stories

1. As the Owner, I want a brief confirmation when an action succeeds (saved,
   deleted, rescanned, checked), so that I know it worked without hunting for a
   change on screen.
2. As the Owner, I want a clear, non-blocking message when an action fails, so
   that I understand what went wrong without an alert box stealing focus.
3. As the Owner, I want toasts to stack and auto-dismiss after a few seconds, so
   that they don't pile up or require me to clear each one.
4. As the Owner, I want to dismiss a toast manually, so that I can clear it early.
5. As the Owner, I want error toasts to stay long enough to read (and not
   auto-dismiss too fast), so that I don't miss a failure.
6. As the Owner using a screen reader, I want toasts announced politely
   (assertively for errors), so that feedback is accessible.
7. As the Owner, I want toasts to respect light/dark theme and reduced-motion,
   so that they match the rest of the app.

## Implementation Decisions

### Component + provider

- A `ToastProvider` (in `web/src/lib/toast.tsx`, mirroring `theme.tsx`) holds the
  active toast queue in state and exposes a `useToast()` hook returning an
  imperative API:
  - `toast.success(message, opts?)`, `toast.error(message, opts?)`,
    `toast.info(message, opts?)` — and/or a base `toast({ title, description,
    variant, duration })`.
  - Returns an id so a caller can dismiss programmatically; `toast.dismiss(id)`.
- A `Toaster` viewport renders the stack in a fixed, screen-corner container
  (bottom-right on desktop, bottom-full-width on mobile), on top of everything
  (above `Modal`'s `z-50`). Rendered once near the app root.
- Each toast: a leading status icon (lucide `CheckCircle2` / `XCircle` /
  `Info`), a title, an optional description, and a close button, styled with a
  cva `toastVariants` using the existing `success` / `destructive` / `muted`
  tokens — consistent with `Badge`/`Button` variants.
- Auto-dismiss after a per-variant default (e.g. success/info ~3s, error ~6s),
  overridable via `duration`; `duration: 0` means sticky until dismissed. A
  timer per toast, cleared on manual dismiss/unmount. Pausing on hover is a
  nice-to-have, not required.
- Accessibility: the viewport is an `aria-live` region — `polite` for
  success/info, `assertive` for errors — with `role="status"`/`"alert"`
  accordingly; the close button has an `aria-label`. Honor
  `prefers-reduced-motion` (the CSS already globally dampens animations; the
  enter/leave transition should degrade gracefully).
- Animations reuse the existing `animate-fade-in` / `animate-scale-in`
  keyframes (or a small slide-in added to `tailwind.config`), keeping motion
  consistent with `Modal`.

### Wiring

- `ToastProvider` wraps the app in `web/src/main.tsx` (outside `BrowserRouter`
  is fine, like `ThemeProvider`), and `<Toaster />` is mounted once so any page
  can raise a toast.

### Adoption (replace ad-hoc feedback)

- **Files**: replace the `error` string + its render with `toast.error(...)` in
  the create-root / scan / rename / move / delete / batch catch blocks; add
  `toast.success(...)` for completed scans, saves, and batch operations.
- **Journal**: replace the manual `copiedId` + `setTimeout` with a
  `toast.success("Copied")` (keep the inline icon affordance if desired, but the
  transient text state goes through the toast).
- **Bookmarks**: surface create/edit/delete/check failures (currently swallowed
  in `catch {}`) via `toast.error(...)`, and confirm "Checked all" via
  `toast.success(...)`.
- **Tasks**: surface create/update/reorder/delete failures via `toast.error(...)`.
- **Login** keeps its inline field-level error (form-local validation reads
  better at the field than as a toast); this is intentional and called out so a
  later pass doesn't "fix" it.
- `confirm()` dialogs for destructive deletes are **left as-is** in this
  increment (a confirm → toast-on-result flow); replacing blocking confirms with
  a custom confirm dialog is out of scope here.

## Testing Decisions

- The project's test seam is the Go HTTP API (`server/...`); there is no
  frontend test runner configured, so this increment is verified by:
  - `cd web && npm run build` (type-check + Vite build) passing with the new
    component, provider, and all page edits.
  - Manual smoke of each adopted page: trigger a success and a failure and
    confirm a toast appears, stacks, auto-dismisses, and can be closed.
- If a frontend test setup is added later, the toast provider/hook is a natural
  unit to cover; not required now.

## Out of Scope

- Replacing native `confirm()` deletes with a custom confirmation dialog.
- A notification center / toast history / persistence across reloads.
- Promise-based toasts (`toast.promise`) and action buttons inside toasts — keep
  the first version to success/error/info with auto-dismiss + manual close.
- Any backend change; this is purely the SPA.
- Changing Login's inline field error to a toast.

## Further Notes

- No glossary impact: "Toast" is a UI-kit term, not a Nexus domain noun; no
  `CONTEXT.md` change.
- Keep it dependency-free and in the house style (cva variants, lucide icons,
  HSL tokens, provider+hook like `theme.tsx`), so it reads like the rest of
  `components/ui.tsx`.
- Ticket: `issues/01-toast-component.md` (single ticket — component + provider +
  adoption land together so the app never has two feedback systems at once).
