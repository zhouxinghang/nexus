ALTER TABLE metric_samples
    DROP COLUMN IF EXISTS cpu_cores,
    DROP COLUMN IF EXISTS net_rx_rate,
    DROP COLUMN IF EXISTS net_tx_rate,
    DROP COLUMN IF EXISTS net_rx_pps,
    DROP COLUMN IF EXISTS net_tx_pps,
    DROP COLUMN IF EXISTS psi_cpu_some,
    DROP COLUMN IF EXISTS psi_mem_some,
    DROP COLUMN IF EXISTS psi_mem_full,
    DROP COLUMN IF EXISTS psi_io_some,
    DROP COLUMN IF EXISTS psi_io_full,
    DROP COLUMN IF EXISTS self_cpu,
    DROP COLUMN IF EXISTS self_rss;
