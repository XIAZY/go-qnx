// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build qnx

package runtime

import "internal/runtime/atomic"

// The network poller. Each descriptor a goroutine waits on is armed with
// ionotify to send a pulse to a channel of ours when it is ready, and
// netpoll receives the pulses. This avoids libc's poll, which on QNX 6.5
// can lose a socket's readiness when the descriptors of one resource
// manager are separated in the poll set by another's; arming each
// descriptor separately with ionotify does not lose it.
//
// What QNX 6.5's ionotify does:
//   - A descriptor has one armed event: the last arm wins. So the union
//     of the wanted conditions is armed with one event.
//   - A delivered pulse carries no conditions, so it means "one of the
//     armed conditions": both waiters are woken, and each that has
//     nothing to do sees EAGAIN and waits again.
//   - Arming is all or nothing: if a condition is already met, nothing
//     is armed and the met conditions are returned. Then we send the
//     pulse ourselves.
//   - Closing a descriptor does not disarm it: the server keeps the arm
//     with the old object, and a pulse sent before the close may still
//     arrive after it. Every pulse is checked against the descriptor's
//     current pollDesc and its fdseq.
//   - A signal interrupts MsgReceivePulse (EINTR) and consumes its
//     TimerTimeout.
//
// pd.user holds a bit for a waiting reader and one for a waiting
// writer, changed only by the waiters, under pd.lock: set when they start
// waiting (netpollarm, which arms the union) and cleared when they stop
// (netpollWaitDone). netpoll never touches it, so an arm made for one
// waiter is never undone on behalf of another.
//
// QNX 6.5 has no private channels: any process can attach to ours and
// send pulses. A pulse is only ever used as a hint to look at a
// descriptor whose pollDesc and fdseq match it, never as a pointer.

const (
	pulseCodeReady = 1 // a descriptor's armed condition is met
	pulseCodeBreak = 2 // netpollBreak

	pulseFDBits  = 16
	pulseSeqMask = 0xfff // 12 bits: _NOTIFY_COND_MASK takes the top 4

	pulseWantRead  = 1
	pulseWantWrite = 2
	// pulseLastMet records that the last arm found its conditions
	// already met, so we sent the pulse ourselves (see pulseArm).
	pulseLastMet = 4

	_NOTIFY_ACTION_TRANARM = 0x0

	pulseRecheckNS = 1e6 // see pulseArm

	pulseBatch = 128 // pulses taken per netpoll call
)

var (
	pulseChid int32
	pulseCoid int32

	netpollWakeSig atomic.Uint32 // used to avoid duplicate calls of netpollBreak

	// pulsePDs maps descriptors to their pollDesc.
	pulseLock mutex
	pulsePDs  []*pollDesc

	// pulseRechecks holds the descriptors armed with TRANARM (see
	// pulseArm), each woken once at its due time from netpoll. It holds
	// no pointers, so that netpoll can change it without write barriers.
	// Guarded by pulseLock.
	pulseRechecks []pulseRecheck

	// Counts of the slow paths in pulseArm and pulseRunRechecks, for
	// tests.
	pulseNSelf, pulseNTran, pulseNRecheck atomic.Uint64
)

type pulseRecheck struct {
	fd  uintptr
	seq uintptr
	due int64
}

func netpollinit() {
	// Receiving a pulse gives the receiving thread the pulse's priority
	// unless the channel has fixed priority: a pulse from io-pkt (21)
	// would leave the M that took it running goroutines at 21, starving
	// the others at 10.
	chid := channelCreate(_NTO_CHF_FIXED_PRIORITY)
	if chid < 0 {
		println("runtime: ChannelCreate failed with", -chid)
		throw("runtime: netpollinit failed")
	}
	coid := connectAttach(0, 0, chid, _NTO_SIDE_CHANNEL, 0)
	if coid < 0 {
		println("runtime: ConnectAttach failed with", -coid)
		throw("runtime: netpollinit failed")
	}
	pulseChid, pulseCoid = chid, coid
}

func netpollIsPollDescriptor(fd uintptr) bool {
	return false
}

func netpollopen(fd uintptr, pd *pollDesc) int32 {
	if fd >= 1<<pulseFDBits {
		// Does not fit in a pulse value (QNX's default limit is
		// 1000 descriptors): treat it as not pollable.
		return _ENOSYS
	}
	// Servers that do not support ionotify (/dev/null, /dev/urandom)
	// fail with ENOSYS: the descriptor is not pollable.
	if _, e := ionotify(int32(fd), _NOTIFY_ACTION_POLL, _NOTIFY_COND_INPUT|_NOTIFY_COND_OUTPUT, nil); e != 0 {
		return e
	}
	lock(&pd.lock)
	pd.user = 0
	unlock(&pd.lock)
	lock(&pulseLock)
	for uintptr(len(pulsePDs)) <= fd {
		pulsePDs = append(pulsePDs, nil)
	}
	pulsePDs[fd] = pd
	unlock(&pulseLock)
	return 0
}

