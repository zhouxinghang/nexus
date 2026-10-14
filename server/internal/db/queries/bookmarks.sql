-- name: CreateBookmark :one
INSERT INTO bookmarks (name, url, icon)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListBookmarks :many
SELECT * FROM bookmarks
ORDER BY created_at ASC, id ASC;

-- name: ListBookmarksByClicks :many
-- Most-visited first; ties go to the more recently opened, and never-opened
-- bookmarks keep creation order so the sort is stable.
SELECT * FROM bookmarks
ORDER BY click_count DESC, last_clicked_at DESC NULLS LAST, created_at ASC, id ASC;

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

-- name: RecordBookmarkMeta :one
-- Persist fetched page metadata without touching Owner-edited fields (name,
-- icon). Empty values are stored as NULL.
UPDATE bookmarks
SET fetched_title       = NULLIF(sqlc.arg(fetched_title)::text, ''),
    fetched_favicon_url = NULLIF(sqlc.arg(fetched_favicon_url)::text, ''),
    updated_at          = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: RecordBookmarkClick :execrows
-- Count one open. Not an edit, so updated_at is left alone.
UPDATE bookmarks
SET click_count     = click_count + 1,
    last_clicked_at = now()
WHERE id = $1;

-- name: BookmarkHealthSummary :one
SELECT
    count(*) FILTER (WHERE health = 'up')      AS up,
    count(*) FILTER (WHERE health = 'down')    AS down,
    count(*) FILTER (WHERE health = 'unknown') AS unknown
FROM bookmarks;

-- name: ListDownBookmarks :many
SELECT id, name, url, last_checked_at FROM bookmarks
WHERE health = 'down'
ORDER BY name ASC, id ASC;
