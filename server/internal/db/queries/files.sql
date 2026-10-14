-- name: CreateManagedRoot :one
INSERT INTO managed_roots (path)
VALUES ($1)
RETURNING *;

-- name: ListManagedRoots :many
SELECT * FROM managed_roots
ORDER BY created_at ASC, id ASC;

-- name: GetManagedRoot :one
SELECT * FROM managed_roots WHERE id = $1;

-- name: DeleteManagedRoot :exec
DELETE FROM managed_roots WHERE id = $1;

-- name: UpsertResource :one
INSERT INTO resources (root_id, path, name, size, is_dir, modified_at, state, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, 'present', now())
ON CONFLICT (path) DO UPDATE
SET root_id     = EXCLUDED.root_id,
    name        = EXCLUDED.name,
    size        = EXCLUDED.size,
    is_dir      = EXCLUDED.is_dir,
    modified_at = EXCLUDED.modified_at,
    state       = 'present',
    updated_at  = now()
RETURNING *;

-- name: GetResource :one
SELECT * FROM resources WHERE id = $1;

-- name: ListResourcesByIDs :many
SELECT * FROM resources
WHERE id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY path ASC;

-- name: GetResourceByPath :one
SELECT * FROM resources WHERE path = $1;

-- name: ListResourcesPage :many
-- One page of Resources under the optional root / directory-prefix / tag
-- filters, ordered by path. The prefix is an already-escaped LIKE pattern
-- (e.g. '/data/docs/%'); the tag filter joins resource_tags in SQL.
SELECT * FROM resources r
WHERE (sqlc.narg(root_id)::bigint IS NULL OR r.root_id = sqlc.narg(root_id)::bigint)
  AND (sqlc.narg(dir_pattern)::text IS NULL OR r.path LIKE sqlc.narg(dir_pattern)::text)
  AND (sqlc.narg(tag)::text IS NULL OR EXISTS (
        SELECT 1 FROM resource_tags rt
        WHERE rt.resource_id = r.id AND rt.tag = sqlc.narg(tag)::text))
ORDER BY r.path ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountResources :one
-- The total for the same filter set as ListResourcesPage.
SELECT count(*) FROM resources r
WHERE (sqlc.narg(root_id)::bigint IS NULL OR r.root_id = sqlc.narg(root_id)::bigint)
  AND (sqlc.narg(dir_pattern)::text IS NULL OR r.path LIKE sqlc.narg(dir_pattern)::text)
  AND (sqlc.narg(tag)::text IS NULL OR EXISTS (
        SELECT 1 FROM resource_tags rt
        WHERE rt.resource_id = r.id AND rt.tag = sqlc.narg(tag)::text));

-- name: ResourceSummary :one
SELECT count(*)                                   AS total,
       count(*) FILTER (WHERE state = 'missing') AS missing
FROM resources;

-- name: ListResourceIDsByRoot :many
SELECT id, path FROM resources
WHERE root_id = $1;

-- name: MarkResourceMissing :exec
UPDATE resources
SET state = 'missing', updated_at = now()
WHERE id = $1;

-- name: DeleteResource :exec
DELETE FROM resources WHERE id = $1;

-- name: DeleteResourcesByIDs :exec
DELETE FROM resources WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: UpdateResourceNote :one
UPDATE resources
SET note = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateResourcePath :one
UPDATE resources
SET path = $2, name = $3, root_id = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: AddResourceTag :exec
INSERT INTO resource_tags (resource_id, tag)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: AddTagToResources :exec
INSERT INTO resource_tags (resource_id, tag)
SELECT id, sqlc.arg(tag)::text
FROM unnest(sqlc.arg(ids)::bigint[]) AS id
ON CONFLICT DO NOTHING;

-- name: RemoveResourceTag :exec
DELETE FROM resource_tags WHERE resource_id = $1 AND tag = $2;

-- name: ListTagsForResource :many
SELECT tag FROM resource_tags WHERE resource_id = $1 ORDER BY tag ASC;

-- name: ListTagsForResources :many
SELECT resource_id, tag FROM resource_tags
WHERE resource_id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY tag ASC;

-- name: InsertAuditLog :one
INSERT INTO file_audit_log (action, resource_id, old_path, new_path)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListAuditLog :many
SELECT * FROM file_audit_log
ORDER BY created_at DESC, id DESC;
