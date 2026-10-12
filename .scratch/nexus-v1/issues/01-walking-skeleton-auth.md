# 01: Walking skeleton + auth

**What to build:** The whole stack exists and is cut through once, end to end, so
the Owner can log in and reach a protected endpoint. The monorepo is laid out
(`web/` React+shadcn+Vite, `server/` Go), a single-service `docker-compose.yml`
runs Postgres, and the Go binary wires `golang-migrate` migrations, `sqlc`
queries, a `chi` router, and `go:embed` of the built SPA. The Owner opens Nexus,
is shown a login screen, enters the single password, and lands on an
authenticated shell; an expired/invalid session redirects back to login; logout
ends the session. This ticket also establishes the shared test harness every
later ticket reuses.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Monorepo layout exists: `web/` (React + shadcn, built with Vite) and `server/` (Go), with a root `docker-compose.yml` that runs only Postgres.
- [ ] The Go binary serves both the embedded SPA (`go:embed`) and the JSON API from one process (native host process, per ADR-0001).
- [ ] `golang-migrate` versioned SQL migrations run on startup; `sqlc` generates type-safe query code; routing uses `net/http` + `chi`.
- [ ] Login accepts the single password (verified against a bcrypt hash held in config/env, not the DB) and rejects any wrong password.
- [ ] A successful login issues a JWT carried in an HttpOnly, SameSite=Strict cookie with ~7-day lifetime; the signing key comes from config/env; there is no refresh token.
- [ ] Auth middleware protects all API endpoints except login; missing/expired/invalid sessions are rejected.
- [ ] A protected endpoint (e.g. current-session/`/me`) returns the authenticated state; logout invalidates the session.
- [ ] The React app shows a login screen, stores no token in JS-readable storage, and redirects to login when the session is invalid or expired.
- [ ] Test harness is in place: the real `chi` router driven via `httptest` against an ephemeral Postgres, a login helper that yields the auth cookie, and an injectable `Host` port seam (even if unused this ticket). Tests assert login success/failure and that protected endpoints reject unauthenticated requests.
