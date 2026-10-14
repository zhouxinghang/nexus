-- Servers: the Bazel-profiler-style resource set. Network throughput is the
-- rate since the previous reading (bytes and packets per second, non-loopback
-- interfaces); psi_* are Linux pressure-stall avg10 percentages; self_* are
-- the Nexus process's own CPU (% of one core) and RSS. All nullable: samples
-- taken before this migration, off Linux (psi_*), or on a Host's first
-- reading (rates) simply have no value.
ALTER TABLE metric_samples
    ADD COLUMN cpu_cores    INTEGER,
    ADD COLUMN net_rx_rate  DOUBLE PRECISION,
    ADD COLUMN net_tx_rate  DOUBLE PRECISION,
    ADD COLUMN net_rx_pps   DOUBLE PRECISION,
    ADD COLUMN net_tx_pps   DOUBLE PRECISION,
    ADD COLUMN psi_cpu_some DOUBLE PRECISION,
    ADD COLUMN psi_mem_some DOUBLE PRECISION,
    ADD COLUMN psi_mem_full DOUBLE PRECISION,
    ADD COLUMN psi_io_some  DOUBLE PRECISION,
    ADD COLUMN psi_io_full  DOUBLE PRECISION,
    ADD COLUMN self_cpu     DOUBLE PRECISION,
    ADD COLUMN self_rss     BIGINT;
