package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	// Rates (disk/network throughput, per-process CPU) are deltas between
	// consecutive readings, so the previous ones are kept here. mu also
	// serialises Sample so overlapping callers don't race the baselines.
	mu       sync.Mutex
	last     Vitals
	lastAt   time.Time
	prevNet  netReading
	prevDisk diskReading
	prevProc procReading
}

// minSampleGap is the shortest window a rate is computed over. Sample calls
// closer together than this (the sampler and a dashboard poll colliding)
// share one reading instead of producing a noisy sub-second rate.
const minSampleGap = time.Second

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
		CreatedAt:  fileCreationTime(path, fi),
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
	return walkFiles(root)
}

func (h *RealHost) ListWithinRoot(root, path string) ([]FileInfo, error) {
	return listWithinRoot(root, path)
}

func (h *RealHost) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (h *RealHost) OpenWithinRoot(root, path string) (io.ReadSeekCloser, FileInfo, bool, error) {
	return openWithinRoot(root, path)
}

func (h *RealHost) WriteText(root, path string, data []byte, revision string) (FileInfo, error) {
	return writeText(root, path, data, revision)
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

// Sample reads the Host's vitals. The resource set follows what Bazel's
// profiler tracks for the machine it builds on (LocalResourceUsageCollectors,
// NetworkMetricsCollector): CPU against the core count, memory as
// Total − Available, non-loopback network throughput in bytes and packets per
// second, Linux pressure-stall information, and the monitor's own CPU/RSS.
func (h *RealHost) Sample() (Vitals, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	if !h.lastAt.IsZero() && now.Sub(h.lastAt) < minSampleGap {
		return h.last, nil
	}

	var v Vitals
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		v.CPUPercent = pcts[0]
	}
	if n, err := cpu.Counts(true); err == nil {
		v.CPUCores = n
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		v.MemTotal = vm.Total
		if vm.Available < vm.Total {
			v.MemUsed = vm.Total - vm.Available
		}
	}
	if du, err := disk.Usage("/"); err == nil {
		v.DiskTotal = du.Total
		v.DiskUsed = du.Used
	}
	v.Disks = diskUsages()
	diskCounters, diskErr := diskIOCounters()
	h.sampleDisk(&v, time.Now(), diskCounters, diskErr)
	if counters, err := gopsnet.IOCounters(true); err == nil {
		cur := netReading{at: now, nics: countedNICs(counters, loopbackNICs())}
		for _, c := range cur.nics {
			v.NetRx += c.BytesRecv
			v.NetTx += c.BytesSent
		}
		v.NetRate = netRate(h.prevNet, cur)
		h.prevNet = cur
	}
	if avg, err := load.Avg(); err == nil {
		v.Load1 = avg.Load1
		v.Load5 = avg.Load5
		v.Load15 = avg.Load15
	}
	v.Pressure = readPressure(pressureDir)
	if up, err := host.Uptime(); err == nil {
		v.UptimeSecs = up
	}
	v.TopProcs, v.Self = h.processes(now, v.MemTotal)

	h.last, h.lastAt = v, now
	return v, nil
}

// --- network throughput ---

// netReading is one snapshot of the per-interface counters that count.
type netReading struct {
	at   time.Time
	nics map[string]gopsnet.IOCountersStat
}

// loopbackNICs names the loopback interfaces. On error it's empty, so
// nothing is excluded rather than everything.
func loopbackNICs() map[string]bool {
	out := map[string]bool{}
	ifs, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifs {
		if ifi.Flags&net.FlagLoopback != 0 {
			out[ifi.Name] = true
		}
	}
	return out
}

// countedNICs keeps the interfaces whose traffic counts toward the Host:
// everything but loopback, which only carries the Host talking to itself
// (Nexus ↔ Postgres on localhost, for one) and would swamp real traffic.
func countedNICs(counters []gopsnet.IOCountersStat, loopback map[string]bool) map[string]gopsnet.IOCountersStat {
	out := make(map[string]gopsnet.IOCountersStat, len(counters))
	for _, c := range counters {
		if !loopback[c.Name] {
			out[c.Name] = c
		}
	}
	return out
}

