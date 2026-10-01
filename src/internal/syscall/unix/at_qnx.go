// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unix

import (
	"internal/abi"
	"syscall"
	"unsafe"
)

// QNX 6.5 has none of the *at functions. Package os uses its non-openat
// code paths there (root_noopenat.go, removeall_noat.go, statat_other.go);
// these constants exist for the code that is shared with other systems.
const (
	AT_EACCESS          = 0x1
	AT_FDCWD            = -0x64
	AT_REMOVEDIR        = 0x08
	AT_SYMLINK_NOFOLLOW = 0x02

	// UTIME_OMIT must match syscall.utimeOmit in syscall_qnx.go, which
	// implements it for UtimesNano.
	UTIME_OMIT = (1 << 30) - 2
)

//go:linkname syscall_syscall syscall.syscall
func syscall_syscall(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno)

func libc_eaccess_trampoline()

//go:cgo_import_dynamic libc_eaccess eaccess "libc.so.3"

// faccessat supports only what Eaccess needs, through QNX's eaccess().
func faccessat(dirfd int, path string, mode uint32, flags int) error {
	if dirfd != AT_FDCWD || flags != AT_EACCESS {
		return syscall.ENOSYS
	}
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, errno := syscall_syscall(abi.FuncPCABI0(libc_eaccess_trampoline), uintptr(unsafe.Pointer(p)), uintptr(mode), 0)
	if errno != 0 {
		return errno
	}
	return nil
}
