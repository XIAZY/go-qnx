// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
	"internal/runtime/atomic"
	"unsafe"
)

// On qnx, nanotime follows the TSC when it can be trusted, and
// clock_gettime(CLOCK_MONOTONIC) otherwise.
//
// QNX 6.5's CLOCK_MONOTONIC advances once per timer tick, 1 ms by default,
// whatever clock_getres says. The TSC gives nanotime sub-microsecond
// resolution. It is converted with the TSC rate the kernel measured at
// boot (the system page's qtime cycles_per_sec), anchored once to
// CLOCK_MONOTONIC when the process starts, and not adjusted afterwards.
// So nanotime tracks real time, even on virtual machines where QNX's tick
// clock loses time because ticks are dropped, and it can differ from
// clock_gettime(CLOCK_MONOTONIC) by what the tick clock has lost. Wall
// time stays on CLOCK_REALTIME.
//
// The TSC is used if the CPU reports an invariant TSC, or the process runs
// under KVM, which keeps guest TSCs synchronised across CPUs. It is read
// with cputicks, which serializes the read (RDTSCP, or MFENCE;LFENCE):
// a bare RDTSC can return a value below one already read on another CPU.
//
// The rate is fixed, so a migration to a host with a different TSC rate
// would skew nanotime. sysmon checks for that (qnxCheckTSC): if nanotime
// and CLOCK_MONOTONIC disagree by more than 5% in two consecutive windows
// of 10 seconds or more, nanotime switches to CLOCK_MONOTONIC for good,
// continuing from the largest value it can have returned, so that it never
// goes backward.

const (
	tscOff       = 0 // nanotime reads CLOCK_MONOTONIC
	tscOn        = 1 // nanotime reads the TSC
	tscSwitching = 2 // sysmon is switching from tscOn to tscOff
)

var qnxTSC struct {
	state atomic.Uint32

	// TSC path, set before state becomes tscOn and then fixed:
	// nanotime = baseNs + tscToNs(cputicks() - base).
	base   uint64
	baseNs int64
	mult   uint32
	shift  uint32

	// CLOCK_MONOTONIC path: nanotime = max(last, CLOCK_MONOTONIC + offset).
	offset int64
	last   atomic.Int64

	// sysmon's check: the clocks at the start of the current window, and
	// the shortest window (a variable for tests).
	checkNs, checkMono int64
	minWindow          int64
	strikes            int32 // consecutive windows off by more than 5%
}

//go:nosplit
func nanotime1() int64 {
	for {
		switch qnxTSC.state.Load() {
		case tscOn:
			v := qnxTSC.baseNs + int64(tscToNs(uint64(cputicks())-qnxTSC.base, qnxTSC.mult, qnxTSC.shift))
			if qnxTSC.state.Load() == tscOn {
				return v
			}
			// sysmon began switching after our first look; the value
			// may be above what the CLOCK_MONOTONIC path continues from.
		case tscSwitching:
			procyield(10)
		default:
			return monoNanotime()
		}
	}
}

// monoNanotime is nanotime on the CLOCK_MONOTONIC path. The atomic
// maximum keeps it from going backward across the switch from the TSC.
//
//go:nosplit
func monoNanotime() int64 {
	v := clockMonotonic() + qnxTSC.offset
	for {
		old := qnxTSC.last.Load()
		if v <= old {
			return old
		}
		if qnxTSC.last.CompareAndSwap(old, v) {
			return v
		}
	}
}

// tscToNs converts d TSC cycles to nanoseconds: d*mult >> shift, with
// shift <= 32, exact for any d. The 96-bit product is taken in two
// 32x32-bit halves: the high half's low shift bits are zero after
// shifting it left by 32, so the two halves can be shifted separately.
//
//go:nosplit
func tscToNs(d uint64, mult, shift uint32) uint64 {
	lo := uint64(uint32(d)) * uint64(mult)
	hi := (d >> 32) * uint64(mult)
	return hi<<(32-shift) + lo>>shift
}

// tscParams returns mult and shift for tscToNs at freq cycles per second:
// the largest shift <= 32 for which mult = 1e9<<shift / freq fits in 32
// bits. ok is false if freq is too low for that.
func tscParams(freq uint64) (mult, shift uint32, ok bool) {
	if freq < 1e6 {
		return 0, 0, false
	}
	for shift = 32; ; shift-- {
		m := uint64(1e9) << shift / freq
		if m < 1<<32 {
			return uint32(m), shift, m > 0
		}
		if shift == 0 {
			return 0, 0, false
		}
	}
}

