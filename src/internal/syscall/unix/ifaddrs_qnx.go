// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unix

import (
	"internal/abi"
	"syscall"
	"unsafe"
)

// Ifaddrs is struct ifaddrs (<ifaddrs.h>).
type Ifaddrs struct {
	Next    *Ifaddrs
	Name    *byte
	Flags   uint32
	Addr    *syscall.RawSockaddr
	Netmask *syscall.RawSockaddr
	Dstaddr *syscall.RawSockaddr
	Data    unsafe.Pointer // struct if_data for AF_LINK entries
}

// IfDataMTUOffset is the offset of ifi_mtu in struct if_data (<net/if.h>).
const IfDataMTUOffset = 8

func libc_getifaddrs_trampoline()
func libc_freeifaddrs_trampoline()

//go:cgo_import_dynamic libc_getifaddrs getifaddrs "libsocket.so.3"
//go:cgo_import_dynamic libc_freeifaddrs freeifaddrs "libsocket.so.3"

// Getifaddrs returns the list of the system's interface addresses, which
// the caller must free with Freeifaddrs.
func Getifaddrs() (*Ifaddrs, error) {
	var ifa *Ifaddrs
	r, _, errno := syscall_syscall(abi.FuncPCABI0(libc_getifaddrs_trampoline), uintptr(unsafe.Pointer(&ifa)), 0, 0)
	if int32(r) == -1 {
		return nil, errno
	}
	return ifa, nil
}

// Freeifaddrs frees a list returned by Getifaddrs.
func Freeifaddrs(ifa *Ifaddrs) {
	syscall_syscall(abi.FuncPCABI0(libc_freeifaddrs_trampoline), uintptr(unsafe.Pointer(ifa)), 0, 0)
}
