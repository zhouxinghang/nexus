package host

import (
	"os"
	"syscall"
	"time"
)

func fileCreationTime(_ string, fi os.FileInfo) *time.Time {
	stat, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok || (stat.CreationTime.HighDateTime == 0 && stat.CreationTime.LowDateTime == 0) {
		return nil
	}
	created := time.Unix(0, stat.CreationTime.Nanoseconds()).UTC().Truncate(time.Microsecond)
	return &created
}
