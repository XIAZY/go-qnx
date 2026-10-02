// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// QNX Neutrino 6.5. Every system service goes through libc, which is
// also the dynamic loader (/usr/lib/ldqnx.so.2 is libc.so.3): most
// POSIX calls are messages to resource managers, not kernel traps.
//
// Three properties of QNX 6.5 shape this file:
//
//   - There is no ELF thread-local storage. g lives in the __reserved1
//     field of the thread's control block (see cmd/internal/obj/x86 and
//     tlsGOffset below).
//   - There is no sigaltstack and SA_ONSTACK is rejected: handlers run
//     on the interrupted stack. sigtramp switches to the gsignal stack
//     itself, and every goroutine stack reserves stackSystem bytes at
//     its bottom for the kernel's signal frame (721 bytes measured).
//   - There is no SA_RESTART, so interrupted calls fail with EINTR and
//     are retried by their callers.

package runtime

import (
	"internal/abi"
	"internal/runtime/atomic"
	"unsafe"
)

// tlsGOffset is the offset of g in struct _thread_local_storage: its
// __reserved1 field. Known to the assembler as x86.QNXTLSGOffset.
const tlsGOffset = 0x24

const _SS_DISABLE = 2

type mOS struct {
	// sem is the M's semaphore. It lives in the M, which is never
	// moved or freed while the thread may use it.
	sem semt
	// semInit records that sem has been initialized: semacreate is
	// called on every slow lock path, and QNX's sem_init fails with
	// EBUSY on a semaphore that is already initialized.
	semInit bool
	// delayedFaults counts the consecutive synchronous faults that
	// sigDelayedFault let go (see there).
	delayedFaults uint32
	// qnxBlockopMask is the signal mask that qnxBlockopEnd restores.
	qnxBlockopMask sigset
	// qnxBlockop is set between qnxBlockopBegin and qnxBlockopEnd.
	qnxBlockop bool
	// qnxInBlockop is set while the M is inside a bracketed libc call.
	qnxInBlockop bool

	// For CPU profiling (cpuprof_qnx.go). The profiling thread owns
	// profTid, profClock and profLast; profPending is shared with the
	// signal handler.
	profTid     uint64
	profClock   int32
	profLast    int64
	profPending atomic.Uint32
}

// sigset_all holds every signal an application may use. QNX reserves
// the numbers from _NSIG up (SIGSELECT, SIGPHOTON) for libc and the
// kernel, and Go must not change their disposition in thread masks.
var sigset_all = sigset{[2]uint32{^uint32(0), (1<<(_NSIG-1-32) - 1)}}

func getCPUCount() int32 {
	n := sysconf(_SC_NPROCESSORS_ONLN)
	if n < 1 {
		return 1
	}
	return int32(n)
}

func getPageSize() uintptr {
	n := sysconf(_SC_PAGESIZE)
	if n <= 0 {
		return 0
	}
	return uintptr(n)
}

//go:nosplit
func semacreate(mp *m) {
	if mp.semInit {
		return
	}
	if e := sem_init(&mp.sem, 0, 0); e != 0 {
		semfail("sem_init", e)
	}
	mp.semInit = true
}

// The sem_* wrappers return 0 or an errno value, which their
// trampolines read immediately after the call.

//go:nosplit
func semasleep(ns int64) int32 {
	mp := getg().m
	if ns < 0 {
		for {
			e := sem_wait(&mp.sem)
			if e == 0 {
				return 0
			}
			if e != _EINTR {
				semfail("sem_wait", e)
			}
		}
	}
	// The deadline is absolute and on the monotonic clock, so neither
	// EINTR retries nor changes to the wall clock move it.
	var ts timespec
	ts.setNsec(nanotime1() + ns)
	for {
		switch e := sem_timedwait_monotonic(&mp.sem, &ts); e {
		case 0:
			return 0
		case _ETIMEDOUT:
			return -1
		case _EINTR:
		default:
			semfail("sem_timedwait_monotonic", e)
		}
	}
}

