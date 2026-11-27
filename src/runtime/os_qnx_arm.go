// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
	"internal/cpu"
	"unsafe"
)

const (
	// From <sys/syspage.h> and <arm/syspage.h> of QNX 6.5.0. These are
	// bits in a cpuinfo_entry's flags word.
	_CPU_FLAG_FPU      = 1 << 31 // CPU has floating-point hardware
	_ARM_CPU_FLAG_V7   = 0x20    // ARMv7 architecture
	_ARM_CPU_FLAG_NEON = 0x40    // Advanced SIMD (Neon)
	// _ARM_CPU_FLAG_VFP_D32 (VFP has 32 double registers) is a BlackBerry
	// 10 addition; it is not defined in the QNX 6.5.0 arm/syspage.h, where
	// only the NEON bit tells us the register count. It is harmless to AND
	// against a word that never has it set on 6.5.0.
	_ARM_CPU_FLAG_VFP_D32 = 0x800
)

// qnxHasFPU reports whether the CPU has floating-point hardware. sigtramp
// (sys_qnx_arm.s) saves the VFP registers whenever it does, regardless of
// how Go was built: a softfloat Go binary on an FPU machine still
// interrupts libc code that uses VFP. Set once in checkgoarm.
var qnxHasFPU bool

// qnxVFPd32 reports whether the CPU has the upper VFP double registers
// d16-d31, so that sigtramp saves them too. Set once in checkgoarm. Neon
// implies d16-d31 exist even when VFP_D32 is not set.
var qnxVFPd32 bool

// qnxSyspage returns the address of the kernel's system page, or 0 if
// it cannot be found. libc keeps it in the global _syspage_ptr.
//
//go:nosplit
func qnxSyspage() uintptr {
	p := dlsym(^uintptr(1), &syspagePtrName[0]) // RTLD_DEFAULT is (void *)-2
	if p == 0 {
		return 0
	}
	return *(*uintptr)(unsafe.Pointer(p))
}

// qnxCPUFlags returns the flags of the first CPU from the kernel's
// system page, or 0 if it cannot be read. QNX has no auxv, so this is
// the only way to learn the CPU's features.
//
// The offsets were checked against <sys/syspage.h> of QNX 6.5.0: in
// struct syspage_entry the syspage_entry_info cpuinfo is at 24 (after
// the four _Uint16t size, total_size, type and num_cpu and four 4-byte
// syspage_entry_info), and its entry_off is its first _Uint16t; in
// struct cpuinfo_entry the _Uint32t flags is at 8 (after cpu and
// speed). SYSPAGE_ENTRY(cpuinfo) is the system page plus entry_off.
//
//go:nosplit
func qnxCPUFlags() uint32 {
	sp := qnxSyspage()
	if sp == 0 {
		return 0
	}
	off := *(*uint16)(unsafe.Pointer(sp + 24))
	return *(*uint32)(unsafe.Pointer(sp + uintptr(off) + 8))
}

func checkgoarm() {
	flags := qnxCPUFlags()
	qnxHasFPU = flags&_CPU_FLAG_FPU != 0
	qnxVFPd32 = flags&(_ARM_CPU_FLAG_NEON|_ARM_CPU_FLAG_VFP_D32) != 0
	if flags&_ARM_CPU_FLAG_NEON != 0 {
		// QNX has no auxv, so tell internal/cpu about Neon with the
		// HWCAP bit that Linux and FreeBSD use for it.
		cpu.HWCap |= 1 << 12 // HWCAP_NEON
	}
	if flags&_CPU_FLAG_FPU == 0 && goarmsoftfp == 0 {
		print("runtime: this CPU has no floating point hardware, so it cannot run\n")
		print("a binary compiled for hard floating point. Recompile adding ,softfloat\n")
		print("to GOARM.\n")
		exit(1)
	}
	if goarm > 6 && flags&_ARM_CPU_FLAG_V7 == 0 && goarmsoftfp == 0 {
		print("runtime: this CPU is not ARMv7, so it cannot run a binary compiled for\n")
		print("GOARM=7. Recompile with GOARM=6, or add ,softfloat to GOARM.\n")
		exit(1)
	}
}