// netRate is the per-second throughput between two readings, summed over
// the interfaces present in both. It's nil without a usable earlier reading.
func netRate(prev, cur netReading) *NetRate {
	secs := cur.at.Sub(prev.at).Seconds()
	if prev.nics == nil || secs <= 0 {
		return nil
	}
	var r NetRate
	for name, c := range cur.nics {
		p, ok := prev.nics[name]
		if !ok {
			continue // appeared since the last reading: no baseline yet
		}
		r.RxBytes += float64(counterDelta(p.BytesRecv, c.BytesRecv))
		r.TxBytes += float64(counterDelta(p.BytesSent, c.BytesSent))
		r.RxPackets += float64(counterDelta(p.PacketsRecv, c.PacketsRecv))
		r.TxPackets += float64(counterDelta(p.PacketsSent, c.PacketsSent))
	}
	r.RxBytes /= secs
	r.TxBytes /= secs
	r.RxPackets /= secs
	r.TxPackets /= secs
	return &r
}

// counterDelta is how much a monotonic counter grew between readings. One
// that went backwards wrapped or was reset (e.g. a driver reload), so its new
// value is the growth since — the same rule as Bazel's network collector.
func counterDelta(prev, cur uint64) uint64 {
	if cur < prev {
		return cur
	}
	return cur - prev
}

// --- pressure stall information ---

// pressureDir is where Linux (4.20+, CONFIG_PSI) exposes PSI.
const pressureDir = "/proc/pressure"

// readPressure reads cpu/memory/io PSI from dir. It's nil when any file is
// missing or unparsable — not Linux, an old kernel, or PSI disabled.
func readPressure(dir string) *Pressure {
	read := func(name string) (some, full float64, ok bool) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return 0, 0, false
		}
		return parsePSI(string(b))
	}
	cpuSome, _, okCPU := read("cpu")
	memSome, memFull, okMem := read("memory")
	ioSome, ioFull, okIO := read("io")
	if !okCPU || !okMem || !okIO {
		return nil
	}
	return &Pressure{CPUSome: cpuSome, MemorySome: memSome, MemoryFull: memFull, IOSome: ioSome, IOFull: ioFull}
}

// parsePSI extracts the avg10 figures from one /proc/pressure/* file:
//
//	some avg10=1.25 avg60=0.40 avg300=0.08 total=123456
//	full avg10=0.00 avg60=0.00 avg300=0.00 total=0
//
// ok is false without a parsable "some" line; a missing "full" line is 0.
func parsePSI(s string) (some, full float64, ok bool) {
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		for _, f := range fields[1:] {
			raw, found := strings.CutPrefix(f, "avg10=")
			if !found {
				continue
			}
			avg10, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				break
			}
			switch fields[0] {
			case "some":
				some, ok = avg10, true
			case "full":
				full = avg10
			}
			break
		}
	}
	return some, full, ok
}

// --- processes ---

// procCPU is one process's cumulative CPU time. The start time rides along so
// a recycled pid isn't mistaken for the process it replaced.
type procCPU struct {
	created int64   // unix ms
	total   float64 // user + system seconds
}

// procReading is the CPU times of every process at one moment.
type procReading struct {
	at   time.Time
	cpus map[int32]procCPU
}

// cpuPercent is a process's CPU use in percent of one core. With a baseline
// for the same process it's the rate over the window — what top shows. On the
// first reading, or for a process started since, it falls back to the
// lifetime average.
func cpuPercent(prev procCPU, hasPrev bool, cur procCPU, window float64, now time.Time) float64 {
	if hasPrev && prev.created == cur.created && window > 0 && cur.total >= prev.total {
		return 100 * (cur.total - prev.total) / window
	}
	if cur.created <= 0 {
		return 0
	}
	life := now.Sub(time.UnixMilli(cur.created)).Seconds()
	if life <= 0 {
		return 0
	}
	return 100 * cur.total / life
}

