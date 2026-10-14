-- name: CreateJournalEntry :one
INSERT INTO journal_entries (body)
VALUES ($1)
RETURNING id, body, created_at;

-- name: ListJournalEntries :many
-- One page of the stream, newest first. Keyset on (created_at, id): pass the
-- last row of the previous page as the cursor, or NULLs for the first page.
-- The stream is append-only, so a cursor never skips or repeats an entry.
SELECT id, body, created_at
FROM journal_entries
WHERE sqlc.narg(cursor_created_at)::timestamptz IS NULL
   OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::bigint)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: SearchJournalEntries :many
-- Case-insensitive substring match, language-agnostic (works for CJK). The
-- caller passes an already-escaped term wrapped in '%'; backslash is the
-- LIKE escape character. Backed by a GIN trigram index on body. Paged with
-- the same keyset as ListJournalEntries.
SELECT id, body, created_at
FROM journal_entries
WHERE body ILIKE sqlc.arg(pattern)
  AND (sqlc.narg(cursor_created_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::bigint))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: LatestJournalEntries :many
SELECT id, body, created_at
FROM journal_entries
ORDER BY created_at DESC, id DESC
LIMIT $1;
