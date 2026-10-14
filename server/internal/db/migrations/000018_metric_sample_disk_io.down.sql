ALTER TABLE metric_samples
    DROP COLUMN disk_read_rate,
    DROP COLUMN disk_write_rate,
    DROP COLUMN disk_read_iops,
    DROP COLUMN disk_write_iops;
