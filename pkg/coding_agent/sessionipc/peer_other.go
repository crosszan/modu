//go:build !darwin && !linux

package sessionipc

import "net"

// Directory and socket ownership checks remain in force on platforms where
// this package has no peer-credential implementation.
func validatePeer(conn *net.UnixConn) error { return nil }
