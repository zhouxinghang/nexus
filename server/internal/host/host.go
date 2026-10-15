// Package host defines the Host port: the single seam through which Nexus
// interacts with the live machine it runs on. It has three capability groups —
// filesystem, metrics, and docker — per the Nexus spec. A real implementation
// runs on the Host; a fake is injected in tests.
package host

import (
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned by the detail lookups when the process or container
// no longer exists.
var ErrNotFound = errors.New("not found")

// ErrDockerUnavailable is returned by ContainerDetail when Docker isn't
// running or its socket can't be reached.
var ErrDockerUnavailable = errors.New("docker is not available")

// FileInfo describes a single filesystem entry within a Managed Root.
type FileInfo struct {
	Path       string     `json:"path"` // absolute path on the Host
	Name       string     `json:"name"`
	Size       int64      `json:"size"`
	IsDir      bool       `json:"is_dir"`
	ModifiedAt time.Time  `json:"modified_at"`
	CreatedAt  *time.Time `json:"created_at"` // filesystem birth time; nil if unavailable
}

// Process is one entry in the top-N process list.
type Process struct {
	PID     int32   `json:"pid"`
	Name    string  `json:"name"`
	CPU     float64 `json:"cpu"`     // percent
	MemRSS  uint64  `json:"mem_rss"` // resident set size in bytes
	MemPerc float64 `json:"mem_perc"`
}

// Container is one Docker container's state.
type Container struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"`
}

// DiskUsage is one mounted filesystem's usage in the current snapshot.
type DiskUsage struct {
	Path        string  `json:"path"`   // mount point
	Fstype      string  `json:"fstype"` // e.g. ext4, apfs, xfs
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
}

// ProcessDetail is the read-only drill-in view of one process. Fields the OS
// won't reveal (e.g. another user's command line) are left empty.
type ProcessDetail struct {
	PID        int32     `json:"pid"`
	PPID       int32     `json:"ppid"`
	Name       string    `json:"name"`
	Exe        string    `json:"exe"`
	Cmdline    string    `json:"cmdline"`
	User       string    `json:"user"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	NumThreads int32     `json:"num_threads"`
	CPU        float64   `json:"cpu"`     // percent
	MemRSS     uint64    `json:"mem_rss"` // bytes
	MemPerc    float64   `json:"mem_perc"`
}

// PortBinding is one published container port.
type PortBinding struct {
	ContainerPort string `json:"container_port"` // e.g. "5432/tcp"
	HostIP        string `json:"host_ip"`
	HostPort      string `json:"host_port"`
}

// Mount is one container mount (bind or volume).
type Mount struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"read_only"`
}

// ContainerDetail is the read-only drill-in view of one Docker container.
type ContainerDetail struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Image     string        `json:"image"`
	State     string        `json:"state"`  // running, exited, …
	Status    string        `json:"status"` // human string, e.g. "Up 2 hours"
	CreatedAt time.Time     `json:"created_at"`
	StartedAt *time.Time    `json:"started_at"`
	Command   string        `json:"command"`
	Ports     []PortBinding `json:"ports"`
	Mounts    []Mount       `json:"mounts"`
}

// NetRate is network throughput over the window since an earlier reading,
// summed across the interfaces that count toward the Host (see countedNIC).
type NetRate struct {
	RxBytes   float64 `json:"rx_bytes_per_sec"`
	TxBytes   float64 `json:"tx_bytes_per_sec"`
	RxPackets float64 `json:"rx_packets_per_sec"`
	TxPackets float64 `json:"tx_packets_per_sec"`
}

// DiskRate is whole-device Host storage throughput over the sampling window,
// not filesystem capacity or Nexus-only file activity. Operations are IOPS.
type DiskRate struct {
	ReadBytes  float64 `json:"read_bytes_per_sec"`
	WriteBytes float64 `json:"write_bytes_per_sec"`
	ReadOps    float64 `json:"read_ops_per_sec"`
	WriteOps   float64 `json:"write_ops_per_sec"`
}

// Pressure is Linux pressure-stall information (PSI): the percentage of the
// last 10 seconds (avg10) in which some / all non-idle tasks were stalled
// waiting on a resource. CPU "full" is undefined system-wide, so it's omitted.
type Pressure struct {
	CPUSome    float64 `json:"cpu_some"`
	MemorySome float64 `json:"memory_some"`
	MemoryFull float64 `json:"memory_full"`
	IOSome     float64 `json:"io_some"`
	IOFull     float64 `json:"io_full"`
}

