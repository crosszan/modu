//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris

package sessionipc

import "os"

func currentUID() int { return 0 }

func ownedByCurrentUser(info os.FileInfo) bool { return true }
