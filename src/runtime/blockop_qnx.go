// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/runtime/atomic"
	_ "unsafe" // for go:linkname
)

// io-pkt, QNX 6.5's network stack, serves a few socket calls by handing
// part of the work to its main thread: binding a Unix socket to a name,
// removing a socket name, and sending descriptors over a Unix socket
// (SCM_RIGHTS). The handing thread keeps the request on its own stack
// and sleeps until the main thread answers. If the calling thread stops
// waiting first, because a signal unblocks it or because its process
// exits, io-pkt abandons the request while the main thread still holds
// it, and later calls through a dead stack frame: io-pkt crashes, and
// every socket on the machine with it, until a reboot. A C program that
// exits while one of its threads is in bind() crashes it, with no
// signals at all.
//
// Go's own signals (preemption, profiling) and its exits are the parts
// a Go program controls. qnxBlockopBegin and qnxBlockopEnd bracket the
// calls that reach this path: the calling thread blocks every
// asynchronous signal for the duration, so none can unblock it, and exit
// waits until no such call is in flight. Not covered: calls made by C
// code; a process killed from outside, not tested but presumably the
// same as an exit; and GOTRACEBACK=crash, whose abort raises SIGABRT
// instead of calling exit.

// Only the libc call itself is counted, between entersyscall and
// exitsyscall in syscall_syscall, so that it is uncounted as soon as it
// returns. Were it counted until qnxBlockopEnd, a call that had returned
// would still hold up an exit that holds the only P (GOMAXPROCS=1), as
// the caller would need that P to get from exitsyscall to
// qnxBlockopEnd.
//
// The correctness argument is the order of two pairs of operations:
// qnxBlockopCallStart increments qnxBlockops and only then reads
// qnxExiting; exit sets qnxExiting and only then reads qnxBlockops. The
// operations are sequentially consistent, so whichever of the two comes
// second sees the other's write: either exit sees the call in flight and
// waits for it, or the call sees the exit and never starts. In the other
// order both could miss each other.
var (
	// qnxBlockops counts the bracketed libc calls in flight.
	qnxBlockops atomic.Int32
	// qnxExiting is set once exit has started waiting for them.
	qnxExiting atomic.Bool
)

// qnxBlockopWait is how many times exit sleeps for 1 ms while calls are
// in flight. A 1 ms sleep lasts about 2.5 ms on QNX 6.5, whose clock
// ticks every millisecond, so this is about a second. Such a call
// normally takes far less; if io-pkt has stopped answering, exit goes
// ahead.
const qnxBlockopWait = 400

// qnxBlockopBegin is called before a bracketed call. It pins the
// goroutine to its thread and blocks every signal on the thread except
// the synchronous ones, which must stay deliverable and which a system
// call cannot raise.
//
//go:linkname syscall_qnxBlockopBegin syscall.qnxBlockopBegin
func syscall_qnxBlockopBegin() { qnxBlockopBegin() }

//go:linkname syscall_qnxBlockopEnd syscall.qnxBlockopEnd
func syscall_qnxBlockopEnd() { qnxBlockopEnd() }

//go:linkname poll_qnxBlockopBegin internal/poll.qnxBlockopBegin
func poll_qnxBlockopBegin() { qnxBlockopBegin() }

//go:linkname poll_qnxBlockopEnd internal/poll.qnxBlockopEnd
func poll_qnxBlockopEnd() { qnxBlockopEnd() }

func qnxBlockopBegin() {
	lockOSThread()
	mp := getg().m
	set := sigset_all
	sigdelset(&set, _SIGSEGV)
	sigdelset(&set, _SIGBUS)
	sigdelset(&set, _SIGFPE)
	sigdelset(&set, _SIGILL)
	sigdelset(&set, _SIGTRAP)
	sigprocmask(_SIG_BLOCK, &set, &mp.qnxBlockopMask)
	mp.qnxBlockop = true
}

// qnxBlockopEnd is called after a bracketed call. It undoes
// qnxBlockopBegin; signals that arrived meanwhile are delivered now.
func qnxBlockopEnd() {
	mp := getg().m
	mp.qnxBlockop = false
	sigprocmask(_SIG_SETMASK, &mp.qnxBlockopMask, nil)
	unlockOSThread()
}

// qnxBlockopCallStart is called by syscall_syscall, in syscall state,
// before a libc call made between qnxBlockopBegin and qnxBlockopEnd.
//
//go:nosplit
func qnxBlockopCallStart(mp *m) {
	mp.qnxInBlockop = true
	qnxBlockops.Add(1) // before reading qnxExiting: see above
	if qnxExiting.Load() {
		// The process is exiting and must not be left with this
		// call in flight. Wait for the exit to end the thread.
		qnxBlockops.Add(-1)
		mp.qnxInBlockop = false
		for {
			usleep_no_g(1e6)
		}
	}
}

// qnxBlockopCallDone is called by syscall_syscall, in syscall state,
// as soon as the libc call returns.
//
//go:nosplit
func qnxBlockopCallDone(mp *m) {
	mp.qnxInBlockop = false
	qnxBlockops.Add(-1)
}

// qnxBlockopExit is called by exit before the process ends. It must not
// block for long or take locks: it also runs on crash paths and in
// signal handlers.
//
// A thread can exit from inside a bracketed call, if the call faults
// and the fault is fatal. That call can never finish, so it is not
// waited for.
//
//go:nosplit
func qnxBlockopExit() {
	qnxExiting.Store(true) // before reading qnxBlockops: see above
	var own int32
	if gp := getg(); gp != nil && gp.m != nil && gp.m.qnxInBlockop {
		own = 1
	}
	// Count iterations rather than read a clock: this runs on crash
	// paths and in signal handlers.
	for i := 0; i < qnxBlockopWait && qnxBlockops.Load() > own; i++ {
		usleep_no_g(1000)
	}
}
