# 02: Journal module

**What to build:** The Owner can keep an append-only journal. From the Journal
view they type plain text and append a Journal Entry, which is automatically
stamped with its creation time and shown at the top of a newest-first stream.
They can full-text search their entries by content. Entries cannot be edited or
deleted — the Journal is a trustworthy, immutable record (ADR-0003).

**Blocked by:** 01 (walking skeleton + auth)

**Status:** done

- [x] The Owner can append a plain-text Journal Entry through the API and UI.
- [x] Each entry is automatically stamped with its creation time; the Owner does not supply a timestamp.
- [x] Entries are displayed newest-first as a stream.
- [x] Full-text search over entry text (Postgres full-text search) returns matching entries.
- [x] There is no edit and no delete path — the API offers neither, and the UI exposes neither.
- [x] Entries carry no tags.
- [x] Tests (through the HTTP API seam) assert: append works, timestamps are auto-set, newest-first ordering, search finds entries by content, and edit/delete are not possible.

## Comments

- `journal_entries` table: `body`, `created_at DEFAULT now()`, and a generated `tsvector` (`to_tsvector('english', body)`) with a GIN index. Search uses `websearch_to_tsquery`.
- API: `POST /api/journal/entries` (append) and `GET /api/journal/entries` (newest-first, optional `?q=` full-text search). No edit/delete routes exist. UI in `web/src/pages/Journal.tsx`.
- Tests in `internal/httpapi/journal_test.go` cover append + auto timestamp, newest-first ordering, full-text search, absence of edit/delete (405/404), and empty-body rejection.