// readProcCPU reads p's cumulative CPU time; ok is false when the OS won't
// say (another user's process, or one that just exited).
func readProcCPU(p *process.Process) (procCPU, bool) {
	t, err := p.Times()
	if err != nil || t == nil {
		return procCPU{}, false
	}
	created, _ := p.CreateTime()
	return procCPU{created: created, total: t.User + t.System}, true
}

// processes lists every process once, returning the top-N lists and the
// Nexus process's own usage. CPU is a rate against the previous reading, which
// it then replaces. Callers hold h.mu.
func (h *RealHost) processes(now time.Time, memTotal uint64) ([]Process, *SelfUsage) {
	procs, err := process.Processes()
	if err != nil {
		return nil, nil
	}
	window := now.Sub(h.prevProc.at).Seconds()
	cur := procReading{at: now, cpus: make(map[int32]procCPU, len(procs))}
	selfPID := int32(os.Getpid())
	var self *SelfUsage
	all := make([]Process, 0, len(procs))
	for _, p := range procs {
		name, _ := p.Name()
		var cpuPct float64
		if c, ok := readProcCPU(p); ok {
			prev, hasPrev := h.prevProc.cpus[p.Pid]
			cpuPct = cpuPercent(prev, hasPrev, c, window, now)
			cur.cpus[p.Pid] = c
		}
		var rss uint64
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			rss = mi.RSS
		}
		var memPct float64
		if memTotal > 0 {
			memPct = 100 * float64(rss) / float64(memTotal)
		}
		if p.Pid == selfPID {
			self = &SelfUsage{CPU: cpuPct, MemRSS: rss}
		}
		all = append(all, Process{
			PID:     p.Pid,
			Name:    name,
			CPU:     cpuPct,
			MemRSS:  rss,
			MemPerc: memPct,
		})
	}
	h.prevProc = cur
	return topN(all, h.TopN), self
}

