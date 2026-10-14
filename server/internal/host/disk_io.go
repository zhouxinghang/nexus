package host

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

// diskReading keeps per-device baselines: subtracting aggregate counters would
// produce spikes when a drive appears/disappears or its counters reset.
type diskReading struct {
	at      time.Time
	devices map[string]disk.IOCountersStat
}

// diskIOCounters reads one storage layer, rather than summing a disk, its
// partitions and device-mapper/RAID counters for the same I/O. Darwin's
// gopsutil provider already reports whole storage devices.
func diskIOCounters() (map[string]disk.IOCountersStat, error) {
	if runtime.GOOS != "linux" {
		return disk.IOCounters()
	}
	sys := os.Getenv("HOST_SYS")
	if sys == "" {
		sys = "/sys"
	}
	names, err := diskDeviceNames(filepath.Join(sys, "block"))
	if err != nil || len(names) == 0 {
		return nil, err // Never call IOCounters() unfiltered on Linux.
	}
	return disk.IOCounters(names...)
}

// diskDeviceNames selects whole, leaf storage devices from Linux /sys/block.
// Top-level entries exclude partitions; a non-empty slaves directory denotes
// another accounting layer over those same drives. Virtual-machine disks are
// included; memory/loop-backed storage is not.
func diskDeviceNames(blockDir string) ([]string, error) {
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "zram") {
			continue
		}
		slaves, err := os.ReadDir(filepath.Join(blockDir, name, "slaves"))
		if os.IsNotExist(err) {
			continue // drive was removed during discovery
		}
		if err != nil {
			return nil, err
		}
		if len(slaves) == 0 {
			names = append(names, name)
		}
	}
	return names, nil
}

func diskRate(prev, cur diskReading) *DiskRate {
	secs := cur.at.Sub(prev.at).Seconds()
	if prev.at.IsZero() || len(prev.devices) == 0 || secs <= 0 {
		return nil
	}
	var rate DiskRate
	matched := false
	for name, c := range cur.devices {
		p, ok := prev.devices[name]
		if !ok || (p.SerialNumber != "" && c.SerialNumber != "" && p.SerialNumber != c.SerialNumber) {
			continue // newly attached/replaced drive needs a fresh baseline
		}
		if c.ReadBytes < p.ReadBytes || c.WriteBytes < p.WriteBytes || c.ReadCount < p.ReadCount || c.WriteCount < p.WriteCount {
			continue // counter reset/wrap: don't fabricate a throughput spike
		}
		matched = true
		rate.ReadBytes += float64(c.ReadBytes-p.ReadBytes) / secs
		rate.WriteBytes += float64(c.WriteBytes-p.WriteBytes) / secs
		rate.ReadOps += float64(c.ReadCount-p.ReadCount) / secs
		rate.WriteOps += float64(c.WriteCount-p.WriteCount) / secs
	}
	if !matched {
		return nil
	}
	return &rate // a measured idle disk is a real zero, not "no data"
}

// sampleDisk clears the baseline on a failed/empty observation. Recovery
// requires two fresh observations, not an average across an unknown gap.
func (h *RealHost) sampleDisk(v *Vitals, at time.Time, counters map[string]disk.IOCountersStat, err error) {
	v.DiskRate = nil
	if err != nil || len(counters) == 0 {
		h.prevDisk = diskReading{}
		return
	}
	cur := diskReading{at: at, devices: counters}
	v.DiskRate = diskRate(h.prevDisk, cur)
	h.prevDisk = cur
}
