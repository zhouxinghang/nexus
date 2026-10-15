//go:build !darwin && !linux && !windows

package host

import (
	"os"
	"time"
)

func fileCreationTime(_ string, _ os.FileInfo) *time.Time {
	return nil // Do not substitute modification, inode-change, or index time.
}
