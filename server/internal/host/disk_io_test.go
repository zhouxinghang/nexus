package host

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

func TestDiskRateUsesPerDeviceDeltas(t *testing.T) {
	start := time.Unix(1000, 0)
	prev := diskReading{at: start, devices: map[string]disk.IOCountersStat{
		"a":       {ReadBytes: 1000, WriteBytes: 2000, ReadCount: 10, WriteCount: 20},
		"b":       {ReadBytes: 5000, WriteBytes: 6000, ReadCount: 50, WriteCount: 60},
		"removed": {ReadBytes: 1 << 40},
	}}
	cur := diskReading{at: start.Add(2 * time.Second), devices: map[string]disk.IOCountersStat{
		"a":   {ReadBytes: 3000, WriteBytes: 3000, ReadCount: 30, WriteCount: 30},
		"b":   {ReadBytes: 9000, WriteBytes: 8000, ReadCount: 90, WriteCount: 80},
		"new": {ReadBytes: 1 << 40, WriteBytes: 1 << 40, ReadCount: 1 << 30, WriteCount: 1 << 30},
	}}
	want := &DiskRate{ReadBytes: 3000, WriteBytes: 1500, ReadOps: 30, WriteOps: 15}
	if got := diskRate(prev, cur); !reflect.DeepEqual(got, want) {
		t.Fatalf("rate = %+v, want %+v", got, want)
	}
	// All devices measured with unchanged counters means zero activity.
	idle := diskReading{at: cur.at.Add(time.Second), devices: cur.devices}
	if got := diskRate(cur, idle); got == nil || *got != (DiskRate{}) {
		t.Fatalf("idle rate = %+v, want measured zero", got)
	}
	for _, pair := range [][2]diskReading{
		{{}, cur}, {cur, cur}, {cur, prev},
		{prev, {at: cur.at, devices: map[string]disk.IOCountersStat{"new": {ReadBytes: 1 << 40}}}},
		{prev, {at: cur.at}},
	} {
		if got := diskRate(pair[0], pair[1]); got != nil {
			t.Fatalf("unusable window produced %+v", got)
		}
	}
}

func TestDiskRateResetAndReplacementNeedNewBaseline(t *testing.T) {
	start := time.Unix(1000, 0)
	counter := disk.IOCountersStat{SerialNumber: "old", ReadBytes: 1000, WriteBytes: 2000, ReadCount: 10, WriteCount: 20}
	prev := diskReading{at: start, devices: map[string]disk.IOCountersStat{"disk0": counter}}
	for _, reset := range []disk.IOCountersStat{
		{ReadBytes: 1, WriteBytes: 2000, ReadCount: 10, WriteCount: 20},
		{ReadBytes: 1000, WriteBytes: 1, ReadCount: 10, WriteCount: 20},
		{ReadBytes: 1000, WriteBytes: 2000, ReadCount: 1, WriteCount: 20},
		{ReadBytes: 1000, WriteBytes: 2000, ReadCount: 10, WriteCount: 1},
		{SerialNumber: "replacement", ReadBytes: 10000, WriteBytes: 20000, ReadCount: 100, WriteCount: 200},
	} {
		if got := diskRate(prev, diskReading{at: start.Add(time.Second), devices: map[string]disk.IOCountersStat{"disk0": reset}}); got != nil {
			t.Fatalf("reset/replacement produced a false rate: %+v", got)
		}
	}
}

func TestSampleDiskFailureAndResetRecovery(t *testing.T) {
	h := &RealHost{}
	at := time.Unix(1000, 0)
	counters := map[string]disk.IOCountersStat{"disk0": {ReadBytes: 1000, WriteBytes: 2000}}
	sample := func(data map[string]disk.IOCountersStat, err error) *DiskRate {
		t.Helper()
		var v Vitals
		h.sampleDisk(&v, at, data, err)
		at = at.Add(time.Second)
		return v.DiskRate
	}
	if got := sample(counters, nil); got != nil {
		t.Fatalf("first reading = %+v, want nil", got)
	}
	if got := sample(counters, nil); got == nil || *got != (DiskRate{}) {
		t.Fatalf("second idle reading = %+v", got)
	}
	if got := sample(counters, errors.New("unavailable")); got != nil {
		t.Fatalf("error reading = %+v", got)
	}
	if got := sample(counters, nil); got != nil {
		t.Fatalf("recovery must establish a fresh baseline, got %+v", got)
	}
	if got := sample(map[string]disk.IOCountersStat{"disk0": {ReadBytes: 1, WriteBytes: 1}}, nil); got != nil {
		t.Fatalf("reset reading = %+v", got)
	}
	if got := sample(map[string]disk.IOCountersStat{"disk0": {ReadBytes: 101, WriteBytes: 201}}, nil); got == nil || got.ReadBytes != 100 || got.WriteBytes != 200 {
		t.Fatalf("post-reset rate = %+v", got)
	}
	if got := sample(nil, nil); got != nil {
		t.Fatalf("empty observation = %+v", got)
	}
	if got := sample(counters, nil); got != nil {
		t.Fatalf("empty observation must clear the baseline, got %+v", got)
	}
}

func TestDiskDeviceNamesExcludeDuplicateAccountingLayers(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"sda", "nvme0n1", "vda", "mmcblk0", "dm-0", "md0", "loop0", "ram0", "zram0"} {
		if err := os.MkdirAll(filepath.Join(dir, name, "slaves"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"dm-0", "md0"} {
		if err := os.WriteFile(filepath.Join(dir, name, "slaves", "sda"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A disappeared drive (no slaves directory) is not a candidate.
	if err := os.Mkdir(filepath.Join(dir, "removed"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := diskDeviceNames(dir)
	want := []string{"mmcblk0", "nvme0n1", "sda", "vda"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("devices = %v err=%v, want %v", got, err, want)
	}
	if _, err := diskDeviceNames(filepath.Join(dir, "absent")); err == nil {
		t.Fatal("unreadable topology must not fall back to unfiltered counters")
	}
}

func TestLiveDiskCountersCanProduceFiniteRates(t *testing.T) {
	first, err := diskIOCounters()
	if err != nil || len(first) == 0 {
		t.Skipf("Host disk counters unavailable: %v", err)
	}
	h := &RealHost{}
	var v Vitals
	h.sampleDisk(&v, time.Now(), first, nil)
	if v.DiskRate != nil {
		t.Fatal("live first reading fabricated a rate")
	}
	time.Sleep(20 * time.Millisecond)
	second, err := diskIOCounters()
	if err != nil {
		t.Fatal(err)
	}
	h.sampleDisk(&v, time.Now(), second, nil)
	if v.DiskRate == nil {
		t.Fatal("two live readings did not produce a rate")
	}
	for _, rate := range []float64{v.DiskRate.ReadBytes, v.DiskRate.WriteBytes, v.DiskRate.ReadOps, v.DiskRate.WriteOps} {
		if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			t.Fatalf("invalid live rate: %+v", v.DiskRate)
		}
	}
	t.Logf("live disk I/O: %+v (%d storage devices)", *v.DiskRate, len(second))
}
