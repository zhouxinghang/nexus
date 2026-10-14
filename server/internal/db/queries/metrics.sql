-- name: InsertMetricSample :one
INSERT INTO metric_samples (
    cpu_percent, mem_total, mem_used, disk_total, disk_used,
    net_rx, net_tx, load1, load5, load15, uptime_secs, top_procs, containers
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING *;

-- name: ListMetricSamplesInRange :many
SELECT * FROM metric_samples
WHERE taken_at >= $1 AND taken_at <= $2
ORDER BY taken_at ASC, id ASC;

-- name: PruneMetricSamplesBefore :execrows
DELETE FROM metric_samples WHERE taken_at < $1;

-- name: ListMetricSamplesBucketed :many
-- Display-time downsampling: average the samples in [from_at, to_at] into
-- fixed-width time buckets aligned to from_at, one row per non-empty bucket,
-- ascending. Gauges (CPU, memory/disk used, load) are averaged; totals and
-- cumulative counters (net bytes, uptime) take the bucket's max, i.e. the
-- latest reading. This is read-path aggregation only — the stored samples and
-- their retention are untouched.
SELECT
    date_bin(sqlc.arg(bucket)::interval, taken_at, sqlc.arg(from_at)::timestamptz)::timestamptz AS bucket_at,
    max(id)::bigint                        AS id,
    avg(cpu_percent)::double precision     AS cpu_percent,
    max(mem_total)::bigint                 AS mem_total,
    avg(mem_used)::bigint                  AS mem_used,
    max(disk_total)::bigint                AS disk_total,
    avg(disk_used)::bigint                 AS disk_used,
    max(net_rx)::bigint                    AS net_rx,
    max(net_tx)::bigint                    AS net_tx,
    avg(load1)::double precision           AS load1,
    avg(load5)::double precision           AS load5,
    avg(load15)::double precision          AS load15,
    max(uptime_secs)::bigint               AS uptime_secs
FROM metric_samples
WHERE taken_at >= sqlc.arg(from_at)::timestamptz AND taken_at <= sqlc.arg(to_at)::timestamptz
GROUP BY 1
ORDER BY 1 ASC;