// qnxInitTSC decides, at osinit, whether nanotime uses the TSC.
func qnxInitTSC() {
	qnxTSC.minWindow = 10e9
	if qnxGO386Softfloat() {
		return // cputicks cannot serialize the read without SSE2
	}
	_, _, ecx, edx := qnxCpuid(1, 0)
	if edx&(1<<4) == 0 || edx&(1<<26) == 0 { // TSC, SSE2
		return
	}
	trusted := false
	if maxExt, _, _, _ := qnxCpuid(0x80000000, 0); maxExt >= 0x80000007 {
		_, _, _, d := qnxCpuid(0x80000007, 0)
		trusted = d&(1<<8) != 0 // invariant TSC
	}
	if !trusted && ecx&(1<<31) != 0 { // running under a hypervisor
		_, b, c, d := qnxCpuid(0x40000000, 0)
		trusted = b == 0x4b4d564b && c == 0x564b4d56 && d == 0x0000004d // "KVMKVMKVM\0\0\0"
	}
	if !trusted {
		return
	}
	mult, shift, ok := tscParams(qnxCyclesPerSec())
	if !ok {
		return
	}
	qnxTSC.mult, qnxTSC.shift = mult, shift
	qnxTSC.base = uint64(cputicks())
	qnxTSC.baseNs = max(clockMonotonic(), qnxTSC.last.Load())
	qnxTSC.checkNs, qnxTSC.checkMono = qnxTSC.baseNs, clockMonotonic()
	qnxTSC.state.Store(tscOn)
}

// syspagePtrName is static data: qnxCyclesPerSec runs from osinit,
// before the heap exists.
var syspagePtrName = []byte("_syspage_ptr\x00")

// qnxCyclesPerSec returns the system page's qtime cycles_per_sec, or 0.
// The offsets were checked against <sys/syspage.h> of QNX 6.5.0: in
// struct syspage_entry, the syspage_entry_info qtime is at 32 (after the
// four _Uint16t size, total_size, type, num_cpu and six 4-byte
// syspage_entry_info), and its entry_off is its first _Uint16t; in struct
// qtime_entry, the _Uint64t cycles_per_sec is at 0. SYSPAGE_ENTRY(qtime)
// is the system page plus entry_off.
func qnxCyclesPerSec() uint64 {
	p := dlsym(^uintptr(1), &syspagePtrName[0]) // RTLD_DEFAULT is (void *)-2
	if p == 0 {
		return 0
	}
	sp := *(*uintptr)(unsafe.Pointer(p))
	if sp == 0 {
		return 0
	}
	off := *(*uint16)(unsafe.Pointer(sp + 32))
	return *(*uint64)(unsafe.Pointer(sp + uintptr(off)))
}

// qnxCheckTSC is sysmon's check that the TSC rate still matches
// CLOCK_MONOTONIC to within 5%, over windows of at least minWindow,
// switching to CLOCK_MONOTONIC after two windows in a row are off.
func qnxCheckTSC() {
	if qnxTSC.state.Load() != tscOn {
		return
	}
	now, mono := nanotime1(), clockMonotonic()
	dn, dm := now-qnxTSC.checkNs, mono-qnxTSC.checkMono
	if dm < qnxTSC.minWindow {
		return
	}
	qnxTSC.checkNs, qnxTSC.checkMono = now, mono
	if !tscRateOff(dn, dm) {
		qnxTSC.strikes = 0
		return
	}
	// Ticks are lost in bursts on virtual machines; a single window
	// could be off by more than usual. Switch only after two in a row.
	if qnxTSC.strikes++; qnxTSC.strikes >= 2 {
		qnxSwitchToMonotonic()
	}
}

// tscRateOff reports whether dn and dm, the same interval measured by the
// TSC and by CLOCK_MONOTONIC, differ by more than 5%. QNX's tick clock can
// lose about 1% on a virtual machine; a changed TSC rate is far larger.
func tscRateOff(dn, dm int64) bool {
	return dn*20 > dm*21 || dn*20 < dm*19
}

// qnxSwitchToMonotonic moves nanotime from the TSC to CLOCK_MONOTONIC for
// good. A TSC reader that returns has read the TSC before state left
// tscOn, so its value is at most snap, which the other path continues
// from.
func qnxSwitchToMonotonic() {
	qnxTSC.state.Store(tscSwitching)
	snap := qnxTSC.baseNs + int64(tscToNs(uint64(cputicks())-qnxTSC.base, qnxTSC.mult, qnxTSC.shift))
	qnxTSC.offset = snap - clockMonotonic()
	if snap > qnxTSC.last.Load() {
		qnxTSC.last.Store(snap)
	}
	qnxTSC.state.Store(tscOff)
}

//go:nosplit
func dlsym(handle uintptr, name *byte) uintptr {
	args := struct {
		handle uintptr
		name   *byte
	}{handle, name}
	return uintptr(libcCall(unsafe.Pointer(abi.FuncPCABI0(dlsym_trampoline)), unsafe.Pointer(&args)))
}
func dlsym_trampoline()

//go:cgo_import_dynamic libc_dlsym dlsym "libc.so.3"

// Implemented in sys_qnx_386.s.
func qnxCpuid(eax, ecx uint32) (a, b, c, d uint32)
func qnxGO386Softfloat() bool
