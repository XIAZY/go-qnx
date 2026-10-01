// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unix

import (
	"internal/abi"
	"unsafe"
)

// QNX confstr names (<confname.h>).
const (
	CS_DOMAIN  = 201
	CS_RESOLVE = 202 // the in-memory resolv.conf
)

func libc_confstr_trampoline()

//go:cgo_import_dynamic libc_confstr confstr "libc.so.3"

// Confstr returns the value of the configuration string name, or "" if
// it is not set.
func Confstr(name int) string {
	var buf [1024]byte
	n, _, _ := syscall_syscall(abi.FuncPCABI0(libc_confstr_trampoline), uintptr(name), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return "" // unset, or an invalid name
	}
	if n > uintptr(len(buf)) {
		b := make([]byte, n)
		n, _, _ = syscall_syscall(abi.FuncPCABI0(libc_confstr_trampoline), uintptr(name), uintptr(unsafe.Pointer(&b[0])), n)
		if n == 0 || n > uintptr(len(b)) {
			return ""
		}
		return string(b[:n-1])
	}
	return string(buf[:n-1]) // n counts the terminating NUL
}
