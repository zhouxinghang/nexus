// Package host defines the Host port: the single seam through which Nexus
// interacts with the live machine it runs on. It has three capability groups —
// filesystem, metrics, and docker — per the Nexus spec. A real implementation
// runs on the Host; a fake is injected in tests.
package host

import (
	"io"
	"time"
)

// FileInfo describes a single filesystem entry within a Managed Root.
type FileInfo struct {
	Path       string    `json:"path"` // absolute path on the Host
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	IsDir      bool      `json:"is_dir"`
	ModifiedAt time.Time `json:"modified_at"`
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

// Vitals is a single point-in-time reading of the Host's health.
type Vitals struct {
	CPUPercent float64   `json:"cpu_percent"`
	MemTotal   uint64    `json:"mem_total"`
	MemUsed    uint64    `json:"mem_used"`
	DiskTotal  uint64    `json:"disk_total"`
	DiskUsed   uint64    `json:"disk_used"`
	NetRx      uint64    `json:"net_rx"`
	NetTx      uint64    `json:"net_tx"`
	Load1      float64   `json:"load1"`
	Load5      float64   `json:"load5"`
	Load15     float64   `json:"load15"`
	UptimeSecs uint64    `json:"uptime_secs"`
	TopProcs   []Process `json:"top_procs"`
}

// DockerStatus reports the Docker containers, or that Docker is not enabled.
type DockerStatus struct {
	Enabled    bool        `json:"enabled"`
	Containers []Container `json:"containers"`
}

// FS is the filesystem capability: list/stat within a Managed Root and mutate
// files (rename/move/delete). All operations take absolute paths.
type FS interface {
	// Stat returns info for a single path.
	Stat(path string) (FileInfo, error)
	// List returns the direct children of a directory (non-recursive).
	List(dir string) ([]FileInfo, error)
	// Walk returns every regular file beneath root (recursive).
	Walk(root string) ([]FileInfo, error)
	// Open opens a file's contents for reading/download.
	Open(path string) (io.ReadCloser, error)
	// Rename renames/moves a file from oldPath to newPath.
	Rename(oldPath, newPath string) error
	// Remove permanently deletes a file (no trash).
	Remove(path string) error
	// MkdirAll ensures a directory exists.
	MkdirAll(path string) error
}

// Metrics is the metrics capability: a single Sample() of the Host's vitals.
type Metrics interface {
	Sample() (Vitals, error)
}

// Docker is the docker capability: list containers or report unavailable.
type Docker interface {
	Status() DockerStatus
}

// Host bundles the three capability groups into the single port.
type Host interface {
	FS
	Metrics
	Docker
}