// cputicks returns QNX's ClockCycles, a free-running counter whose rate
// the system page gives (cycles_per_sec). On ARM, libc emulates it with
// a call into the kernel, which on a 19.2 MHz device took about 350 ns,
// for a resolution of about 52 ns; nanotime here is CLOCK_MONOTONIC,
// which advances in 1 ms ticks, too coarse to order events such as
// debuglog's. The runtime calibrates ticks against nanotime itself and
// does not need them synchronised across CPUs.
//
// The kernel serialises the call: with all four CPUs of that device
// calling at once, each call took about 1.3 µs, against about 430 ns
// for clock_gettime (and the system page read that nanotime uses now
// costs no call at all). That is why nanotime does not use ClockCycles.
//
//go:nosplit
func cputicks() int64 {
	var r [2]uint32 // low, high
	libcCall(unsafe.Pointer(abi.FuncPCABI0(clockCycles_trampoline)), unsafe.Pointer(&r))
	return int64(uint64(r[1])<<32 | uint64(r[0]))
}
func clockCycles_trampoline()

//go:cgo_import_dynamic libc_ClockCycles ClockCycles "libc.so.3"

// nanotime1 returns CLOCK_MONOTONIC. QNX 6.5's monotonic clock advances
// once per timer tick, 1 ms by default; qnx/arm has no user-space cycle
// counter to refine it with, as qnx/386 does with the TSC. The kernel
// keeps the same value in the qtime entry of the system page, which
// qnxInitTSC checked against clock_gettime at start-up: if it agreed,
// nanotime reads it from memory, with no call into libc and none into the
// kernel, which serialises clock_gettime across CPUs. Otherwise nanotime
// calls clock_gettime.
//
//go:nosplit
func nanotime1() int64 {
	if q := qnxQtime; q != 0 {
		for {
			n, s := qtimeLoadNsec(q)
			if n == s {
				return int64(n)
			}
		}
	}
	return clockMonotonic()
}

// qnxQtime is the address of the system page's qtime entry if qnxInitTSC
// found it to match the libc clocks, and 0 if not (the clocks are then
// read through libc). qnxQtimeWall reports that walltime matched too.
//
// The layout is struct qtime_entry from <sys/syspage.h> of QNX 6.5.0,
// confirmed on a BlackBerry Q10 (QNX 8.0.0) by dumping it: the volatile
// _Uint64t nsec_tod_adjust at 8 and nsec at 16, flags at 60 and the
// volatile _Uint64t nsec_stable at 72. nsec is CLOCK_MONOTONIC in ns,
// advanced once per tick, and CLOCK_REALTIME is nsec + nsec_tod_adjust.
// On a 32-bit CPU the kernel writes each 64-bit value as two words, so a
// read can see half of an update. With QTIME_FLAG_CHECK_STABLE (0x2) in
// flags, the kernel copies nsec to nsec_stable after each update: a read
// of nsec that equals the following read of nsec_stable is whole, since a
// half-updated value equals no whole one. This holds whatever the tick
// size, tickless kernels included.
var (
	qnxQtime     uintptr
	qnxQtimeWall bool
)

const (
	qtimeTolerance           = 2e6 // ns that qnxInitTSC allows between the clocks: two ticks
	_QTIME_FLAG_CHECK_STABLE = 0x2
)

// qtimeLoadNsec and qtimeLoadWall (sys_qnx_arm.s) read the qtime entry at
// q: nsec and then nsec_stable, and for qtimeLoadWall nsec_tod_adjust
// before and after them. ARM may reorder loads, so each load's address is
// made to depend on the value of the one before (an address dependency
// orders them) instead of using atomic.Load, whose DMB cost about 60 ns on
// a BlackBerry Q10 against about 2 ns for a plain load of the system page.
func qtimeLoadNsec(q uintptr) (nsec, stable uint64)
func qtimeLoadWall(q uintptr) (tod1, nsec, stable, tod2 uint64)

// qtimeDiv1e9 returns ns / 1e9. A 64-bit division is a call into a
// shift-and-subtract loop on 32-bit ARM, which took about 590 ns on a
// BlackBerry Q10; this is the exact reciprocal multiplication compilers
// emit for it: the high 64 bits of (ns>>9) * 0x44B82FA09B5A53, shifted
// right by 11.
//
//go:nosplit
func qtimeDiv1e9(ns uint64) uint64 {
	a, b := ns>>9, uint64(0x44B82FA09B5A53)
	a0, a1 := a&0xFFFFFFFF, a>>32
	b0, b1 := b&0xFFFFFFFF, b>>32
	t := a1*b0 + (a0*b0)>>32
	w := t&0xFFFFFFFF + a0*b1
	return (a1*b1 + t>>32 + w>>32) >> 11
}

