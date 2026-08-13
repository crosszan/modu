//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package sessionipc

import (
	"fmt"
	"os"
)

func acquireEndpointLock(path string) (*os.File, error) {
	return nil, fmt.Errorf("session IPC advisory locks are not supported on this platform")
}

func releaseEndpointLock(file *os.File) {
	if file != nil {
		_ = file.Close()
	}
}