// netpollclose runs before the descriptor is closed (internal/poll's
// FD.destroy), but it does not disarm it: a pulse already sent would
// still arrive, and the fdseq check makes it harmless, while disarming
// would add an io-pkt request to every close. Forgetting the pollDesc is
// enough.
func netpollclose(fd uintptr) int32 {
	lock(&pulseLock)
	if fd < uintptr(len(pulsePDs)) {
		pulsePDs[fd] = nil
	}
	unlock(&pulseLock)
	return 0
}

func pulseModeBit(mode int) uint32 {
	switch mode {
	case 'r':
		return pulseWantRead
	case 'w':
		return pulseWantWrite
	}
	throw("runtime: bad mode")
	return 0
}

func netpollarm(pd *pollDesc, mode int) {
	lock(&pd.lock)
	pd.user |= pulseModeBit(mode)
	pulseArm(pd)
	unlock(&pd.lock)
}

// netpollWaitDone clears the waiter's bit. It does not re-arm for
// the other waiter, if any: the pulse that ended this wait woke that one
// too (netpoll wakes both modes), and it arms again itself after its
// EAGAIN. A waiter that stops without a pulse (a timeout, a close) leaves
// at worst a stale arm, which can only cause a spurious wake. Re-arming
// here was measured to cost one more arm per round and to save no waits.
func netpollWaitDone(pd *pollDesc, mode int) {
	lock(&pd.lock)
	pd.user &^= pulseModeBit(mode)
	unlock(&pd.lock)
}

// pulseArm arms pd's descriptor for the conditions in pd.user. pd.lock
// must be held. It is called by a waiter that has just seen EAGAIN.
//
// If a condition is already met, nothing is armed and we send the pulse
// ourselves. That is not always true readiness: a pipe reports output
// "met" when one byte is free, while a write of up to PIPE_BUF (5120)
// bytes is atomic and gets EAGAIN until there is room for all of it. A
// writer would then loop arm, "met", pulse, EAGAIN: measured at 90,000
// times a second, with netpoll never blocking and the process's timers
// unserved for seconds. So when the previous arm was answered "met" and
// the waiter is back with EAGAIN, the next arm is TRANARM, which fires
// on the next change (a read from the pipe), and the descriptor is also
// put on pulseRechecks, to be woken by netpoll after pulseRecheckNS.
// TRANARM alone could wait for good: if the reader drained the pipe
// between the writer's EAGAIN and its arm, and then waits for the
// writer, no further read comes. The recheck covers that, at the cost of
// at most one wake a millisecond while a descriptor stays in this state.
//
// While a pipe stays full the arms alternate: POLLARM answered "met" (a
// self-pulse), then TRANARM and a recheck, about 3 ms a cycle once the
// recheck's 1 ms is rounded to the clock tick. Staying on TRANARM until
// the waiter makes progress would halve the wakes, but the runtime does
// not see progress: the write is retried in internal/poll, and
// netpollWaitDone runs on every wake, progress or not. Left as a
// follow-up; it needs internal/poll to say when a write went through.
func pulseArm(pd *pollDesc) {
	if pd.closing {
		return
	}
	var cond int32
	if pd.user&pulseWantRead != 0 {
		cond |= _NOTIFY_COND_INPUT
	}
	if pd.user&pulseWantWrite != 0 {
		cond |= _NOTIFY_COND_OUTPUT
	}
	value := int32(pd.fd) | int32(pd.fdseq.Load()&pulseSeqMask)<<pulseFDBits
	ev := sigeventPulse{
		notify:   _SIGEV_PULSE,
		coid:     pulseCoid,
		value:    value,
		code:     pulseCodeReady,
		priority: _SIGEV_PULSE_PRIO_INHERIT,
	}
	if pd.user&pulseLastMet != 0 {
		pd.user &^= pulseLastMet
		pulseNTran.Add(1)
		ionotify(int32(pd.fd), _NOTIFY_ACTION_TRANARM, cond, &ev)
		lock(&pulseLock)
		pulseRechecks = append(pulseRechecks, pulseRecheck{pd.fd, pd.fdseq.Load(), nanotime() + pulseRecheckNS})
		unlock(&pulseLock)
		netpollBreak() // so that a blocked netpoll takes the recheck into its delay
		return
	}
	r, e := ionotify(int32(pd.fd), _NOTIFY_ACTION_POLLARM, cond, &ev)
	// If a condition is already met, nothing was armed. If arming
	// failed, the waiter must still wake to find out why. Either way,
	// send the pulse the descriptor would have sent.
	if e != 0 || uint32(r)&_NOTIFY_COND_MASK&uint32(cond) != 0 {
		if e == 0 {
			pd.user |= pulseLastMet
		}
		pulseNSelf.Add(1)
		msgSendPulse(pulseCoid, -1, pulseCodeReady, value)
	}
}

