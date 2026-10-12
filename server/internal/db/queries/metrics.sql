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
