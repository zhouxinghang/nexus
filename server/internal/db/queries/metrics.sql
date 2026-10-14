-- name: InsertMetricSample :one
INSERT INTO metric_samples (
    cpu_percent, mem_total, mem_used, disk_total, disk_used,
    net_rx, net_tx, load1, load5, load15, uptime_secs, top_procs, containers,
    cpu_cores, net_rx_rate, net_tx_rate, net_rx_pps, net_tx_pps,
    psi_cpu_some, psi_mem_some, psi_mem_full, psi_io_some, psi_io_full,
    self_cpu, self_rss, disk_read_rate, disk_write_rate, disk_read_iops, disk_write_iops
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
          $14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
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
-- ascending. Gauges (CPU, memory/disk used, load, network rates, pressure,
-- disk I/O, Nexus's own usage) are averaged; totals and cumulative counters (net bytes,
-- uptime, cores) take the bucket's max, i.e. the latest reading. This is
-- read-path aggregation only — the stored samples and their retention are
-- untouched.
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
    max(uptime_secs)::bigint               AS uptime_secs,
    coalesce(max(cpu_cores), 0)::integer   AS cpu_cores,
    -- The newer gauges average only the samples that carry them; the *_n
    -- counts tell a bucket with no such samples apart from a real zero.
    count(net_rx_rate)::bigint                       AS net_rate_n,
    coalesce(avg(net_rx_rate), 0)::double precision  AS net_rx_rate,
    coalesce(avg(net_tx_rate), 0)::double precision  AS net_tx_rate,
    coalesce(avg(net_rx_pps), 0)::double precision   AS net_rx_pps,
    coalesce(avg(net_tx_pps), 0)::double precision   AS net_tx_pps,
    count(disk_read_rate)::bigint                      AS disk_rate_n,
    coalesce(avg(disk_read_rate), 0)::double precision  AS disk_read_rate,
    coalesce(avg(disk_write_rate), 0)::double precision AS disk_write_rate,
    coalesce(avg(disk_read_iops), 0)::double precision  AS disk_read_iops,
    coalesce(avg(disk_write_iops), 0)::double precision AS disk_write_iops,
    count(psi_cpu_some)::bigint                      AS psi_n,
    coalesce(avg(psi_cpu_some), 0)::double precision AS psi_cpu_some,
    coalesce(avg(psi_mem_some), 0)::double precision AS psi_mem_some,
    coalesce(avg(psi_mem_full), 0)::double precision AS psi_mem_full,
    coalesce(avg(psi_io_some), 0)::double precision  AS psi_io_some,
    coalesce(avg(psi_io_full), 0)::double precision  AS psi_io_full,
    count(self_cpu)::bigint                          AS self_n,
    coalesce(avg(self_cpu), 0)::double precision     AS self_cpu,
    coalesce(avg(self_rss), 0)::bigint               AS self_rss
FROM metric_samples
WHERE taken_at >= sqlc.arg(from_at)::timestamptz AND taken_at <= sqlc.arg(to_at)::timestamptz
GROUP BY 1
ORDER BY 1 ASC;
