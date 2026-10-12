# Nexus

Single-user personal workbench: one native Go process serving a React UI and a
JSON API over its own host, with four tools — Files, Servers, Tasks, Journal.

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
go run ./cmd/nexus
```

Migrations run automatically on startup. Open `http://localhost:8080` and log in
with the single Owner password.

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
