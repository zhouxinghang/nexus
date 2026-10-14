-- Disk I/O throughput and operations/sec. NULL means no usable observation
-- (legacy row, unsupported Host, first reading or reset counters), not idle.
ALTER TABLE metric_samples
    ADD COLUMN disk_read_rate  DOUBLE PRECISION,
    ADD COLUMN disk_write_rate DOUBLE PRECISION,
    ADD COLUMN disk_read_iops  DOUBLE PRECISION,
    ADD COLUMN disk_write_iops DOUBLE PRECISION;
