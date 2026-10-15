package host

import (
	"os"
	"syscall"
	"time"
)

func fileCreationTime(_ string, fi os.FileInfo) *time.Time {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || (stat.Birthtimespec.Sec == 0 && stat.Birthtimespec.Nsec == 0) {
		return nil
	}
	created := time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec).UTC().Truncate(time.Microsecond)
	return &created
}
