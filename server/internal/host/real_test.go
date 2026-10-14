package host

import (
	"errors"
	"reflect"
	"testing"
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
