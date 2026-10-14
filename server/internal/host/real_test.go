package host

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	gopsnet "github.com/shirou/gopsutil/v4/net"
)

func TestFilterDisks(t *testing.T) {
	usage := map[string]DiskUsage{
		"/":            {Total: 100, Used: 40, UsedPercent: 40},
		"/data":        {Total: 500, Used: 450, UsedPercent: 90},
		"/srv/bind":    {Total: 500, Used: 450, UsedPercent: 90}, // bind of /data
		"/boot/efi":    {Total: 1, Used: 0, UsedPercent: 0},
		"/mnt/gone":    {},
		"/System/VM":   {Total: 100, Used: 40, UsedPercent: 40},
		"/run":         {Total: 10, Used: 1, UsedPercent: 10},
		"/var/lib/ovl": {Total: 100, Used: 40, UsedPercent: 40},
	}
	parts := []partitionInfo{
		{Device: "/dev/sda1", Mountpoint: "/", Fstype: "ext4"},
		{Device: "/dev/sdb1", Mountpoint: "/srv/bind", Fstype: "xfs"},
		{Device: "/dev/sdb1", Mountpoint: "/data", Fstype: "xfs"},
		{Device: "/dev/sda2", Mountpoint: "/boot/efi", Fstype: "vfat"},
		{Device: "/dev/sdc1", Mountpoint: "/mnt/gone", Fstype: "ext4"}, // unreadable
		{Device: "/dev/disk3s6", Mountpoint: "/System/VM", Fstype: "apfs", Opts: []string{"rw", "nobrowse"}},
		{Device: "tmpfs", Mountpoint: "/run", Fstype: "tmpfs"},
		{Device: "overlay", Mountpoint: "/var/lib/ovl", Fstype: "overlay"},
	}
	got := filterDisks(parts, func(path string) (DiskUsage, error) {
		if path == "/mnt/gone" {
			return DiskUsage{}, errors.New("permission denied")
		}
		return usage[path], nil
	})
	want := []DiskUsage{
		{Path: "/", Fstype: "ext4", Total: 100, Used: 40, UsedPercent: 40},
		{Path: "/boot/efi", Fstype: "vfat", Total: 1, Used: 0, UsedPercent: 0},
		{Path: "/data", Fstype: "xfs", Total: 500, Used: 450, UsedPercent: 90},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterDisks =\n %+v\nwant\n %+v", got, want)
	}
}

func TestFilterDisksEmpty(t *testing.T) {
	got := filterDisks(nil, func(string) (DiskUsage, error) { return DiskUsage{}, nil })
	if got == nil || len(got) != 0 {
		t.Errorf("want empty non-nil slice, got %#v", got)
	}
}

func TestContainerDetailRejectsPathTricks(t *testing.T) {
	h := &RealHost{DockerSocket: "/nonexistent.sock"}
	for _, id := range []string{"../images/json", "a/b", "", "x?y"} {
		if _, err := h.ContainerDetail(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("ContainerDetail(%q) err = %v, want ErrNotFound", id, err)
		}
	}
	if _, err := h.ContainerDetail("abc123"); !errors.Is(err, ErrDockerUnavailable) {
		t.Errorf("missing socket err = %v, want ErrDockerUnavailable", err)
	}
}

func TestParsePSI(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		some, full float64
		ok         bool
	}{
		{"memory", "some avg10=1.25 avg60=0.40 avg300=0.08 total=123456\nfull avg10=0.50 avg60=0.10 avg300=0.02 total=789\n", 1.25, 0.50, true},
		{"cpu without full", "some avg10=12.00 avg60=3.00 avg300=1.00 total=1\n", 12, 0, true},
		{"empty", "", 0, 0, false},
		{"garbage", "some avg10=nope\n", 0, 0, false},
	}
	for _, c := range cases {
		some, full, ok := parsePSI(c.in)
		if some != c.some || full != c.full || ok != c.ok {
			t.Errorf("%s: parsePSI = (%v, %v, %v), want (%v, %v, %v)", c.name, some, full, ok, c.some, c.full, c.ok)
		}
	}
}