func netpollBreak() {
	// Failing to cas indicates there is an in-flight wakeup, so we're done here.
	if !netpollWakeSig.CompareAndSwap(0, 1) {
		return
	}
	msgSendPulse(pulseCoid, -1, pulseCodeBreak, 0)
}

// netpoll receives ready pulses. A pulse is processed by whichever
// netpoll call receives it, blocking or not; only the break pulse, if a
// non-blocking call takes it, is sent again for the blocking one.
//
//go:nowritebarrierrec
func netpoll(delay int64) (gList, int32) {
	var toRun gList
	delta := int32(0)
	// Wake the rechecks that are due, and wait no longer than the next.
	if next := pulseRunRechecks(&toRun, &delta); next > 0 && (delay < 0 || next < delay) {
		delay = next
	}
	var p qnxPulse
	for i := 0; i < pulseBatch; i++ {
		// The first receive waits as long as delay says; the others
		// only take what is already queued. A timeout must be set
		// before every receive: the kernel call consumes it.
		var r int32
		if i > 0 || delay == 0 {
			r = pulseReceiveTimeout(&p, 0)
		} else if delay > 0 {
			r = pulseReceiveTimeout(&p, uint64(delay))
		} else {
			r = msgReceivePulse(pulseChid, &p)
		}
		if r == -_ETIMEDOUT {
			break
		}
		if r == -_EINTR {
			// A signal: with delay > 0 the caller recomputes the
			// time left, as with any early return.
			if i > 0 || delay >= 0 {
				break
			}
			i-- // blocking indefinitely: wait again
			continue
		}
		if r < 0 {
			println("runtime: MsgReceivePulse on channel", pulseChid, "failed with", -r)
			throw("runtime: netpoll failed")
		}
		switch p.code {
		case pulseCodeBreak:
			if delay != 0 {
				netpollWakeSig.Store(0)
			} else {
				msgSendPulse(pulseCoid, -1, pulseCodeBreak, 0)
			}
		case pulseCodeReady:
			if pd := pulseLookup(p.value); pd != nil {
				delta += netpollready(&toRun, pd, 'r'+'w')
			}
		}
	}
	return toRun, delta
}

// pulseRunRechecks wakes the waiters of the descriptors in
// pulseRechecks whose time has come, and returns the time until the
// next one is due, or 0 if none is left.
//
//go:nowritebarrierrec
func pulseRunRechecks(toRun *gList, delta *int32) int64 {
	lock(&pulseLock)
	if len(pulseRechecks) == 0 {
		unlock(&pulseLock)
		return 0
	}
	now := nanotime()
	next := int64(0)
	j := 0
	for _, rc := range pulseRechecks {
		if rc.due > now {
			if next == 0 || rc.due-now < next {
				next = rc.due - now
			}
			pulseRechecks[j] = rc
			j++
			continue
		}
		pulseNRecheck.Add(1)
		if rc.fd < uintptr(len(pulsePDs)) {
			if pd := pulsePDs[rc.fd]; pd != nil && pd.fdseq.Load() == rc.seq {
				*delta += netpollready(toRun, pd, 'r'+'w')
			}
		}
	}
	pulseRechecks = pulseRechecks[:j]
	unlock(&pulseLock)
	return next
}

// pulseReceiveTimeout receives a pulse, waiting at most ns nanoseconds.
// TimerTimeout bounds the thread's next kernel call, whichever it is: if
// a signal arrived between it and MsgReceivePulse, the handler's own
// kernel calls (SignalReturn at least) would take the timeout, and the
// receive would then wait with none, for good. So signals are blocked
// around the pair. On QNX 6.5 a signal taken during the receive does
// consume the timeout (EINTR); a signal between the two calls has not
// been seen to cause a failure, but nothing else closes that window.
//
//go:nosplit
func pulseReceiveTimeout(p *qnxPulse, ns uint64) int32 {
	var old sigset
	sigprocmask(_SIG_SETMASK, &sigset_all, &old)
	timerTimeoutReceive(ns)
	r := msgReceivePulse(pulseChid, p)
	sigprocmask(_SIG_SETMASK, &old, nil)
	return r
}

// pulseLookup returns the pollDesc a ready pulse's value names, or nil
// if the value is stale (the descriptor was closed) or not ours.
//
//go:nowritebarrierrec
func pulseLookup(value int32) *pollDesc {
	fd := uintptr(value) & (1<<pulseFDBits - 1)
	seq := uintptr(value) >> pulseFDBits & pulseSeqMask
	lock(&pulseLock)
	var pd *pollDesc
	if fd < uintptr(len(pulsePDs)) {
		pd = pulsePDs[fd]
	}
	unlock(&pulseLock)
	if pd == nil || pd.fdseq.Load()&pulseSeqMask != seq {
		return nil
	}
	return pd
}
