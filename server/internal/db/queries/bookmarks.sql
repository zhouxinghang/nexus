-- name: CreateBookmark :one
INSERT INTO bookmarks (name, url, icon)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListBookmarks :many
SELECT * FROM bookmarks
ORDER BY created_at ASC, id ASC;

-- name: GetBookmark :one
SELECT * FROM bookmarks WHERE id = $1;

-- name: UpdateBookmark :one
UPDATE bookmarks
SET name       = $2,
    url        = $3,
    icon       = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteBookmark :exec
DELETE FROM bookmarks WHERE id = $1;

-- name: RecordBookmarkCheck :one
-- Persist the outcome of a health check without touching Owner-edited fields.
UPDATE bookmarks
SET health          = $2,
    last_status     = $3,
    last_latency_ms = $4,
    last_checked_at = now(),
    updated_at      = now()
WHERE id = $1
RETURNING *;