// SelfUsage is the Nexus process's own footprint, so the monitor's overhead
// is visible next to the Host it measures.
type SelfUsage struct {
	CPU    float64 `json:"cpu"`     // percent of one core, like Process.CPU
	MemRSS uint64  `json:"mem_rss"` // bytes
}

// Vitals is a single point-in-time reading of the Host's health.
type Vitals struct {
	CPUPercent float64 `json:"cpu_percent"`
	CPUCores   int     `json:"cpu_cores"` // logical CPUs
	MemTotal   uint64  `json:"mem_total"`
	// MemUsed is Total − Available: memory that can't be reclaimed without
	// swapping. Droppable page cache doesn't count as used.
	MemUsed uint64 `json:"mem_used"`
	// DiskTotal/DiskUsed retain root filesystem capacity for Overview and
	// API compatibility; Servers displays DiskRate separately from capacity.
	DiskTotal uint64      `json:"disk_total"`
	DiskUsed  uint64      `json:"disk_used"`
	Disks     []DiskUsage `json:"disks"`     // per-mount, current snapshot only
	DiskRate  *DiskRate   `json:"disk_rate"` // nil before a usable rate exists
	// NetRx/NetTx are since-boot byte counters over the non-loopback
	// interfaces; NetRate is the current throughput, nil on the first reading.
	NetRx      uint64     `json:"net_rx"`
	NetTx      uint64     `json:"net_tx"`
	NetRate    *NetRate   `json:"net_rate"`
	Load1      float64    `json:"load1"`
	Load5      float64    `json:"load5"`
	Load15     float64    `json:"load15"`
	Pressure   *Pressure  `json:"pressure"` // nil off Linux or with PSI disabled
	UptimeSecs uint64     `json:"uptime_secs"`
	Self       *SelfUsage `json:"self"` // nil when unreadable
	TopProcs   []Process  `json:"top_procs"`
}

// DockerStatus reports the Docker containers, or that Docker is not enabled.
type DockerStatus struct {
	Enabled    bool        `json:"enabled"`
	Containers []Container `json:"containers"`
}

// FS is the filesystem capability: list/stat within a Managed Root and mutate
// files (rename/move/delete and revision-checked text edits). All operations
// take absolute paths.
type FS interface {
	// Stat returns info for a single path.
	Stat(path string) (FileInfo, error)
	// List returns the direct children of a directory (non-recursive).
	List(dir string) ([]FileInfo, error)
	// ListWithinRoot returns non-hidden direct directories, confined to root.
	ListWithinRoot(root, path string) ([]FileInfo, error)
	// Walk returns files beneath root, excluding dot-prefixed entries and
	// entire dot-prefixed directory subtrees.
	Walk(root string) ([]FileInfo, error)
	// Open opens a file's contents for reading/download.
	Open(path string) (io.ReadCloser, error)
	// OpenWithinRoot opens only a regular file confined to the Managed Root,
	// returning current disk metadata and whether it can be edited.
	OpenWithinRoot(root, path string) (io.ReadSeekCloser, FileInfo, bool, error)
	// WriteText atomically replaces a regular file if its bytes still match
	// revision. Symbolic links are never replaced.
	WriteText(root, path string, data []byte, revision string) (FileInfo, error)
	// Rename renames/moves a file from oldPath to newPath.
	Rename(oldPath, newPath string) error
	// Remove permanently deletes a file (no trash).
	Remove(path string) error
	// MkdirAll ensures a directory exists.
	MkdirAll(path string) error
}

// Metrics is the metrics capability: a single Sample() of the Host's vitals,
// plus a read-only detail lookup for one process.
type Metrics interface {
	Sample() (Vitals, error)
	// ProcessDetail returns ErrNotFound when the pid is gone.
	ProcessDetail(pid int32) (ProcessDetail, error)
}

// Docker is the docker capability: list containers or report unavailable,
// plus a read-only detail lookup for one container. Monitoring only — there
// is deliberately no start/stop/exec.
type Docker interface {
	Status() DockerStatus
	// ContainerDetail returns ErrDockerUnavailable when Docker is off and
	// ErrNotFound when the id is gone.
	ContainerDetail(id string) (ContainerDetail, error)
}

// Host bundles the three capability groups into the single port.
type Host interface {
	FS
	Metrics
	Docker
}
