# 01: Login rate limiting + Secure cookie

Status: done

**What to build:** Make the single-password login resistant to brute force and
safe over HTTPS. Today `POST /api/auth/login` (`server/internal/httpapi/auth_handlers.go`)
calls `auth.VerifyPassword` with no failure throttling, and `auth.SetSessionCookie`
/ `ClearSessionCookie` (`server/internal/auth/auth.go`) never set `Secure`. Add
per-client failure throttling with a cooldown, a configurable `Secure` cookie
flag, and correct client-IP resolution so throttling isn't fooled by a reverse
proxy.

**Blocked by:** None

**Spec:** `.scratch/workbench-polish/spec.md`

## Acceptance criteria

- [x] An in-memory failure limiter (in `internal/auth` or a sibling package)
      tracks consecutive failed logins per client key, with an **injectable
      clock** (mirror the existing `Authenticator.now` seam) so tests are
      deterministic. State is process-local and cleared on restart.
- [x] After **N** consecutive failures within a window, `POST /api/auth/login`
      returns **`429 Too Many Requests`** with a **`Retry-After`** header, for a
      configurable cooldown. Defaults are sane (e.g. 5 failures → 1-minute
      cooldown) and all knobs are env-configurable.
- [x] A correct password **before** the threshold still succeeds; a **successful
      login clears** the client's failure count.
- [x] Once the cooldown elapses, the same client can attempt again and a correct
      password succeeds (no permanent lockout).
- [x] The client key is the **resolved client IP**: use `RemoteAddr` by default;
      honor `X-Forwarded-For` / `X-Real-IP` **only** when
      `NEXUS_TRUSTED_PROXY` is enabled, so header spoofing can't evade or poison
      throttling by default.
- [x] Session cookie gains a **`Secure`** flag from config
      (`NEXUS_COOKIE_SECURE`, default `true`; set `false` for local HTTP dev).
      `HttpOnly` and `SameSite=Strict` are unchanged. `ClearSessionCookie`
      matches so the cookie actually clears under the same attributes.
- [x] New config fields are added to `internal/config` with defaults; `Load`
      documents the new env vars (and `README.md` lists them).
- [x] Tests through the HTTP seam (`internal/httpapi`): N wrong attempts →
      `429` + `Retry-After`; correct password before threshold succeeds; cooldown
      elapse (via injected clock) re-allows login; success resets the counter;
      `Secure` toggles with config; forwarded headers honored only with the
      trusted-proxy toggle on.

## Notes

- Keep the limiter small and dependency-free; a map keyed by client IP with
  `{failures, firstFailureAt / lockedUntil}` plus a mutex is enough for a
  single-process app. Prune stale entries opportunistically so it can't grow
  unbounded.
- The limiter check must run **before** `VerifyPassword` for a locked client
  (don't do bcrypt work while locked out), and record a failure **after** a
  verified-wrong password.
- Consider a short ADR documenting the deliberate choice of in-memory,
  non-persistent throttling (restart clears it) as acceptable for single-Owner.

## Comments

- Implemented: `internal/auth/limiter.go` (mutex-guarded map, pruned past
  1024 entries) is wired through `auth.New` options: `WithClock`,
  `WithCookieSecure`, `WithTrustedProxy`, and `WithLimitPolicy`. A locked
  client gets 429 + `Retry-After` before any bcrypt work, and the request that
  hits the threshold also returns 429. A successful login resets the count.
- Defaults: 5 failures within 15m → 1m lockout. Env vars:
  `NEXUS_LOGIN_MAX_FAILURES`, `NEXUS_LOGIN_WINDOW`, `NEXUS_LOGIN_LOCKOUT`,
  `NEXUS_COOKIE_SECURE` (default true), and `NEXUS_TRUSTED_PROXY` (default
  false). All are documented in `config.Load` and the README table.
- With the trusted-proxy toggle on, the client IP is the right-most
  `X-Forwarded-For` entry (the one the proxy appended), then `X-Real-IP`.
- **Heads-up:** because `Secure` now defaults to true, plain-HTTP local dev
  (`http://localhost:8080`) needs `NEXUS_COOKIE_SECURE=false`, or the
  browser won't send the session cookie back. The README calls this out.
- Login page shows a 429-specific inline message. The trade-off is recorded
  in ADR-0006 (`docs/adr/0006-in-memory-login-throttling.md`).
