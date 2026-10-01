// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import "internal/poll"

// Close closes the connection.
//
// On QNX 6.5, removing the name of a Unix socket that is already closed
// can hang io-pkt, the network stack, until a reboot, while removing the
// name of an open socket is safe. So on qnx, a connection that package
// net bound to a name (with ListenUnixgram, ListenPacket, or a Dial with
// a local address) removes that name before it closes, as a UnixListener
// does, and never leaves it behind. Connections returned by Accept,
// FileConn and FilePacketConn leave names alone.
func (c *UnixConn) Close() error {
	if !c.ok() {
		return c.conn.Close()
	}
	c.unlinkOnce.Do(func() {
		if c.unlink {
			poll.UnlinkSocket(c.path)
		}
	})
	return c.conn.Close()
}
