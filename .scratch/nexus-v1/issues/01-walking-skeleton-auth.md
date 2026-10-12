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

**Status:** done

- [x] Monorepo layout exists: `web/` (React + shadcn, built with Vite) and `server/` (Go), with a root `docker-compose.yml` that runs only Postgres.
- [x] The Go binary serves both the embedded SPA (`go:embed`) and the JSON API from one process (native host process, per ADR-0001).
- [x] `golang-migrate` versioned SQL migrations run on startup; `sqlc` generates type-safe query code; routing uses `net/http` + `chi`.
- [x] Login accepts the single password (verified against a bcrypt hash held in config/env, not the DB) and rejects any wrong password.
- [x] A successful login issues a JWT carried in an HttpOnly, SameSite=Strict cookie with ~7-day lifetime; the signing key comes from config/env; there is no refresh token.
- [x] Auth middleware protects all API endpoints except login; missing/expired/invalid sessions are rejected.
- [x] A protected endpoint (e.g. current-session/`/me`) returns the authenticated state; logout invalidates the session.
- [x] The React app shows a login screen, stores no token in JS-readable storage, and redirects to login when the session is invalid or expired.
- [x] Test harness is in place: the real `chi` router driven via `httptest` against an ephemeral Postgres, a login helper that yields the auth cookie, and an injectable `Host` port seam (even if unused this ticket). Tests assert login success/failure and that protected endpoints reject unauthenticated requests.

## Comments

- Implemented in `server/` (Go module `nexus`). Migrations embedded via `go:embed` and run on startup with `golang-migrate` (pgx/v5 driver). `sqlc` generates `internal/db/dbgen`. Router is `chi` with a `Recoverer` middleware and an `/api` group; the authenticated subgroup is guarded by `auth.Middleware`.
- Auth: bcrypt verify + HS256 JWT (`golang-jwt/v5`) in an HttpOnly, SameSite=Strict cookie, 7-day TTL. Signing key and password hash come from `NEXUS_JWT_SECRET` / `NEXUS_PASSWORD_HASH`. `/api/auth/me`, `/api/auth/logout` added.
- Host port seam established in `internal/host` (FS + Metrics + Docker) with a real impl and an injectable `FakeHost` backed by a real temp dir.
- Shared test harness in `internal/httpapi/harness_test.go` spins up an ephemeral Postgres DB per test (CREATE/DROP DATABASE), runs migrations, drives the real router via `httptest`, and provides a login helper. Auth tests pass.
- SPA login screen + authed shell in `web/` (React + Vite + Tailwind); session carried only via the cookie (no JS-readable token); unauthenticated/expired → redirect to `/login`.
