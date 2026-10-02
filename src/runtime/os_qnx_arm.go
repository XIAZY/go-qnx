// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

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
	p := dlsym(^uintptr(1), &syspagePtrName[0]) // RTLD_DEFAULT is (void *)-2
	if p == 0 {
		return 0
	}
	sp := *(*uintptr)(unsafe.Pointer(p))
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

//go:nosplit
func cputicks() int64 {
	// nanotime() is a poor approximation of CPU ticks that is enough for
	// the profiler. qnx/arm has no cheap cycle counter it can read from
	// user space, so unlike qnx/386 (see tsc_qnx_386.go) nanotime is
	// CLOCK_MONOTONIC.
	return nanotime()
}

// nanotime1 reads CLOCK_MONOTONIC. QNX 6.5's monotonic clock advances
// once per timer tick, 1 ms by default; qnx/arm has no user-space cycle
// counter to refine it with, as qnx/386 does with the TSC.
//
//go:nosplit
func nanotime1() int64 {
	return clockMonotonic()
}

// qnxInitTSC has nothing to do on arm: nanotime is CLOCK_MONOTONIC. It
// is called unconditionally from osinit (see os_qnx.go).
func qnxInitTSC() {}

// qnxCheckTSC has nothing to do on arm; see tsc_qnx_386.go for the x86
// version that sysmon calls.
func qnxCheckTSC() {}

// __tls returns the current thread's control block, where save_g and
// load_g (tls_qnx_arm.s) keep g. It is a pure libc function.
//
//go:cgo_import_dynamic libc___tls __tls "libc.so.3"