//go:nosplit
func semawakeup(mp *m) {
	if e := sem_post(&mp.sem); e != 0 {
		semfail("sem_post", e)
	}
}

//go:nosplit
func semfail(what string, e int32) {
	// Callers are nosplit; print on the system stack.
	systemstack(func() {
		print("runtime: ", what, " failed: errno ", e, "\n")
		throw("semaphore failure")
	})
}

// mstart_stub provides glue code to call mstart from pthread_create.
func mstart_stub()

// Thread stacks are allocated by libc. QNX's default is small; ask for
// the same size as other libc-threaded ports use for g0.
const threadStackSize = 256 << 10

// May run with m.p==nil, so write barriers are not allowed.
//
//go:nowritebarrierrec
func newosproc(mp *m) {
	var attr pthreadattr
	if err := pthread_attr_init(&attr); err != 0 {
		writeErrStr(failthreadcreate)
		exit(1)
	}
	if pthread_attr_setstacksize(&attr, threadStackSize) != 0 {
		writeErrStr(failthreadcreate)
		exit(1)
	}
	var stacksize uintptr
	if pthread_attr_getstacksize(&attr, &stacksize) != 0 {
		writeErrStr(failthreadcreate)
		exit(1)
	}
	mp.g0.stack.hi = stacksize // for mstart

	if pthread_attr_setdetachstate(&attr, _PTHREAD_CREATE_DETACHED) != 0 {
		writeErrStr(failthreadcreate)
		exit(1)
	}

	// The new thread starts with all signals blocked; minit unblocks
	// the ones it handles once g and gsignal are in place.
	var oset sigset
	sigprocmask(_SIG_SETMASK, &sigset_all, &oset)
	err := retryOnEAGAIN(func() int32 {
		return pthread_create(&attr, abi.FuncPCABI0(mstart_stub), unsafe.Pointer(mp))
	})
	sigprocmask(_SIG_SETMASK, &oset, nil)
	if err != 0 {
		writeErrStr(failthreadcreate)
		exit(1)
	}

	pthread_attr_destroy(&attr)
}

func osinit() {
	numCPUStartup = getCPUCount()
	physPageSize = getPageSize()
	qnxInitTSC()
}

var urandom_dev = []byte("/dev/urandom\x00")

//go:nosplit
func readRandom(r []byte) int {
	fd := open(&urandom_dev[0], _O_RDONLY|_O_CLOEXEC, 0)
	if fd < 0 {
		return -1
	}
	n := read(fd, unsafe.Pointer(&r[0]), int32(len(r)))
	closefd(fd)
	return int(n)
}

func goenvs() {
	goenvs_unix()
}

// Called to initialize a new m (including the bootstrap m).
// Called on the parent thread (main thread in case of bootstrap), can allocate memory.
func mpreinit(mp *m) {
	mp.gsignal = malg(32 * 1024)
	mp.gsignal.m = mp
}

// Called to initialize a new m (including the bootstrap m).
// Called on the new thread, cannot allocate memory.
func minit() {
	getg().m.procid = uint64(pthread_self())
	minitSignals()
}

// Called from dropm to undo the effect of an minit.
//
//go:nosplit
func unminit() {
	unminitSignals()
	getg().m.procid = 0
}

// Called from mexit, but not from dropm, to undo the effect of thread-owned
// resources in minit, semacreate, or elsewhere. Do not take locks after calling this.
//
// This always runs without a P, so //go:nowritebarrierrec is required.
//
//go:nowritebarrierrec
func mdestroy(mp *m) {
	// The m's memory may be reused for another m, and QNX's sem_init
	// fails with EBUSY at an address that still holds a semaphore.
	if mp.semInit {
		sem_destroy(&mp.sem)
		mp.semInit = false
	}
}

func sigtramp()

