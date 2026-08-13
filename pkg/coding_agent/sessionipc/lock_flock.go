//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package sessionipc

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func acquireEndpointLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open session IPC lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrSessionAlreadyRunning
		}
		return nil, fmt.Errorf("lock session IPC endpoint: %w", err)
	}
	return file, nil
}

func releaseEndpointLock(file *os.File) {
	if file == nil {
		return
	}
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
	_ = file.Close()
}