func TestReadPressure(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("cpu", "some avg10=4.00 avg60=0 avg300=0 total=0\nfull avg10=9.99 avg60=0 avg300=0 total=0\n")
	write("memory", "some avg10=2.50 avg60=0 avg300=0 total=0\nfull avg10=1.00 avg60=0 avg300=0 total=0\n")
	if p := readPressure(dir); p != nil {
		t.Fatalf("missing io file: want nil, got %+v", p)
	}
	write("io", "some avg10=30.00 avg60=0 avg300=0 total=0\nfull avg10=20.00 avg60=0 avg300=0 total=0\n")
	want := &Pressure{CPUSome: 4, MemorySome: 2.5, MemoryFull: 1, IOSome: 30, IOFull: 20}
	if got := readPressure(dir); !reflect.DeepEqual(got, want) {
		t.Errorf("readPressure = %+v, want %+v", got, want)
	}
	if p := readPressure(filepath.Join(dir, "absent")); p != nil {
		t.Errorf("absent dir: want nil, got %+v", p)
	}
}

func TestNetRate(t *testing.T) {
	t0 := time.Unix(1000, 0)
	counters := []gopsnet.IOCountersStat{
		{Name: "lo0", BytesRecv: 1 << 30, BytesSent: 1 << 30},
		{Name: "eth0", BytesRecv: 1000, BytesSent: 500, PacketsRecv: 10, PacketsSent: 5},
	}
	loop := map[string]bool{"lo0": true}
	prev := netReading{at: t0, nics: countedNICs(counters, loop)}
	if _, ok := prev.nics["lo0"]; ok {
		t.Fatal("loopback must be excluded")
	}
	if r := netRate(netReading{}, prev); r != nil {
		t.Errorf("no baseline: want nil, got %+v", r)
	}

	next := []gopsnet.IOCountersStat{
		{Name: "lo0", BytesRecv: 2 << 30, BytesSent: 2 << 30},
		{Name: "eth0", BytesRecv: 3000, BytesSent: 100, PacketsRecv: 30, PacketsSent: 15}, // tx wrapped
		{Name: "wg0", BytesRecv: 9999, BytesSent: 9999},                                   // new: no baseline
	}
	cur := netReading{at: t0.Add(2 * time.Second), nics: countedNICs(next, loop)}
	want := &NetRate{RxBytes: 1000, TxBytes: 50, RxPackets: 10, TxPackets: 5}
	if got := netRate(prev, cur); !reflect.DeepEqual(got, want) {
		t.Errorf("netRate = %+v, want %+v", got, want)
	}
	if r := netRate(cur, cur); r != nil {
		t.Errorf("zero window: want nil, got %+v", r)
	}
}

func TestCPUPercent(t *testing.T) {
	now := time.UnixMilli(100_000)
	cur := procCPU{created: 40_000, total: 30}
	// Same process seen 10s ago with 25s of CPU: 5s over 10s = 50%.
	if got := cpuPercent(procCPU{created: 40_000, total: 25}, true, cur, 10, now); got != 50 {
		t.Errorf("windowed = %v, want 50", got)
	}
	// No baseline: lifetime average, 30s CPU over 60s alive = 50%.
	if got := cpuPercent(procCPU{}, false, procCPU{created: 40_000, total: 30}, 10, now); got != 50 {
		t.Errorf("lifetime = %v, want 50", got)
	}
	// Recycled pid (different start time) falls back to lifetime too.
	if got := cpuPercent(procCPU{created: 1, total: 1}, true, procCPU{created: 40_000, total: 6}, 10, now); got != 10 {
		t.Errorf("recycled pid = %v, want 10", got)
	}
}
