// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unix

import (
	"internal/abi"
	"syscall"
	"unsafe"
)

func libc__cmdname_trampoline()

//go:cgo_import_dynamic libc__cmdname _cmdname "libc.so.3"

// Cmdname returns the absolute path the running program was started
// from, as QNX's _cmdname reports it: the path given to exec, joined to
// the working directory of the time. Unlike os.Args[0], it does not
// depend on what the parent passed as the program's name.
func Cmdname() (string, error) {
	var buf [1024 + 1]byte // PATH_MAX, and its NUL
	r, _, errno := syscall_syscall(abi.FuncPCABI0(libc__cmdname_trampoline), uintptr(unsafe.Pointer(&buf[0])), 0, 0)
	if r == 0 {
		if errno != 0 {
			return "", errno
		}
		return "", syscall.ENOENT
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i]), nil
		}
	}
	return "", syscall.ENAMETOOLONG
}
