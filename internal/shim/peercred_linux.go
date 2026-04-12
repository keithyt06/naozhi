//go:build linux

package shim

import (
	"net"
	"os"
	"syscall"
)

// checkPeerUID verifies the connecting peer has the same UID via SO_PEERCRED.
func checkPeerUID(conn net.Conn) bool {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	var cred *syscall.Ucred
	var credErr error
	raw.Control(func(fd uintptr) { //nolint:errcheck
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if credErr != nil || cred == nil {
		return false
	}
	return cred.Uid == uint32(os.Getuid())
}

// VerifyPeerUID is exported for use by the shim server's accept handler.
var VerifyPeerUID = checkPeerUID
