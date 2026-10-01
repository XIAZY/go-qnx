// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"internal/poll"
	"syscall"
)

// unlink removes the name. The name of a Unix socket is removed with no
// other socket call of this process in progress: io-pkt stops answering
// for good if one is unlinked while it serves another socket request
// (see internal/poll/socklock_qnx.go). Go never removes the name of a
// datagram socket itself, so programs using one call Remove on it.
func unlink(name string) error {
	// The lstat is itself a request to io-pkt if name is a socket.
	var st syscall.Stat_t
	poll.SockRLock()
	err := ignoringEINTR(func() error {
		return syscall.Lstat(name, &st)
	})
	poll.SockRUnlock()
	if err == nil && st.Mode&syscall.S_IFMT == syscall.S_IFSOCK {
		return poll.UnlinkSocket(name)
	}
	return syscall.Unlink(name)
}
