// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Supporting definitions for os_uname.go on QNX, whose syscall package
// has no Uname.

package osinfo

import (
	"internal/abi"
	"syscall"
	"unsafe"
)

type utsname = syscall.Utsname

//go:cgo_import_dynamic libc_uname uname "libc.so.3"

func libc_uname_trampoline()

//go:linkname rawSyscall syscall.rawSyscall
func rawSyscall(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno)

func uname(buf *utsname) error {
	_, _, errno := rawSyscall(abi.FuncPCABI0(libc_uname_trampoline), uintptr(unsafe.Pointer(buf)), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
