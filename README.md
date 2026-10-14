# Nexus

Single-user personal workbench: one native Go process serving a React UI and a
JSON API over its own host, with five tools — Files, Servers, Tasks, Journal,
Bookmarks.

See `.scratch/nexus-v1/spec.md` for the full spec and `CONTEXT.md` for the
domain vocabulary.

## Layout

```
server/   Go backend (module `nexus`) — chi router, sqlc, golang-migrate, embedded SPA
web/      React + Vite + Tailwind SPA (built into server/internal/web/dist)
docker-compose.yml   Postgres only (ADR-0001: the Go binary runs natively on the Host)
```

## Prerequisites

- Go 1.25+
- Node 20+ / npm
- A reachable Postgres (dev uses `postgresql://postgres:postgres@10.37.41.217:5432/postgres`;
  `docker-compose.yml` provides a local one on `:5432`)

Generator CLIs used during development (install into `$GOBIN`):

```
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Build & run

```sh
# 1. Build the SPA into the server's embed dir
cd web && npm install && npm run build

# 2. Build / run the single binary
cd ../server
export NEXUS_DATABASE_URL='postgresql://postgres:postgres@HOST:5432/nexus'
export NEXUS_PASSWORD_HASH='<bcrypt hash of the Owner password>'
export NEXUS_JWT_SECRET='<random signing key>'
export NEXUS_ADDR=':8080'   # optional, defaults to :8080
export NEXUS_COOKIE_SECURE=false   # only for plain-HTTP local dev (default true)
go run ./cmd/nexus
```

Migrations run automatically on startup. Open `http://localhost:8080` and log in
with the single Owner password.

### Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `NEXUS_DATABASE_URL` | — (required) | Postgres connection string |
| `NEXUS_PASSWORD_HASH` | — (required) | bcrypt hash of the Owner password |
| `NEXUS_JWT_SECRET` | — (required) | session JWT signing key |
| `NEXUS_ADDR` | `:8080` | HTTP listen address |
| `NEXUS_COOKIE_SECURE` | `true` | mark the session cookie `Secure`; set `false` when serving plain HTTP (e.g. `http://localhost`) or the browser won't send it |
| `NEXUS_TRUSTED_PROXY` | `false` | resolve the client IP from `X-Forwarded-For` / `X-Real-IP`; enable only behind a reverse proxy you control |
| `NEXUS_LOGIN_MAX_FAILURES` | `5` | failed logins from one client before a lockout |
| `NEXUS_LOGIN_WINDOW` | `15m` | window in which failures are counted (Go duration) |
| `NEXUS_LOGIN_LOCKOUT` | `1m` | lockout length; the login returns `429` + `Retry-After` meanwhile |

Login throttling is in-memory and clears on restart (ADR-0006).

Generate a password hash, e.g.:

```go
bcrypt.GenerateFromPassword([]byte("your-password"), bcrypt.DefaultCost)
```

## Frontend dev server

```sh
cd web && npm run dev   # proxies /api to http://localhost:8080
```

## Regenerate sqlc code

```sh
cd server && sqlc generate   # reads sqlc.yaml, migrations, and queries
```

## Tests

The backend is tested through the HTTP API seam against an ephemeral Postgres
(each test creates and drops its own database). Override the admin connection
with `NEXUS_TEST_DATABASE_URL`.

```sh
cd server && go test ./...
```
