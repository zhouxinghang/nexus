-- name: CreateJournalEntry :one
INSERT INTO journal_entries (body)
VALUES ($1)
RETURNING id, body, created_at;

-- name: ListJournalEntries :many
SELECT id, body, created_at
FROM journal_entries
ORDER BY created_at DESC, id DESC;

-- name: SearchJournalEntries :many
-- Case-insensitive substring match, language-agnostic (works for CJK). The
-- caller passes an already-escaped term wrapped in '%'; backslash is the
-- LIKE escape character. Backed by a GIN trigram index on body.
SELECT id, body, created_at
FROM journal_entries
WHERE body ILIKE $1
ORDER BY created_at DESC, id DESC;