// topN collects the top n processes by CPU and by memory, deduplicated by PID.
func topN(all []Process, n int) []Process {
	if n <= 0 {
		n = 5
	}
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

// pseudoFstypes are filesystems that never represent real storage the Owner
// cares about: kernel/virtual filesystems, container layers, snap images, and
// automounter placeholders.
var pseudoFstypes = map[string]bool{
	"autofs": true, "binfmt_misc": true, "bpf": true, "cgroup": true, "cgroup2": true,
	"configfs": true, "debugfs": true, "devfs": true, "devicefs": true, "devpts": true,
	"devtmpfs": true, "efivarfs": true, "fusectl": true, "fuse.lxcfs": true,
	"fuse.gvfsd-fuse": true, "fuse.portal": true, "hugetlbfs": true, "mqueue": true,
	"nsfs": true, "nullfs": true, "overlay": true, "proc": true, "pstore": true,
	"ramfs": true, "rpc_pipefs": true, "securityfs": true, "selinuxfs": true,
	"squashfs": true, "sysfs": true, "tmpfs": true, "tracefs": true,
}

// partitionInfo is the subset of a mount table entry the disk filter needs.
type partitionInfo struct {
	Device, Mountpoint, Fstype string
	Opts                       []string
}

// diskUsages enumerates mounted filesystems and reports usage for the real
// ones (see filterDisks). Errors yield an empty list, never a failed sample.
func diskUsages() []DiskUsage {
	parts, err := disk.Partitions(false)
	if err != nil {
		return []DiskUsage{}
	}
	infos := make([]partitionInfo, 0, len(parts))
	for _, p := range parts {
		infos = append(infos, partitionInfo{Device: p.Device, Mountpoint: p.Mountpoint, Fstype: p.Fstype, Opts: p.Opts})
	}
	return filterDisks(infos, func(path string) (DiskUsage, error) {
		u, err := disk.Usage(path)
		if err != nil {
			return DiskUsage{}, err
		}
		return DiskUsage{Total: u.Total, Used: u.Used, UsedPercent: u.UsedPercent}, nil
	})
}

// filterDisks keeps one entry per real filesystem:
//   - pseudo/virtual fstypes are skipped, as are mounts whose usage can't be
//     read or that report zero size;
//   - macOS "nobrowse" mounts (system-internal APFS volumes that share the
//     boot container's numbers and that Finder hides) are skipped;
//   - when one device is mounted more than once (bind mounts), the shortest
//     mount point wins — the real mount, not a bind under it.
//
// Output is sorted by mount point.
func filterDisks(parts []partitionInfo, usage func(path string) (DiskUsage, error)) []DiskUsage {
	byDevice := map[string]DiskUsage{}
	var order []string
	for _, p := range parts {
		if pseudoFstypes[p.Fstype] || hasOpt(p.Opts, "nobrowse") {
			continue
		}
		u, err := usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		u.Path, u.Fstype = p.Mountpoint, p.Fstype
		key := p.Device
		if key == "" || key == "none" {
			key = p.Mountpoint
		}
		prev, seen := byDevice[key]
		if !seen {
			order = append(order, key)
		}
		if !seen || len(u.Path) < len(prev.Path) {
			byDevice[key] = u
		}
	}
	out := make([]DiskUsage, 0, len(order))
	for _, k := range order {
		out = append(out, byDevice[k])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func hasOpt(opts []string, want string) bool {
	for _, o := range opts {
		if o == want {
			return true
		}
	}
	return false
}

// ProcessDetail reads one process's details via gopsutil. Fields the OS won't
// reveal to this user are left empty rather than failing the lookup.
func (h *RealHost) ProcessDetail(pid int32) (ProcessDetail, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		if errors.Is(err, process.ErrorProcessNotRunning) {
			return ProcessDetail{}, ErrNotFound
		}
		return ProcessDetail{}, err
	}
	d := ProcessDetail{PID: pid}
	d.PPID, _ = p.Ppid()
	d.Name, _ = p.Name()
	d.Exe, _ = p.Exe()
	d.Cmdline, _ = p.Cmdline()
	d.User, _ = p.Username()
	if st, err := p.Status(); err == nil {
		d.Status = strings.Join(st, ", ")
	}
	if ms, err := p.CreateTime(); err == nil && ms > 0 {
		d.CreatedAt = time.UnixMilli(ms)
	}
	d.NumThreads, _ = p.NumThreads()
	if c, ok := readProcCPU(p); ok {
		// Rate against the last Sample's reading, so the drill-in agrees with
		// the top-process row it was opened from.
		h.mu.Lock()
		prev, hasPrev := h.prevProc.cpus[pid]
		now := time.Now()
		window := now.Sub(h.prevProc.at).Seconds()
		h.mu.Unlock()
		d.CPU = cpuPercent(prev, hasPrev, c, window, now)
	}
	if mp, err := p.MemoryPercent(); err == nil {
		d.MemPerc = float64(mp)
	}
	if mi, err := p.MemoryInfo(); err == nil && mi != nil {
		d.MemRSS = mi.RSS
	}
	// The process may have exited between the existence check and the reads.
	if d.Name == "" {
		if ok, _ := process.PidExists(pid); !ok {
			return ProcessDetail{}, ErrNotFound
		}
	}
	return d, nil
}

// --- Docker capability ---

// dockerClient returns an HTTP client bound to the Docker socket, or nil when
// Docker isn't configured or the socket is absent.
func (h *RealHost) dockerClient() *http.Client {
	if h.DockerSocket == "" {
		return nil
	}
	if _, err := os.Stat(h.DockerSocket); err != nil {
		return nil
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", h.DockerSocket)
			},
		},
		Timeout: 3 * time.Second,
	}
}

