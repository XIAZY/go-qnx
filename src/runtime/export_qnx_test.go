// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

func TSCToNs(d uint64, mult, shift uint32) uint64 { return tscToNs(d, mult, shift) }

func TSCParams(freq uint64) (mult, shift uint32, ok bool) { return tscParams(freq) }

func TSCRateOff(dn, dm int64) bool { return tscRateOff(dn, dm) }

// QNXTSCOn reports whether nanotime reads the TSC.
func QNXTSCOn() bool { return qnxTSC.state.Load() == tscOn }

// QNXTSCInjectRate multiplies the TSC path's rate by num/den, continuing
// from the current nanotime so that the change itself goes nowhere
// backward, and makes sysmon's check use windows of window ns from now.
func QNXTSCInjectRate(num, den uint32, window int64) {
	qnxTSC.state.Store(tscSwitching)
	snap := qnxTSC.baseNs + int64(tscToNs(uint64(cputicks())-qnxTSC.base, qnxTSC.mult, qnxTSC.shift))
	qnxTSC.base = uint64(cputicks())
	qnxTSC.baseNs = snap
	qnxTSC.mult = uint32(uint64(qnxTSC.mult) * uint64(num) / uint64(den))
	qnxTSC.minWindow = window
	qnxTSC.checkNs, qnxTSC.checkMono = snap, clockMonotonic()
	qnxTSC.strikes = 0
	qnxTSC.state.Store(tscOn)
}

// QNXCheckTSC runs sysmon's check of the TSC rate.
func QNXCheckTSC() { qnxCheckTSC() }

// NetpollPulseCounts returns how many times the network poller has sent a
// descriptor's pulse itself, armed a descriptor with TRANARM, and woken a
// descriptor's waiters for a recheck.
func NetpollPulseCounts() (self, tran, recheck uint64) {
	return pulseNSelf.Load(), pulseNTran.Load(), pulseNRecheck.Load()
}

// QNXBlockopBegin and QNXBlockopEnd bracket a call as io-pkt's blockop
// callers are bracketed (see blockop_qnx.go).
func QNXBlockopBegin() { qnxBlockopBegin() }
func QNXBlockopEnd()   { qnxBlockopEnd() }

// QNXBlockopCallSleep stands for a bracketed libc call that takes usec
// microseconds, as syscall_syscall makes it. It must be called between
// QNXBlockopBegin and QNXBlockopEnd.
//
//go:nosplit
func QNXBlockopCallSleep(usec uint32) {
	entersyscall()
	mp := getg().m
	qnxBlockopCallStart(mp)
	usleep_no_g(usec)
	qnxBlockopCallDone(mp)
	exitsyscall()
}

// QNXBlockopCallHang stands for a bracketed libc call that never returns.
//
//go:nosplit
func QNXBlockopCallHang() {
	entersyscall()
	qnxBlockopCallStart(getg().m)
	for {
		usleep_no_g(1e6)
	}
}

// QNXBlockops returns how many bracketed calls are in flight.
func QNXBlockops() int32 { return qnxBlockops.Load() }

// QNXThreadSigmask returns the calling thread's signal mask.
func QNXThreadSigmask() [2]uint32 {
	var old sigset
	sigprocmask(_SIG_BLOCK, nil, &old)
	return old.__bits
}

// QNXSigBit reports whether signal sig is in mask.
func QNXSigBit(mask [2]uint32, sig int) bool {
	return mask[(sig-1)/32]&(1<<((sig-1)%32)) != 0
}
