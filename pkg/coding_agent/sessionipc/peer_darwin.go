//go:build darwin

package sessionipc

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

func validatePeer(conn *net.UnixConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var peerUID uint32
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		credential, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			controlErr = err
			return
		}
		peerUID = credential.Uid
	}); err != nil {
		return err
	}
	if controlErr != nil {
		return controlErr
	}
	if int(peerUID) != os.Geteuid() {
		return fmt.Errorf("session IPC peer uid %d does not match current uid", peerUID)
	}
	return nil
}