// qnxFastWalltime returns CLOCK_REALTIME from the system page, if
// qnxInitTSC found that it matches clock_gettime; see walltime.
//
// nsec_tod_adjust has no stable copy. It changes only when the clock is
// set or adjusted, and a value that reads the same before and after the
// checked read of nsec is taken as whole.
//
// ponytail: a set that writes nsec_tod_adjust while both of its reads fall
// between the kernel's two stores of it would still give a torn value; the
// upgrade is to re-read until two such reads agree across a ClockId change
// counter, if the kernel ever exposes one.
//
//go:nosplit
func qnxFastWalltime() (sec int64, nsec int32, ok bool) {
	if !qnxQtimeWall {
		return 0, 0, false
	}
	for {
		t1, n, s, t2 := qtimeLoadWall(qnxQtime)
		if n == s && t1 == t2 {
			ns := n + t1
			sec = int64(qtimeDiv1e9(ns))
			return sec, int32(ns - uint64(sec)*1e9), true
		}
	}
}

// qnxCpupagePtr is the address of libc's _cpupage_ptr variable (not its
// value) once qnxInitCpupage has checked it, and 0 until then or if the
// check failed; save_g and load_g (tls_qnx_arm.s) read the thread control
// block through it instead of calling libc's __tls.
var qnxCpupagePtr uintptr

// qnxInitCpupage sets qnxCpupagePtr. Disassembling BlackBerry 10's
// libc.so.3 shows __tls() to be: load &_cpupage_ptr from the GOT,
// r3 = *(&_cpupage_ptr), r0 = *r3, return; and the first field of the
// kernel's cpupage_entry is tls, the current thread's control block. That
// is assumed to hold for any libc this runtime runs on, so check it: the
// control block must be non-nil and its g slot (0x24) must hold g. It does
// because rt0_go calls save_g before osinit, and every setg since then
// has stored the current g there.
func qnxInitCpupage() {
	p := dlsym(^uintptr(1), &cpupagePtrName[0]) // RTLD_DEFAULT is (void *)-2
	if p == 0 {
		return
	}
	cp := *(*uintptr)(unsafe.Pointer(p))
	if cp == 0 {
		return
	}
	tcb := *(*uintptr)(unsafe.Pointer(cp))
	if tcb == 0 || *(*uintptr)(unsafe.Pointer(tcb + 0x24)) != uintptr(unsafe.Pointer(getg())) {
		return
	}
	qnxCpupagePtr = p
}

// qnxInitTSC, called unconditionally from osinit (see os_qnx.go), also
// calls qnxInitCpupage. It decides whether nanotime and walltime can read
// the system page's qtime entry instead of calling libc. It requires the
// entry to lie inside the system page, to be 8-byte aligned and to have
// QTIME_FLAG_CHECK_STABLE, and each clock is then used only if one read of
// it agrees with the libc clock to within qtimeTolerance. Both agreed on
// a BlackBerry Q10 (QNX 8.0.0).
func qnxInitTSC() {
	qnxInitCpupage()
	sp := qnxSyspage()
	if sp == 0 {
		return
	}
	// struct syspage_entry: _Uint16t total_size at 2; the qtime
	// syspage_entry_info's entry_off at 32.
	total := uintptr(*(*uint16)(unsafe.Pointer(sp + 2)))
	off := uintptr(*(*uint16)(unsafe.Pointer(sp + 32)))
	if off == 0 || off+80 > total || (sp+off)&7 != 0 {
		return
	}
	q := sp + off
	if *(*uint32)(unsafe.Pointer(q + 60))&_QTIME_FLAG_CHECK_STABLE == 0 {
		return
	}
	qnxQtime = q // nanotime1 reads it from here on
	m := clockMonotonic()
	if qm := nanotime1(); qm < m-qtimeTolerance || qm > m+qtimeTolerance {
		qnxQtime = 0
		return
	}
	qnxQtimeWall = true
	sec, nsec := libcWalltime()
	w := int64(sec)*1e9 + int64(nsec)
	if qs, qn, _ := qnxFastWalltime(); qs*1e9+int64(qn) < w-qtimeTolerance || qs*1e9+int64(qn) > w+qtimeTolerance {
		qnxQtimeWall = false
	}
}

// qnxCheckTSC has nothing to do on arm; see tsc_qnx_386.go for the x86
// version that sysmon calls.
func qnxCheckTSC() {}

// __tls returns the current thread's control block, where save_g and
// load_g (tls_qnx_arm.s) keep g. It is a pure libc function. They call it
// only until qnxInitCpupage lets them read the control block directly.
//
//go:cgo_import_dynamic libc___tls __tls "libc.so.3"
