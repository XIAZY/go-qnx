// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/runtime/atomic"
	"unsafe"
)

// A poll(2) based network poller, derived from netpoll_aix.go. QNX 6.5
// has no kqueue or epoll equivalent for arbitrary descriptors. A pipe
// wakes the poller when the descriptor set changes.

// pollErr are the revents bits that report a descriptor in error. A
// closed descriptor reports POLLNVAL, and counts too: otherwise it
// would make every later poll return at once.
const pollErr = _POLLHUP | _POLLERR | _POLLNVAL

// pollServer identifies the server (resource manager) of a descriptor.
type pollServer struct {
	nd        uint32
	pid, chid int32
}

// serverOf returns the server of descriptor fd. If that is not known,
// it returns a value that matches no other descriptor's.
//
// ConnectServerInfo does not fail for a coid that is not a connection:
// it describes the next one above it and returns that one's coid (for
// closed fds 5 and 6 below an open socket 7 it returned 7, and above
// the last fd a side channel, 0x40000000). So anything but fd itself
// means unknown.
func serverOf(fd int32) pollServer {
	var si serverInfo
	if connectServerInfo(fd, &si) != fd {
		return pollServer{pid: -1, chid: fd}
	}
	return pollServer{si.nd, si.pid, si.chid}
}

var (
	// pfds, pds and servers are parallel. pfds keeps the descriptors
	// of each server together, because QNX 6.5's poll gets it wrong
	// when a server's descriptors are separated by another server's:
	// with a pipe, /dev/urandom, another pipe and a socket, in that
	// order, the socket came back POLLNVAL although it was open, was
	// not counted in poll's result, and its readiness was lost. With
	// each server's descriptors next to each other it never did.
	pfds           []pollfd
	pds            []*pollDesc
	servers        []pollServer
	mtxpoll        mutex
	mtxset         mutex
	rdwake         int32
	wrwake         int32
	pendingUpdates int32

	netpollWakeSig atomic.Uint32 // used to avoid duplicate calls of netpollBreak

	// polling is 1 while an M is inside netpoll's poll(2). A blocking
	// poll holds mtxset for as long as it sleeps, possibly until the
	// next timer, so a non-blocking netpoll that finds polling set
	// returns at once instead of waiting for mtxset: the blocking
	// poller will report ready descriptors. If no M is polling, the
	// non-blocking netpoll polls itself. It must: when every P is busy,
	// no M blocks in netpoll, and sysmon's and the scheduler's
	// non-blocking polls are all that deliver I/O readiness.
	polling atomic.Uint32
)

func netpollinit() {
	// Create the pipe we use to wakeup poll.
	r, w, errno := nonblockingPipe()
	if errno != 0 {
		throw("netpollinit: failed to create pipe")
	}
	rdwake = r
	wrwake = w

	// Pre-allocate array of pollfd structures for poll.
	pfds = make([]pollfd, 1, 128)

	// Poll the read side of the pipe.
	pfds[0].fd = rdwake
	pfds[0].events = _POLLIN

	pds = make([]*pollDesc, 1, 128)
	pds[0] = nil

	servers = make([]pollServer, 1, 128)
	servers[0] = serverOf(rdwake)
}

func netpollIsPollDescriptor(fd uintptr) bool {
	return fd == uintptr(rdwake) || fd == uintptr(wrwake)
}

// netpollwakeup writes on wrwake to wakeup poll before any changes.
func netpollwakeup() {
	if pendingUpdates == 0 {
		pendingUpdates = 1
		b := [1]byte{0}
		write(uintptr(wrwake), unsafe.Pointer(&b[0]), 1)
	}
}

func netpollopen(fd uintptr, pd *pollDesc) int32 {
	srv := serverOf(int32(fd))

	lock(&mtxpoll)
	netpollwakeup()

	lock(&mtxset)
	unlock(&mtxpoll)

	// We don't worry about pd.fdseq here,
	// as mtxset protects us from stale pollDescs.

	// Insert fd after the last descriptor of the same server, if any.
	// The wakeup pipe stays at index 0.
	i := len(pfds)
	for j := len(servers) - 1; j >= 0; j-- {
		if servers[j] == srv {
			i = j + 1
			break
		}
	}
	pfds = append(pfds, pollfd{})
	pds = append(pds, nil)
	servers = append(servers, pollServer{})
	copy(pfds[i+1:], pfds[i:])
	copy(pds[i+1:], pds[i:])
	copy(servers[i+1:], servers[i:])
	pfds[i] = pollfd{fd: int32(fd)}
	pds[i] = pd
	servers[i] = srv
	for j := i; j < len(pds); j++ {
		pds[j].user = uint32(j)
	}
	unlock(&mtxset)
	return 0
}

