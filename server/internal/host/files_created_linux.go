package host

import (
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func fileCreationTime(path string, fi os.FileInfo) *time.Time {
	before, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	flags := 0
	if fi.Mode()&os.ModeSymlink != 0 {
		flags = unix.AT_SYMLINK_NOFOLLOW
	}
	var stat unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, flags, unix.STATX_BTIME|unix.STATX_INO, &stat); err != nil || stat.Mask&unix.STATX_BTIME == 0 {
		return nil // Unsupported filesystems/kernels do not have a birth time.
	}
	// The path may have changed after Stat/Open; never combine two files'
	// metadata, including when a symlink is replaced during a confined read.
	if stat.Ino != before.Ino || unix.Mkdev(stat.Dev_major, stat.Dev_minor) != uint64(before.Dev) {
		return nil
	}
	created := time.Unix(stat.Btime.Sec, int64(stat.Btime.Nsec)).UTC().Truncate(time.Microsecond)
	return &created
}
