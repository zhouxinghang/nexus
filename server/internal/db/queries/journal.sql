-- name: CreateJournalEntry :one
INSERT INTO journal_entries (body)
VALUES ($1)
RETURNING id, body, created_at;

-- name: ListJournalEntries :many
SELECT id, body, created_at
FROM journal_entries
ORDER BY created_at DESC, id DESC;

-- name: SearchJournalEntries :many
SELECT id, body, created_at
FROM journal_entries
WHERE search @@ websearch_to_tsquery('english', $1)
ORDER BY created_at DESC, id DESC;
