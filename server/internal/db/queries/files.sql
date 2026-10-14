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

-- name: BulkUpsertResources :exec
-- Upserts many Resources in a single round-trip from parallel arrays. A plain
-- scan otherwise fans out to one INSERT per file; collapsing the changed rows
-- into one statement is the difference between thousands of round-trips and one
-- (the FileBrowser-Quantum "batch the index writes" idea, over Postgres). The
-- arrays are positionally aligned and zipped back together by ordinality:
-- paths[i] carries names[i], sizes[i], etc.
INSERT INTO resources (root_id, path, name, size, is_dir, modified_at, state, updated_at)
SELECT sqlc.arg(root_id)::bigint, p.path, n.name, s.size, d.is_dir, m.modified_at, 'present', now()
FROM unnest(sqlc.arg(paths)::text[]) WITH ORDINALITY AS p(path, i)
JOIN unnest(sqlc.arg(names)::text[]) WITH ORDINALITY AS n(name, i) USING (i)
JOIN unnest(sqlc.arg(sizes)::bigint[]) WITH ORDINALITY AS s(size, i) USING (i)
JOIN unnest(sqlc.arg(is_dirs)::boolean[]) WITH ORDINALITY AS d(is_dir, i) USING (i)
JOIN unnest(sqlc.arg(modified_ats)::timestamptz[]) WITH ORDINALITY AS m(modified_at, i) USING (i)
ON CONFLICT (path) DO UPDATE
SET root_id     = EXCLUDED.root_id,
    name        = EXCLUDED.name,
    size        = EXCLUDED.size,
    is_dir      = EXCLUDED.is_dir,
    modified_at = EXCLUDED.modified_at,
    state       = 'present',
    updated_at  = now();

-- name: MarkResourcesMissingUnderRoot :execrows
-- Reconciles a root in one statement: every present Resource whose path wasn't
-- seen on disk this scan is marked missing. Replaces per-row MarkResourceMissing
-- calls, and returns the number of rows transitioned so a scan can report how
-- many Resources vanished. An empty seen set (empty directory) marks every
-- present row missing, since `path <> ALL('{}')` is true for every path.
UPDATE resources
SET state = 'missing', updated_at = now()
WHERE root_id = sqlc.arg(root_id)::bigint
  AND state <> 'missing'
  AND path <> ALL(sqlc.arg(seen_paths)::text[]);

-- name: ListResourceDigestByRoot :many
-- The present Resources' change-detection digest for a root. A scan loads these
-- once, then skips re-upserting any file whose size and modified_at are
-- unchanged — the modtime-based "quick scan" that makes rescans cheap.
SELECT path, size, modified_at FROM resources
WHERE root_id = $1 AND state = 'present';

-- name: GetResource :one
SELECT * FROM resources WHERE id = $1;

-- name: ListResourcesByIDs :many
SELECT * FROM resources
WHERE id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY path ASC;

-- name: GetResourceByPath :one
SELECT * FROM resources WHERE path = $1;

-- name: ListResourcesPage :many
-- One page of Resources under the optional root / directory-prefix / tag /
-- kind filters, ordered by path. The prefix is an already-escaped LIKE pattern
-- (e.g. '/data/docs/%'); the tag filter joins resource_tags in SQL; kind
-- matches the generated resource.kind column.
SELECT * FROM resources r
WHERE (sqlc.narg(root_id)::bigint IS NULL OR r.root_id = sqlc.narg(root_id)::bigint)
  AND (sqlc.narg(dir_pattern)::text IS NULL OR r.path LIKE sqlc.narg(dir_pattern)::text)
  AND (sqlc.narg(kind)::text IS NULL OR r.kind = sqlc.narg(kind)::text)
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
  AND (sqlc.narg(kind)::text IS NULL OR r.kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(tag)::text IS NULL OR EXISTS (
        SELECT 1 FROM resource_tags rt
        WHERE rt.resource_id = r.id AND rt.tag = sqlc.narg(tag)::text));

-- name: CountResourcesByKind :many
-- Per-kind totals for the root / directory-prefix / tag filters, deliberately
-- ignoring the kind filter itself so the UI's type chips always show how many
-- Resources of each kind are reachable within the other active filters.
SELECT r.kind, count(*) AS count FROM resources r
WHERE (sqlc.narg(root_id)::bigint IS NULL OR r.root_id = sqlc.narg(root_id)::bigint)
  AND (sqlc.narg(dir_pattern)::text IS NULL OR r.path LIKE sqlc.narg(dir_pattern)::text)
  AND (sqlc.narg(tag)::text IS NULL OR EXISTS (
        SELECT 1 FROM resource_tags rt
        WHERE rt.resource_id = r.id AND rt.tag = sqlc.narg(tag)::text))
GROUP BY r.kind;

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
