-- Servers module: Metric Samples capturing Host vitals over time.
CREATE TABLE metric_samples (
    id          BIGSERIAL PRIMARY KEY,
    taken_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    cpu_percent DOUBLE PRECISION NOT NULL,
    mem_total   BIGINT NOT NULL,
    mem_used    BIGINT NOT NULL,
    disk_total  BIGINT NOT NULL,
    disk_used   BIGINT NOT NULL,
    net_rx      BIGINT NOT NULL,
    net_tx      BIGINT NOT NULL,
    load1       DOUBLE PRECISION NOT NULL,
    load5       DOUBLE PRECISION NOT NULL,
    load15      DOUBLE PRECISION NOT NULL,
    uptime_secs BIGINT NOT NULL,
    -- top-N processes and docker containers captured as JSON snapshots
    top_procs   JSONB NOT NULL DEFAULT '[]'::jsonb,
    containers  JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE INDEX metric_samples_taken_at_idx ON metric_samples (taken_at DESC);
