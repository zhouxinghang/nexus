# 01: Unified toast component + provider + adoption

Status: ready-for-agent

**What to build:** A dependency-free toast system for the SPA and its adoption
across the pages, replacing today's scattered feedback (Files' single `error`
string, Journal's hand-rolled "Copied" state, swallowed `catch {}` in Bookmarks
/ Tasks). One consistent, non-blocking, accessible way to confirm success and
report failure.

**Blocked by:** None

**Spec:** `.scratch/ui-toast/spec.md`

## Acceptance criteria

### Component + provider

- [ ] `web/src/lib/toast.tsx` adds a `ToastProvider` + `useToast()` hook
      (mirroring `theme.tsx`), exposing `toast.success/error/info(message, opts?)`
      plus a base `toast({title, description, variant, duration})`;
      `toast.dismiss(id)` dismisses programmatically.
- [ ] A `Toaster` viewport renders the stack fixed in a screen corner
      (bottom-right desktop / bottom full-width mobile), above `Modal`'s `z-50`,
      mounted once near the app root.
- [ ] Each toast has a leading status icon (lucide `CheckCircle2`/`XCircle`/
      `Info`), title, optional description, and a close button, styled via a cva
      `toastVariants` using the existing `success`/`destructive`/`muted` tokens
      (house style, like `Badge`/`Button`).
- [ ] Auto-dismiss per variant (success/info ~3s, error ~6s), overridable via
      `duration`; `duration: 0` = sticky. Timers cleared on manual dismiss /
      unmount. Toasts stack and newest is visible.
- [ ] Accessibility: viewport is an `aria-live` region (`polite` for
      success/info, `assertive` for errors) with appropriate `role`; close
      button has an `aria-label`. Respects `prefers-reduced-motion`.
- [ ] `ToastProvider` wraps the app in `web/src/main.tsx`; `<Toaster />` mounted
      once.

### Adoption

- [ ] **Files**: replace the `error` string + its render with `toast.error(...)`
      in create-root / scan / rename / move / delete / batch catch blocks; add
      `toast.success(...)` for completed scan / save / batch ops.
- [ ] **Journal**: replace the `copiedId` + `setTimeout` state with
      `toast.success("Copied")` (inline icon affordance may remain).
- [ ] **Bookmarks**: surface create/edit/delete/check failures (currently
      swallowed) via `toast.error(...)`; confirm "Checked all" via
      `toast.success(...)`.
- [ ] **Tasks**: surface create/update/reorder/delete failures via
      `toast.error(...)`.
- [ ] **Login** keeps its inline field-level error (intentional; do not convert).
- [ ] Native `confirm()` deletes are left as-is (confirm → toast-on-result).

### Verify

- [ ] `cd web && npm run build` passes (type-check + Vite build) with the new
      component, provider, and all page edits.

## Notes

- No new dependencies (no `sonner`/`react-hot-toast`); keep it self-contained
  like the portal-free `Modal`.
- Reuse existing `animate-fade-in`/`animate-scale-in` or add one small slide-in
  keyframe to `tailwind.config`.
- Component + provider + adoption land together so the app never runs two
  feedback systems at once.