func (h *RealHost) Status() DockerStatus {
	client := h.dockerClient()
	if client == nil {
		return DockerStatus{Enabled: false}
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
		ID    string   `json:"Id"`
		Names []string `json:"Names"`
		Image string   `json:"Image"`
		State string   `json:"State"`
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

// containerRef matches a container id (full or short) or name, so a
// user-supplied ref can't smuggle path segments into the socket URL.
var containerRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

// ContainerDetail inspects one container through the Docker socket
// (GET /containers/{id}/json), plus the list endpoint for docker-ps's human
// status string. Read-only.
func (h *RealHost) ContainerDetail(id string) (ContainerDetail, error) {
	if !containerRef.MatchString(id) {
		return ContainerDetail{}, ErrNotFound
	}
	client := h.dockerClient()
	if client == nil {
		return ContainerDetail{}, ErrDockerUnavailable
	}
	resp, err := client.Get("http://docker/v1.41/containers/" + url.PathEscape(id) + "/json")
	if err != nil {
		return ContainerDetail{}, ErrDockerUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ContainerDetail{}, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return ContainerDetail{}, fmt.Errorf("docker inspect: status %d", resp.StatusCode)
	}
	var raw struct {
		ID      string   `json:"Id"`
		Name    string   `json:"Name"`
		Created string   `json:"Created"`
		Path    string   `json:"Path"`
		Args    []string `json:"Args"`
		State   struct {
			Status    string `json:"Status"`
			StartedAt string `json:"StartedAt"`
		} `json:"State"`
		Config struct {
			Image string `json:"Image"`
		} `json:"Config"`
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ContainerDetail{}, fmt.Errorf("decode docker inspect: %w", err)
	}

	d := ContainerDetail{
		ID:      raw.ID,
		Name:    raw.Name,
		Image:   raw.Config.Image,
		State:   raw.State.Status,
		Status:  raw.State.Status,
		Command: strings.TrimSpace(raw.Path + " " + strings.Join(raw.Args, " ")),
		Ports:   []PortBinding{},
		Mounts:  []Mount{},
	}
	if len(d.ID) > 12 {
		d.ID = d.ID[:12]
	}
	if t, err := time.Parse(time.RFC3339Nano, raw.Created); err == nil {
		d.CreatedAt = t
	}
	// Docker reports a zero time ("0001-01-01T00:00:00Z") for never-started.
	if t, err := time.Parse(time.RFC3339Nano, raw.State.StartedAt); err == nil && t.Year() > 1 {
		d.StartedAt = &t
	}
	for port, bindings := range raw.NetworkSettings.Ports {
		for _, b := range bindings {
			d.Ports = append(d.Ports, PortBinding{ContainerPort: port, HostIP: b.HostIP, HostPort: b.HostPort})
		}
	}
	sort.Slice(d.Ports, func(i, j int) bool {
		if d.Ports[i].ContainerPort != d.Ports[j].ContainerPort {
			return d.Ports[i].ContainerPort < d.Ports[j].ContainerPort
		}
		return d.Ports[i].HostIP < d.Ports[j].HostIP
	})
	for _, m := range raw.Mounts {
		d.Mounts = append(d.Mounts, Mount{Type: m.Type, Source: m.Source, Destination: m.Destination, ReadOnly: !m.RW})
	}

	// The human status ("Up 2 hours", "Exited (0) 3 days ago") only comes from
	// the list endpoint; fall back to the bare state if that call fails.
	filters, _ := json.Marshal(map[string][]string{"id": {raw.ID}})
	if lr, err := client.Get("http://docker/v1.41/containers/json?all=1&filters=" + url.QueryEscape(string(filters))); err == nil {
		defer lr.Body.Close()
		var list []struct {
			Status string `json:"Status"`
		}
		if lr.StatusCode == http.StatusOK && json.NewDecoder(lr.Body).Decode(&list) == nil && len(list) == 1 && list[0].Status != "" {
			d.Status = list[0].Status
		}
	}
	return d, nil
}

var _ Host = (*RealHost)(nil)
