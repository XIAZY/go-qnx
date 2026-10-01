// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"runtime"
	"syscall"
	"time"
)

// QNX 6.5's io-pkt has TCP_KEEPALIVE, which takes seconds, as on
// Darwin, but no TCP_KEEPINTVL or TCP_KEEPCNT.

func setKeepAliveIdle(fd *netFD, d time.Duration) error {
	if d == 0 {
		d = defaultTCPKeepAliveIdle
	} else if d < 0 {
		return nil
	}

	// The kernel expects seconds so round to next highest second.
	secs := int(roundDurationUp(d, time.Second))
	err := fd.pfd.SetsockoptInt(syscall.IPPROTO_TCP, syscall.TCP_KEEPALIVE, secs)
	runtime.KeepAlive(fd)
	return wrapSyscallError("setsockopt", err)
}

func setKeepAliveInterval(_ *netFD, d time.Duration) error {
	if d < 0 {
		return nil
	}
	return syscall.ENOPROTOOPT
}

func setKeepAliveCount(_ *netFD, n int) error {
	if n < 0 {
		return nil
	}
	return syscall.ENOPROTOOPT
}
