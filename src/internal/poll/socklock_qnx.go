// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package poll

import (
	"sync"
	"syscall"
)

// qnxSockLock works around a bug in io-pkt, QNX 6.5's network stack: if the
// name of a Unix socket is unlinked while io-pkt serves another socket
// request, of any protocol, io-pkt stops answering for good, and every
// process that sends it a request blocks. UnlinkSocket holds qnxSockLock
// exclusively, and the calls that make, configure, connect, accept, shut
// down and close sockets hold it shared, so that within this process no
// unlink of a socket name overlaps one of them.
//
// Reads and writes on sockets are left out, because they do not trigger the
// bug: in a C reproducer, 444,857 unlocked reads and writes overlapping
// 20,000 locked unlinks, and 700,440 with a poller in poll(2) and unlocked
// reads as well, never wedged io-pkt. The poller's poll(2) is left out for
// the same reason.
//
// os.Remove (and so os.RemoveAll, and os.Root's Remove and RemoveAll) of
// a socket name goes through UnlinkSocket too. Not covered: other
// processes; sockets used through os.File, whose FDs are files to this
// package; calls made by C code, including the cgo resolver; and a
// RawConn's Control, Read and Write functions.
//
// A second hazard is beyond any lock: removing the name of a Unix socket
// at all, whether the socket is open or closed and whichever program
// removes it (rm included), can hang io-pkt the same way, until a
// reboot, with a small chance each time. Removing the name after the
// socket is closed did so more often. Sockets without names (socketpair,
// unbound sockets) did not trigger it. So programs that create and remove
// many named Unix sockets should prefer socketpair or loopback TCP, and
// where they must remove a name, remove it while the socket is open.
//
// io-pkt keeps every socket name until it is removed, closed sockets'
// included, about a thousand at most; after that, bind fails with
// EMFILE. It also lets a second socket bind a name that is in use.
//
// Only the system call itself is covered, never a wait for readiness in
// the poller: a non-blocking socket call returns at once, so an unlink
// waits for at most one system call on each other socket. A socket made
// blocking holds the lock for as long as its call blocks, and an unlink
// waits for it.
//
// Lock order: qnxSockLock (shared), then syscall.ForkLock (shared, in
// net's sysSocket) or syscall's close lock (shared, inside
// syscall.Close). Whoever holds either of those exclusively must make no
// socket call. No shared hold of qnxSockLock may nest inside another: an
// UnlinkSocket waiting for the lock blocks new shared holders, so the
// inner one would wait forever.
var qnxSockLock sync.RWMutex

// sockRLock takes qnxSockLock shared if fd is a socket.
func (fd *FD) sockRLock() {
	if !fd.isFile {
		qnxSockLock.RLock()
	}
}

// sockRUnlock releases what sockRLock took.
func (fd *FD) sockRUnlock() {
	if !fd.isFile {
		qnxSockLock.RUnlock()
	}
}

// SockRLock takes the socket lock shared, for socket calls that package
// net makes outside an FD.
func SockRLock() { qnxSockLock.RLock() }

// SockRUnlock releases what SockRLock took.
func SockRUnlock() { qnxSockLock.RUnlock() }

// UnlinkSocket removes the name of a Unix socket, with no other socket
// call of this process in progress.
//
// The unlink is also bracketed against signals and exit, as io-pkt can
// crash if the caller stops waiting for it; see runtime/blockop_qnx.go.
func UnlinkSocket(path string) error {
	qnxSockLock.Lock()
	defer qnxSockLock.Unlock()
	qnxBlockopBegin()
	defer qnxBlockopEnd()
	return syscall.Unlink(path)
}

// Implemented in the runtime package (runtime/blockop_qnx.go).
func qnxBlockopBegin()
func qnxBlockopEnd()
