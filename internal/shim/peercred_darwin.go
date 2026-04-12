//go:build darwin

package shim

import (
	"net"
	"os"
	"syscall"
	"unsafe"
)

// checkPeerUID verifies the connecting peer has the same UID via LOCAL_PEERCRED.
func checkPeerUID(conn net.Conn) bool {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	var uid uint32
	var credErr error
	raw.Control(func(fd uintptr) { //nolint:errcheck
		// struct xucred { u_int cr_version; uid_t cr_uid; short cr_ngroups; gid_t cr_groups[16]; }
		var xucred [19]uint32 // xucred is 76 bytes
		xucredLen := uint32(unsafe.Sizeof(xucred))
		_, _, errno := syscall.Syscall6(
			syscall.SYS___SYSCTL, 0, 0, 0, 0, 0, 0, // unused, we use getsockopt
		)
		_ = errno
		// Use getsockopt with LOCAL_PEERCRED
		_, _, errno = syscall.Syscall6(
			syscall.SYS_GETSOCKOPT,
			fd,
			0, // SOL_LOCAL = 0 on darwin
			1, // LOCAL_PEERCRED = 1 on darwin
			uintptr(unsafe.Pointer(&xucred[0])),
			uintptr(unsafe.Pointer(&xucredLen)),
			0,
		)
		if errno != 0 {
			credErr = errno
			return
		}
		uid = xucred[1] // cr_uid is the second field
	})
	if credErr != nil {
		return false
	}
	return uid == uint32(os.Getuid())
}

// VerifyPeerUID is exported for use by the shim server's accept handler.
var VerifyPeerUID = checkPeerUID
