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

### Load configuration from `.env`

Instead of exporting each setting, copy `.env.example` to `.env` at the repo
root, restrict its permissions (`chmod 600 .env`), and fill in the database URL,
bcrypt password hash and JWT signing key. Use **single quotes** around hashes
and keys so `$` characters stay literal.

The Go process automatically reads `.env` in its startup working directory.
When started from `server/`, it falls back to the repo root's `.env` if
`server/.env` is absent. It loads only the first file found, never merging both.
No `source` command is needed:

```sh
cd server
go run ./cmd/nexus
```

Exported environment variables take priority over `.env`, including explicitly
empty values. If the startup shell still exports an old password hash or signing
key, unset that variable to use the file value. `.env` is optional for
environment-only deployments. A present but unreadable or malformed file aborts
startup without logging its contents.

To select a file explicitly (for example, when running a compiled binary from
another directory), set `NEXUS_ENV_FILE` in the startup environment. Relative
paths resolve against the startup working directory; an explicitly selected
missing file is an error. Do not put this selector inside `.env`.

```sh
NEXUS_ENV_FILE=/absolute/path/to/nexus.env ./nexus
```

Do not commit `.env` or send it in chat. Settings are read once on startup;
restart Nexus after editing them.

### Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `NEXUS_ENV_FILE` | auto-discovery (see above) | explicit dotenv file path, set in the startup environment |
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

## Page loading and static assets

The UI uses system sans-serif and monospace fonts, with no external font requests.
Each tool and the authenticated Shell load on demand; route loading and failures
leave navigation available, with a reload action if a chunk cannot be fetched.

`npm run build` also generates gzip siblings for HTML, JavaScript, CSS and SVG.
The native Go process serves these precompressed files when the browser accepts
gzip, retaining the original content type and uncompressed fallback. Hash-named
assets are cached for one year (`immutable`); HTML is revalidated on every use.
Missing asset URLs return 404 rather than SPA HTML. Byte-range requests use the
original representation.

After frontend changes, rebuild **and restart the Go process**: the SPA and its
gzip files are embedded at Go compile time.

## Copying Journal Entries

Journal's Copy button first uses the browser Clipboard API when available. On
ordinary HTTP or when that API rejects access, it attempts user-initiated text
selection copying without changing the Journal Entry or the append draft.
Temporary copy controls are removed and focus/selection are restored.

If the browser blocks both clipboard paths, a read-only dialog selects the
original text for Ctrl/Cmd+C or the device's Copy action. The dialog contains
keyboard focus, closes with Escape, and returns focus to the Copy button.
Successful-copy feedback is shown only when an automatic copy succeeds.
The Copy button stays visible on small screens.

## Host disk I/O statistics

Servers' **Disk I/O** card shows Host-wide read/write throughput (KB/s, MB/s)
and operations per second (IOPS). The disk history chart plots separate Read and
Write curves; its data table is also keyboard-accessible. Filesystem space
usage remains in **Filesystems**.

Rates come from per-device counter differences over the sampling interval,
not Nexus file operations or cumulative byte totals. On Linux, whole leaf
storage devices are counted once, excluding partitions, stacked storage
layers and memory/loop-backed devices.

Migration 18 adds nullable disk throughput and IOPS fields to Metric Samples.
Older samples and unavailable observations remain gaps, while measured idle
storage displays zero. The first observation establishes a baseline.

## Files directory browsing and metadata

Files has two distinct browsing modes:

- **Folders**: browse one Managed Root directory at a time, with the complete
  direct-child list and no pagination. Single-click selects; double-click or
  Enter opens a folder. Breadcrumbs, browser back/forward and Ctrl/Cmd+Up return
  through the hierarchy, including empty and Missing directories.
- **By type**: browse indexed Resources recursively, grouped by type on each
  server page (200 Resources). Combine type, Managed Root, folder-subtree, exact
  tag and case-insensitive name-substring filters. Previous/Next fetch just one
  page; the URL preserves filters, sorting and page position.

Use **Manage roots** to add/remove Managed Roots and start a Scan. Root management,
advanced filters and batch forms are disclosed only when needed. Single-click
selects without opening a detail panel; use the **File details** icon to manage
tags, rename, move or delete. Ctrl/Cmd-click and Shift-click support multi/range
selection. Double-click, Enter or Space previews a present Resource.

The **Name**, **Size**, **Modified** and **Created** headers sort in either direction. Sorting
and filtering happen on the server before type-view pagination; folders remain
above Resources in folder mode. Scan indexes new files; Missing Resources keep
their tags and remain reachable until cleared.

Scan skips dot-prefixed files and directory subtrees before indexing; single-file
registration rejects them too. Hidden directories are omitted from navigation.
This does not delete Host files or automatically clear existing Resource rows.

**Created** uses the Host file's actual creation time, not its Resource's index
time. Migration 19 adds nullable `file_created_at` metadata; rescan existing
Managed Roots to fill it, even for files whose size and modified time are
unchanged. Unavailable creation times display as `—` and sort last in both
directions. Mobile exposes the same sorting through **Sort by**.

Resource metadata is tags-only. Migration 17 permanently removes existing note
text; it does not change tags or Host files.

## Resource preview and text editing

In Files, double-click a Resource (or press Enter on its row) to preview it.
Images support zoom, audio/video use browser playback, and PDFs use the
browser's PDF viewer. Unsupported formats remain downloadable.

UTF-8 text up to 2 MiB opens as read-only source. Choose **Edit** for syntax
highlighting, search/replace, undo/redo and **Ctrl/Cmd+S** to save directly to the
Host. BOM and consistent line endings are preserved. HTML, Markdown and SVG
remain source, rather than rendered documents. Office editing and transcoding
are not included.

Unsaved edits prompt before closing/reloading/changing files. A disk-version
conflict keeps your draft and offers a draft download; reload before saving.
Writes are confined to the Managed Root, atomic, permission-preserving and
audited. Symlinks and files without write permission are read-only.

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

Browser interaction tests (mock API, no running backend required):

```sh
cd web
npx playwright install chromium   # once, if Chromium is not installed
npm test
```

`npm test` runs the existing development-server interaction suite followed by
production-bundle loading tests (including external-font independence, lazy
routes and lossless gzip output). Run just the latter with `npm run test:production`.
