package host

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// RealHost is the production Host implementation. It reads the live machine's
// filesystem and vitals and talks to the local Docker socket.
type RealHost struct {
	// DockerSocket is the path to the Docker daemon socket; empty disables.
	DockerSocket string
	// TopN is how many processes to include in the top-process lists.
	TopN int
}

// NewRealHost builds a RealHost with sensible defaults.
func NewRealHost() *RealHost {
	return &RealHost{DockerSocket: "/var/run/docker.sock", TopN: 5}
}

// --- FS capability ---

func toFileInfo(path string, fi os.FileInfo) FileInfo {
	return FileInfo{
		Path:       path,
		Name:       fi.Name(),
		Size:       fi.Size(),
		IsDir:      fi.IsDir(),
		ModifiedAt: fi.ModTime(),
	}
}

func (h *RealHost) Stat(path string) (FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}
	return toFileInfo(path, fi), nil
}

func (h *RealHost) List(dir string) ([]FileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, toFileInfo(filepath.Join(dir, e.Name()), fi))
	}
	return out, nil
}

func (h *RealHost) Walk(root string) ([]FileInfo, error) {
	var out []FileInfo
	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if fi.IsDir() {
			return nil
		}
		out = append(out, toFileInfo(path, fi))
		return nil
	})
	return out, err
}

func (h *RealHost) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (h *RealHost) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (h *RealHost) Remove(path string) error {
	return os.Remove(path)
}

func (h *RealHost) MkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

// --- Metrics capability ---

func (h *RealHost) Sample() (Vitals, error) {
	var v Vitals

	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		v.CPUPercent = pcts[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		v.MemTotal = vm.Total
		v.MemUsed = vm.Used
	}
	if du, err := disk.Usage("/"); err == nil {
		v.DiskTotal = du.Total
		v.DiskUsed = du.Used
	}
	if counters, err := gopsnet.IOCounters(false); err == nil && len(counters) > 0 {
		v.NetRx = counters[0].BytesRecv
		v.NetTx = counters[0].BytesSent
	}
	if avg, err := load.Avg(); err == nil {
		v.Load1 = avg.Load1
		v.Load5 = avg.Load5
		v.Load15 = avg.Load15
	}
	if up, err := host.Uptime(); err == nil {
		v.UptimeSecs = up
	}
	v.TopProcs = h.topProcesses()
	return v, nil
}

func (h *RealHost) topProcesses() []Process {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	all := make([]Process, 0, len(procs))
	for _, p := range procs {
		name, _ := p.Name()
		cpuPct, _ := p.CPUPercent()
		memPct, _ := p.MemoryPercent()
		var rss uint64
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			rss = mi.RSS
		}
		all = append(all, Process{
			PID:     p.Pid,
			Name:    name,
			CPU:     cpuPct,
			MemRSS:  rss,
			MemPerc: float64(memPct),
		})
	}
	n := h.TopN
	if n <= 0 {
		n = 5
	}
	// Collect top-N by CPU and by memory, deduplicated by PID.
	byCPU := make([]Process, len(all))
	copy(byCPU, all)
	sort.Slice(byCPU, func(i, j int) bool { return byCPU[i].CPU > byCPU[j].CPU })
	byMem := make([]Process, len(all))
	copy(byMem, all)
	sort.Slice(byMem, func(i, j int) bool { return byMem[i].MemRSS > byMem[j].MemRSS })

	seen := map[int32]bool{}
	out := []Process{}
	take := func(list []Process) {
		count := 0
		for _, p := range list {
			if count >= n {
				break
			}
			if !seen[p.PID] {
				seen[p.PID] = true
				out = append(out, p)
			}
			count++
		}
	}
	take(byCPU)
	take(byMem)
	return out
}

// --- Docker capability ---

func (h *RealHost) Status() DockerStatus {
	if h.DockerSocket == "" {
		return DockerStatus{Enabled: false}
	}
	if _, err := os.Stat(h.DockerSocket); err != nil {
		return DockerStatus{Enabled: false}
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", h.DockerSocket)
			},
		},
		Timeout: 3 * time.Second,
	}
	resp, err := client.Get("http://docker/v1.41/containers/json?all=1")
	if err != nil {
		return DockerStatus{Enabled: false}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DockerStatus{Enabled: false}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DockerStatus{Enabled: false}
	}
	var raw []struct {
		ID     string   `json:"Id"`
		Names  []string `json:"Names"`
		Image  string   `json:"Image"`
		State  string   `json:"State"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return DockerStatus{Enabled: false}
	}
	containers := make([]Container, 0, len(raw))
	for _, c := range raw {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		id := c.ID
		if len(id) > 12 {
			id = id[:12]
		}
		containers = append(containers, Container{
			ID:    id,
			Name:  name,
			Image: c.Image,
			State: c.State,
		})
	}
	return DockerStatus{Enabled: true, Containers: containers}
}

var _ Host = (*RealHost)(nil)