//go:nosplit
//go:nowritebarrierrec
func setsig(i uint32, fn uintptr) {
	var sa sigactiont
	// No SA_ONSTACK (sigtramp moves to the gsignal stack itself) and
	// no SA_RESTART (QNX 6.5 has neither).
	sa.sa_flags = _SA_SIGINFO
	sa.sa_mask = sigset_all
	// Leave the synchronous signals unblocked in every handler: see
	// sigDelayedFault.
	for _, s := range [...]int{_SIGSEGV, _SIGBUS, _SIGFPE, _SIGILL, _SIGTRAP} {
		sigdelset(&sa.sa_mask, s)
	}
	if fn == abi.FuncPCABIInternal(sighandler) { // abi.FuncPCABIInternal(sighandler) matches the callers in signal_unix.go
		fn = abi.FuncPCABI0(sigtramp)
	}
	sa.sa_handler = fn
	sigaction(i, &sa, nil)
}

// setsigstack would add SA_ONSTACK to a handler installed by non-Go
// code. QNX has no alternate signal stacks, so there is nothing to do.
//
//go:nosplit
//go:nowritebarrierrec
func setsigstack(i uint32) {
}

//go:nosplit
//go:nowritebarrierrec
func getsig(i uint32) uintptr {
	var sa sigactiont
	sigaction(i, nil, &sa)
	return sa.sa_handler
}

// sigaltstack reports that no alternate signal stack is set, which is
// always true on QNX, and ignores requests to set one. signal_unix.go
// then treats the gsignal stack as the signal stack, and sigtramp
// makes that so.
//
//go:nosplit
func sigaltstack(new, old *stackt) {
	if old != nil {
		*old = stackt{ss_flags: _SS_DISABLE}
	}
}

// setSignalstackSP sets the ss_sp field of a stackt.
//
//go:nosplit
func setSignalstackSP(s *stackt, sp uintptr) {
	s.ss_sp = sp
}

//go:nosplit
//go:nowritebarrierrec
func sigaddset(mask *sigset, i int) {
	mask.__bits[(i-1)/32] |= 1 << ((uint32(i) - 1) & 31)
}

func sigdelset(mask *sigset, i int) {
	mask.__bits[(i-1)/32] &^= 1 << ((uint32(i) - 1) & 31)
}

//go:nosplit
func (c *sigctxt) fixsigcode(sig uint32) {
}

//go:nosplit
func validSIGPROF(mp *m, c *sigctxt) bool {
	return true
}

//go:nosplit
func raise(sig uint32) {
	pthread_kill(pthread_self(), int32(sig))
}

func signalM(mp *m, sig int) {
	pthread_kill(pthread(mp.procid), int32(sig))
}

// sigPerThreadSyscall is only used on linux, so we assign a bogus signal
// number.
const sigPerThreadSyscall = 1 << 31

//go:nosplit
func runPerThreadSyscall() {
	throw("runPerThreadSyscall only valid on linux")
}

// Threads exit by returning from mstart_stub to libc.
func exitThread(wait *atomic.Uint32) {
	throw("exitThread")
}

//go:nowritebarrierrec
//go:nosplit
func libpreinit() {}

//go:nowritebarrierrec
//go:nosplit
func newosproc0(stacksize uintptr, fn unsafe.Pointer) {
	throw("bad newosproc0")
}

// madvise is spelled posix_madvise on QNX 6.5, which only knows
// POSIX_MADV_DONTNEED; mem_bsd.go uses both names below for it.
const (
	_MADV_DONTNEED = _POSIX_MADV_DONTNEED
	_MADV_FREE     = _POSIX_MADV_DONTNEED
)

//go:nosplit
func madvise(addr unsafe.Pointer, n uintptr, flags int32) {
	posix_madvise(addr, n, flags)
}

//go:nosplit
func setNonblock(fd int32) {
	flags, _ := fcntl(fd, _F_GETFL, 0)
	if flags != -1 {
		fcntl(fd, _F_SETFL, flags|_O_NONBLOCK)
	}
}