func netpollclose(fd uintptr) int32 {
	lock(&mtxpoll)
	netpollwakeup()

	lock(&mtxset)
	unlock(&mtxpoll)

	// Remove fd keeping the order, and so each server's descriptors
	// together.
	for i := 1; i < len(pfds); i++ {
		if pfds[i].fd == int32(fd) {
			copy(pfds[i:], pfds[i+1:])
			pfds = pfds[:len(pfds)-1]
			copy(pds[i:], pds[i+1:])
			pds[len(pds)-1] = nil
			pds = pds[:len(pds)-1]
			copy(servers[i:], servers[i+1:])
			servers = servers[:len(servers)-1]
			for j := i; j < len(pds); j++ {
				pds[j].user = uint32(j)
			}
			break
		}
	}
	unlock(&mtxset)
	return 0
}

func netpollarm(pd *pollDesc, mode int) {
	lock(&mtxpoll)
	netpollwakeup()

	lock(&mtxset)
	unlock(&mtxpoll)

	switch mode {
	case 'r':
		pfds[pd.user].events |= _POLLIN
	case 'w':
		pfds[pd.user].events |= _POLLOUT
	}
	unlock(&mtxset)
}

// netpollBreak interrupts a poll.
func netpollBreak() {
	// Failing to cas indicates there is an in-flight wakeup, so we're done here.
	if !netpollWakeSig.CompareAndSwap(0, 1) {
		return
	}

	b := [1]byte{0}
	write(uintptr(wrwake), unsafe.Pointer(&b[0]), 1)
}

// netpoll checks for ready network connections.
// Returns a list of goroutines that become runnable,
// and a delta to add to netpollWaiters.
// This must never return an empty list with a non-zero delta.
//
// delay < 0: blocks indefinitely
// delay == 0: does not block, just polls
// delay > 0: block for up to that many nanoseconds
//
//go:nowritebarrierrec
func netpoll(delay int64) (gList, int32) {
	var timeout int32
	if delay < 0 {
		timeout = -1
	} else if delay == 0 {
		// Never wait for a blocking poller (see polling): the
		// scheduler calls this from startTheWorld and findRunnable.
		if !polling.CompareAndSwap(0, 1) {
			return gList{}, 0
		}
		timeout = 0
	} else if delay < 1e6 {
		timeout = 1
	} else if delay < 1e15 {
		timeout = int32(delay / 1e6)
	} else {
		// An arbitrary cap on how long to wait for a timer.
		// 1e9 ms == ~11.5 days.
		timeout = 1e9
	}
	if delay != 0 {
		// A non-blocking poll holds polling only briefly.
		for !polling.CompareAndSwap(0, 1) {
			osyield()
		}
	}
retry:
	lock(&mtxpoll)
	lock(&mtxset)
	pendingUpdates = 0
	unlock(&mtxpoll)

	// QNX 6.5's poll does not always count every descriptor it reports:
	// in one net/http/httptest run it returned 2 with three descriptors
	// reporting (the wakeup pipe POLLIN, a descriptor POLLNVAL and a
	// connecting socket POLLOUT), and the socket's connect never
	// completed. A poll that timed out, returning 0, still had a
	// POLLNVAL in revents. So clear revents here and look at every
	// descriptor below, whatever n says.
	for i := range pfds {
		pfds[i].revents = 0
	}
	n, e := poll(&pfds[0], uint32(len(pfds)), timeout)
	if n < 0 {
		if e != _EINTR {
			println("errno=", e, " len(pfds)=", len(pfds))
			throw("poll failed")
		}
		unlock(&mtxset)
		// If a timed sleep was interrupted, just return to
		// recalculate how long we should sleep now.
		if timeout >= 0 {
			polling.Store(0)
			return gList{}, 0
		}
		goto retry
	}
	// Check if some descriptors need to be changed
	if pfds[0].revents&(_POLLIN|pollErr) != 0 {
		if delay != 0 {
			// A netpollwakeup could be picked up by a
			// non-blocking poll. Only clear the wakeup
			// if blocking.
			var b [1]byte
			for read(rdwake, unsafe.Pointer(&b[0]), 1) == 1 {
			}
			netpollWakeSig.Store(0)
		}
		// Still look at the other fds even if the mode may have
		// changed, as netpollBreak might have been called.
	}
	var toRun gList
	delta := int32(0)
	for i := 1; i < len(pfds); i++ {
		pfd := &pfds[i]

		var mode int32
		if pfd.revents&(_POLLIN|pollErr) != 0 {
			mode += 'r'
			pfd.events &= ^_POLLIN
		}
		if pfd.revents&(_POLLOUT|pollErr) != 0 {
			mode += 'w'
			pfd.events &= ^_POLLOUT
		}
		if mode != 0 {
			pds[i].setEventErr(pfd.revents&(_POLLERR|_POLLNVAL) != 0 && pfd.revents&(_POLLIN|_POLLOUT) == 0, 0)
			delta += netpollready(&toRun, pds[i], mode)
		}
	}
	unlock(&mtxset)
	polling.Store(0)
	return toRun, delta
}
