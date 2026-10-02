// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/runtime/atomic"
	"unsafe"
)

// CPU profiling on qnx. QNX 6.5 has no CPU-time timers: setitimer's
// ITIMER_PROF and ITIMER_VIRTUAL, and timer_create on a CPU-time clock,
// all fail with EINVAL. It can read a thread's CPU time, though. So, as
// on Windows, a profiling thread wakes at the profiling rate, reads the
// CPU time of each M's thread, and sends SIGPROF to the threads whose
// CPU time grew by at least one period since the last signal; the
// signal handler records the stack as usual, weighted by the number of
// periods (cpuProfWeight). Threads that did not run are not signalled.
//
// QNX 6.5 counts a thread's CPU time in clock ticks (1 ms), and the
// profiling thread wakes at most once a tick, so samples have a
// resolution of a tick, at any rate; the weighting keeps the totals
// right.
//
// The thread is started the first time profiling is turned on. While
// profiling is off it sleeps on a note and uses no CPU.
//
// The clock of an M's thread is looked up by thread id and kept with the
// M. If the thread exits and QNX gives its id to a new thread, the M
// that had it no longer runs, so at worst one live thread gets a SIGPROF
// with nothing pending, which records no sample.

var qnxProf struct {
	lock     mutex
	hz       atomic.Int32
	started  bool
	sleeping bool
	wake     note
}

func setProcessCPUProfiler(hz int32) {
	lock(&qnxProf.lock)
	qnxProf.hz.Store(hz)
	start := false
	if hz != 0 {
		if !qnxProf.started {
			qnxProf.started = true
			start = true
		} else if qnxProf.sleeping {
			qnxProf.sleeping = false
			notewakeup(&qnxProf.wake)
		}
	}
	unlock(&qnxProf.lock)
	if start {
		newm(qnxProfileLoop, nil, -1)
	}
}

func setThreadCPUProfiler(hz int32) {
	setThreadCPUProfilerHz(hz)
}

func qnxProfileLoop() {
	for {
		hz := qnxProf.hz.Load()
		if hz == 0 {
			lock(&qnxProf.lock)
			if qnxProf.hz.Load() != 0 {
				unlock(&qnxProf.lock)
				continue
			}
			noteclear(&qnxProf.wake)
			qnxProf.sleeping = true
			unlock(&qnxProf.lock)
			notesleep(&qnxProf.wake)
			continue
		}
		period := 1e9 / int64(hz)
		// Sleep at least a tick: SetCPUProfileRate accepts any rate,
		// and above 1,000,000 Hz period/1000 is 0, which would spin.
		// Rates above 1000 Hz all sample once a tick anyway.
		usleep(uint32(max(period/1000, 1000)))
		qnxProfileThreads(period)
	}
}

// qnxProfileThreads signals the Ms whose threads used at least one
// period of CPU time since they were last signalled.
func qnxProfileThreads(period int64) {
	self := getg().m
	for mp := (*m)(atomic.Loadp(unsafe.Pointer(&allm))); mp != nil; mp = mp.alllink {
		tid := mp.procid
		if mp == self || tid == 0 {
			continue
		}
		if mp.profilehz == 0 {
			// Not profiling (yet): start counting afresh when it is.
			mp.profLast = 0
			continue
		}
		if gp := mp.curg; gp != nil && gp.stackguard0 == stackFork {
			// Between BeforeFork and AfterFork, around a vfork: leave
			// the thread alone until it is back. Both are single-word
			// loads of another M's state, and a stale one costs at most
			// one skipped or one deferred signal: BeforeFork blocks every
			// signal on the thread, and a SIGPROF to the parent's thread
			// never reaches the vfork child, another process.
			continue
		}
		if mp.profTid != tid {
			// A new thread for this M (an extra M used by cgo
			// changes threads), or none known yet.
			id := clockId(0, int32(tid))
			if id < 0 {
				continue
			}
			mp.profTid, mp.profClock, mp.profLast = tid, id, 0
		}
		var t int64
		if clockTime(mp.profClock, &t) != 0 {
			mp.profTid = 0 // the thread is gone; look again next time
			continue
		}
		if mp.profLast == 0 {
			mp.profLast = t
			continue
		}
		n := (t - mp.profLast) / period
		if n <= 0 {
			continue
		}
		mp.profLast += n * period
		mp.profPending.Add(int32(n))
		signalM(mp, _SIGPROF)
	}
}

// cpuProfWeight returns the number of periods the current SIGPROF stands
// for (see the comment at the top of this file). A SIGPROF that finds
// none pending, sent with kill or arriving after its count was taken,
// stands for none and adds no sample.
//
//go:nosplit
func cpuProfWeight() uint64 {
	return uint64(getg().m.profPending.Swap(0))
}
