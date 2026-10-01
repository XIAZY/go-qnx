// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// System calls and other sys.stuff for 386, QNX Neutrino 6.5.
// Everything goes through libc.so.3; this file holds the trampolines
// that convert from the Go to the C calling convention.
//
// A trampoline is called by libcCall or asmcgocall on the g0 stack
// with one argument, a pointer to its arguments, and returns an int32
// in AX. Each one keeps the caller's frame pointer in BP and addresses
// that pointer as 8(BP), aligns the stack to 16 bytes before calling
// into libc, and reads errno right after a failing call.

#include "go_asm.h"
#include "go_tls.h"
#include "textflag.h"

#define ENTER(n) \
	PUSHL	BP; \
	MOVL	SP, BP; \
	SUBL	$(n), SP; \
	ANDL	$~15, SP; \
	MOVL	8(BP), DX

#define LEAVE \
	MOVL	BP, SP; \
	POPL	BP; \
	RET

// ERRNO loads errno into AX. It clobbers CX and DX. It is two
// instructions: never skip it with an n(PC) jump, only with a label.
#define ERRNO \
	CALL	libc___get_errno_ptr(SB); \
	MOVL	(AX), AX

// Only rt0_go's LDT setup refers to setldt, and QNX skips it: libc has
// already given every thread its control block.
TEXT runtime·setldt(SB),NOSPLIT,$0
	RET

// qnxtlsbad reports that the __reserved1 slot of a thread's control
// block, where g lives, was not zero before Go first used it: some
// other code is using the slot, and Go cannot run safely in this process.
TEXT runtime·qnxtlsbad(SB),NOSPLIT|NOFRAME,$0
	ANDL	$~15, SP
	SUBL	$16, SP
	MOVL	$2, 0(SP)
	MOVL	$qnxtlsbadmsg<>(SB), AX
	MOVL	AX, 4(SP)
	MOVL	$59, 8(SP)		// len(qnxtlsbadmsg)
	CALL	libc_write(SB)
	MOVL	$2, 0(SP)
	CALL	libc__exit(SB)
	INT	$3

DATA qnxtlsbadmsg<>+0x00(SB)/59, $"fatal error: QNX thread control block slot for g is in use\n"
GLOBL qnxtlsbadmsg<>(SB), RODATA, $59

// mstart_stub is the first function executed on a new thread started by
// pthread_create. It sets g and calls mstart.
// Note: called with the C calling convention.
TEXT runtime·mstart_stub(SB),NOSPLIT,$28
	NOP	SP	// tell vet SP changed - stop checking offsets

	// We are already on m's g0 stack.

	// Save callee-save registers.
	MOVL	BX, bx-4(SP)
	MOVL	BP, bp-8(SP)
	MOVL	SI, si-12(SP)
	MOVL	DI, di-16(SP)

	MOVL	32(SP), AX	// m
	MOVL	m_g0(AX), DX
	get_tls(CX)
	CMPL	g(CX), $0
	JEQ	2(PC)
	CALL	runtime·qnxtlsbad(SB)
	MOVL	DX, g(CX)

	CALL	runtime·mstart(SB)

	// Restore callee-save registers.
	MOVL	di-16(SP), DI
	MOVL	si-12(SP), SI
	MOVL	bp-8(SP),  BP
	MOVL	bx-4(SP),  BX

	// Go is all done with this OS thread.
	// Tell pthread everything is ok (we never join with this thread, so
	// the value here doesn't really matter).
	MOVL	$0, AX
	RET

TEXT runtime·sigfwd(SB),NOSPLIT,$0-16
	MOVL	fn+0(FP), AX
	MOVL	sig+4(FP), BX
	MOVL	info+8(FP), CX
	MOVL	ctx+12(FP), DX
	MOVL	SP, SI
	SUBL	$32, SP
	ANDL	$~15, SP	// align stack: handler might be a C function
	MOVL	BX, 0(SP)
	MOVL	CX, 4(SP)
	MOVL	DX, 8(SP)
	MOVL	SI, 12(SP)	// save SI: handler might be a Go function
	CALL	AX
	MOVL	12(SP), AX
	MOVL	AX, SP
	RET

// sigtramp is the signal handler, called by libc with the C calling
// convention: sigtramp(signo, info, context).
//
// QNX has no alternate signal stacks: the kernel builds the signal
// frame on the interrupted stack, which may be a goroutine stack with
// only stackSystem bytes to spare. So sigtramp does as little as
// possible there and moves to the M's gsignal stack before calling
// Go, unless it is not on a Go thread or is already on gsignal. All
// signals are blocked while it runs (setsig's sa_mask), so frames
// cannot nest.
//
// It also preserves errno, as a signal handler must: the interrupted
// code may be between a failing libc call and its read of errno. And it
// saves the FPU and SSE registers with FXSAVE before calling Go and
// restores them with FXRSTOR after: QNX 6.5 does not reliably restore
// them when a handler returns, and Go's handler code uses X registers
// (memmove, float conversions). The save area is on the stack sigtramp
// calls Go on, never on a goroutine stack.
TEXT runtime·sigtramp(SB),NOSPLIT|TOPFRAME|NOFRAME,$0
	PUSHL	BP
	MOVL	SP, BP		// 8(BP) signo, 12(BP) info, 16(BP) context
	PUSHL	BX
	PUSHL	SI
	PUSHL	DI
	ERRNO
	PUSHL	AX		// saved errno
	MOVL	SP, SI		// the stack to return to

	get_tls(CX)
	MOVL	g(CX), AX
	TESTL	AX, AX
	JZ	call		// not a Go thread, or g not set yet
	MOVL	g_m(AX), AX
	TESTL	AX, AX
	JZ	call
	MOVL	m_gsignal(AX), AX
	TESTL	AX, AX
	JZ	call
	MOVL	(g_stack+stack_hi)(AX), DI
	CMPL	SP, (g_stack+stack_lo)(AX)
	JCS	switch		// SP < lo
	CMPL	SP, DI
	JCS	call		// lo <= SP < hi: already on gsignal
switch:
	MOVL	DI, SP
call:
	SUBL	$(16+512), SP
	ANDL	$~15, SP
	FXSAVE	16(SP)		// 512 bytes, 16-byte aligned
	MOVL	8(BP), AX
	MOVL	AX, 0(SP)	// signo
	MOVL	12(BP), AX
	MOVL	AX, 4(SP)	// info
	MOVL	16(BP), AX
	MOVL	AX, 8(SP)	// context
	MOVL	SI, 12(SP)	// Go code may clobber SI
	CALL	runtime·sigtrampgo(SB)
	FXRSTOR	16(SP)
	MOVL	12(SP), SP

	CALL	libc___get_errno_ptr(SB)
	POPL	BX
	MOVL	BX, (AX)	// restore errno
	POPL	DI
	POPL	SI
	POPL	BX
	POPL	BP
	RET

TEXT runtime·pthread_attr_init_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// attr
	CALL	libc_pthread_attr_init(SB)
	LEAVE

TEXT runtime·pthread_attr_destroy_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// attr
	CALL	libc_pthread_attr_destroy(SB)
	LEAVE

TEXT runtime·pthread_attr_getstacksize_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	4(DX), BX
	MOVL	AX, 0(SP)		// attr
	MOVL	BX, 4(SP)		// size
	CALL	libc_pthread_attr_getstacksize(SB)
	LEAVE

TEXT runtime·pthread_attr_setstacksize_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	4(DX), BX
	MOVL	AX, 0(SP)		// attr
	MOVL	BX, 4(SP)		// size
	CALL	libc_pthread_attr_setstacksize(SB)
	LEAVE

TEXT runtime·pthread_attr_setdetachstate_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	4(DX), BX
	MOVL	AX, 0(SP)		// attr
	MOVL	BX, 4(SP)		// state
	CALL	libc_pthread_attr_setdetachstate(SB)
	LEAVE

TEXT runtime·pthread_create_trampoline(SB),NOSPLIT,$0
	ENTER(32)
	NOP	SP	// tell vet SP changed - stop checking offsets
	LEAL	16(SP), AX
	MOVL	AX, 0(SP)		// &thread ID (discarded)
	MOVL	0(DX), AX
	MOVL	AX, 4(SP)		// attr
	MOVL	4(DX), AX
	MOVL	AX, 8(SP)		// start
	MOVL	8(DX), AX
	MOVL	AX, 12(SP)		// arg
	CALL	libc_pthread_create(SB)
	LEAVE

TEXT runtime·pthread_self_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	CALL	libc_pthread_self(SB)
	MOVL	8(BP), DX
	MOVL	AX, 0(DX)		// result
	LEAVE

TEXT runtime·pthread_kill_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	4(DX), BX
	MOVL	AX, 0(SP)		// thread
	MOVL	BX, 4(SP)		// sig
	CALL	libc_pthread_kill(SB)
	LEAVE

// The sem_* trampolines return 0 or errno.
TEXT runtime·sem_init_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// sem
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// pshared
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// value
	CALL	libc_sem_init(SB)
	TESTL	AX, AX
	JEQ	ok
	ERRNO
ok:
	LEAVE

TEXT runtime·sem_wait_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// sem
	CALL	libc_sem_wait(SB)
	TESTL	AX, AX
	JEQ	ok
	ERRNO
ok:
	LEAVE

TEXT runtime·sem_timedwait_monotonic_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// sem
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// abstime
	CALL	libc_sem_timedwait_monotonic(SB)
	TESTL	AX, AX
	JEQ	ok
	ERRNO
ok:
	LEAVE

TEXT runtime·sem_post_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// sem
	CALL	libc_sem_post(SB)
	TESTL	AX, AX
	JEQ	ok
	ERRNO
ok:
	LEAVE

TEXT runtime·sem_destroy_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// sem
	CALL	libc_sem_destroy(SB)
	TESTL	AX, AX
	JEQ	ok
	ERRNO
ok:
	LEAVE

TEXT runtime·sched_yield_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	CALL	libc_sched_yield(SB)
	LEAVE

TEXT runtime·exit_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// status
	CALL	libc_exit(SB)
	MOVL	$0xf1, 0xf1		// crash
	LEAVE

TEXT runtime·_exit_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// status
	CALL	libc__exit(SB)
	MOVL	$0xf1, 0xf1		// crash
	LEAVE

TEXT runtime·getpid_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	CALL	libc_getpid(SB)
	MOVL	8(BP), DX
	MOVL	AX, 0(DX)		// result
	LEAVE

TEXT runtime·raiseproc_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	CALL	libc_getpid(SB)
	MOVL	AX, 0(SP)		// pid
	MOVL	8(BP), DX
	MOVL	0(DX), AX
	MOVL	AX, 4(SP)		// sig
	CALL	libc_kill(SB)
	LEAVE

TEXT runtime·getuid_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	CALL	libc_getuid(SB)
	MOVL	8(BP), DX
	MOVL	AX, 0(DX)
	LEAVE

TEXT runtime·geteuid_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	CALL	libc_geteuid(SB)
	MOVL	8(BP), DX
	MOVL	AX, 0(DX)
	LEAVE

TEXT runtime·getgid_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	CALL	libc_getgid(SB)
	MOVL	8(BP), DX
	MOVL	AX, 0(DX)
	LEAVE

TEXT runtime·getegid_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	CALL	libc_getegid(SB)
	MOVL	8(BP), DX
	MOVL	AX, 0(DX)
	LEAVE

// mmap's off_t is 32 bits here: the plain mmap symbol, not mmap64.
TEXT runtime·mmap_trampoline(SB),NOSPLIT,$0
	ENTER(32)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// addr
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// len
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// prot
	MOVL	12(DX), AX
	MOVL	AX, 12(SP)		// flags
	MOVL	16(DX), AX
	MOVL	AX, 16(SP)		// fd
	MOVL	20(DX), AX
	MOVL	AX, 20(SP)		// offset
	CALL	libc_mmap(SB)
	MOVL	$0, BX
	CMPL	AX, $-1			// MAP_FAILED
	JNE	ok
	ERRNO
	MOVL	AX, BX
	MOVL	$0, AX
ok:
	MOVL	8(BP), DX
	MOVL	AX, 24(DX)		// ret1
	MOVL	BX, 28(DX)		// ret2 (errno)
	LEAVE

TEXT runtime·munmap_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// addr
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// len
	CALL	libc_munmap(SB)
	CMPL	AX, $-1			// return errno, or 0
	JNE	munmapok
	ERRNO
munmapok:
	LEAVE

// posix_madvise returns an error number, not -1 and errno.
TEXT runtime·posix_madvise_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// addr
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// len
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// advice
	CALL	libc_posix_madvise(SB)
	LEAVE

TEXT runtime·open_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// path
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// flags
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// mode
	CALL	libc_open(SB)
	CMPL	AX, $-1			// return the descriptor, or -errno
	JNE	openok
	ERRNO
	NEGL	AX
openok:
	LEAVE

TEXT runtime·close_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// fd
	CALL	libc_close(SB)
	LEAVE

// read and write return the count, or -errno.
TEXT runtime·read_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// fd
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// buf
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// count
	CALL	libc_read(SB)
	CMPL	AX, $-1
	JNE	noerr
	ERRNO
	NEGL	AX
noerr:
	LEAVE

TEXT runtime·write_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// fd
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// buf
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// count
	CALL	libc_write(SB)
	CMPL	AX, $-1
	JNE	noerr
	ERRNO
	NEGL	AX
noerr:
	LEAVE

// pipe returns 0 or errno.
TEXT runtime·pipe_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	DX, 0(SP)		// fds
	CALL	libc_pipe(SB)
	TESTL	AX, AX
	JEQ	ok
	ERRNO
ok:
	LEAVE

TEXT runtime·setitimer_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// which
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// new
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// old
	CALL	libc_setitimer(SB)
	TESTL	AX, AX
	JEQ	ok
	ERRNO
	MOVL	8(BP), DX
	MOVL	AX, 12(DX)		// errno
ok:
	LEAVE

TEXT runtime·usleep_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// usec
	CALL	libc_usleep(SB)
	LEAVE

TEXT runtime·sysconf_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// name
	CALL	libc_sysconf(SB)
	LEAVE

TEXT runtime·fcntl_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// fd
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// cmd
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// arg
	CALL	libc_fcntl(SB)
	MOVL	$0, BX
	CMPL	AX, $-1
	JNE	noerr
	ERRNO
	MOVL	AX, BX
	MOVL	$-1, AX
noerr:
	MOVL	8(BP), DX
	MOVL	AX, 12(DX)		// ret
	MOVL	BX, 16(DX)		// errno
	LEAVE

// clock_gettime returns 0 or -errno.
TEXT runtime·clock_gettime_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// clock_id
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// tp
	CALL	libc_clock_gettime(SB)
	CMPL	AX, $-1
	JNE	noerr
	ERRNO
	NEGL	AX
noerr:
	LEAVE

TEXT runtime·sigaction_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// sig
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// new
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// old
	CALL	libc_sigaction(SB)
	CMPL	AX, $-1
	JNE	2(PC)
	MOVL	$0xf1, 0xf1		// crash
	LEAVE

// sigprocmask is per thread: pthread_sigmask. It returns an error
// number, and can only fail on bad arguments.
TEXT runtime·sigprocmask_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// how
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// new
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// old
	CALL	libc_pthread_sigmask(SB)
	TESTL	AX, AX
	JEQ	2(PC)
	MOVL	$0xf1, 0xf1		// crash
	LEAVE

// vforkexec_trampoline: see syscall_rawVforkExec. Its arguments are
// struct { child, arg, stk uintptr; pid int; err uintptr }.
// SI, DI and BX are callee-saved in the C ABI, so vfork preserves them in
// both processes; the child uses only them until it is on its own stack.
TEXT runtime·vforkexec_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), SI		// child function
	MOVL	4(DX), DI		// its argument
	MOVL	8(DX), BX		// top of the child's stack
	CALL	libc_vfork(SB)
	TESTL	AX, AX
	JNE	parent
	// Child. From here on nothing may be written to the parent's stack.
	MOVL	BX, SP
	ANDL	$~15, SP
	SUBL	$16, SP
	MOVL	DI, 0(SP)
	CALL	SI			// does not return
	INT	$3
parent:
	MOVL	$0, BX
	CMPL	AX, $-1
	JNE	ok
	ERRNO
	MOVL	AX, BX
	MOVL	$0, AX
ok:
	MOVL	8(BP), DX
	MOVL	AX, 12(DX)		// pid
	MOVL	BX, 16(DX)		// err
	LEAVE

// syscall calls a function in libc on behalf of the syscall package.
// It takes a pointer to a struct like:
// struct {
//	fn    uintptr
//	a1    uintptr
//	a2    uintptr
//	a3    uintptr
//	r1    uintptr
//	r2    uintptr
//	err   uintptr
// }
// It must be called on the g0 stack with the C calling convention (use
// libcCall). It expects a 32-bit result and tests AX for -1 to decide
// that the call failed and err must be set.
TEXT runtime·syscall(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	(1*4)(DX), AX
	MOVL	AX, 0(SP)		// a1
	MOVL	(2*4)(DX), AX
	MOVL	AX, 4(SP)		// a2
	MOVL	(3*4)(DX), AX
	MOVL	AX, 8(SP)		// a3
	MOVL	(0*4)(DX), AX
	CALL	AX			// fn
	MOVL	8(BP), BX
	MOVL	AX, (4*4)(BX)		// r1
	MOVL	DX, (5*4)(BX)		// r2
	CMPL	AX, $-1
	JNE	ok
	ERRNO
	MOVL	8(BP), BX
	MOVL	AX, (6*4)(BX)		// err
ok:
	MOVL	$0, AX
	LEAVE

// syscallX is like syscall but expects a 64-bit result in DX:AX
// and tests both halves for -1.
TEXT runtime·syscallX(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	(1*4)(DX), AX
	MOVL	AX, 0(SP)		// a1
	MOVL	(2*4)(DX), AX
	MOVL	AX, 4(SP)		// a2
	MOVL	(3*4)(DX), AX
	MOVL	AX, 8(SP)		// a3
	MOVL	(0*4)(DX), AX
	CALL	AX			// fn
	MOVL	8(BP), BX
	MOVL	AX, (4*4)(BX)		// r1
	MOVL	DX, (5*4)(BX)		// r2
	CMPL	AX, $-1
	JNE	ok
	CMPL	DX, $-1
	JNE	ok
	ERRNO
	MOVL	8(BP), BX
	MOVL	AX, (6*4)(BX)		// err
ok:
	MOVL	$0, AX
	LEAVE

// syscallPtr is like syscall but for functions that return a pointer:
// it treats a NULL result as failure.
TEXT runtime·syscallPtr(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	(1*4)(DX), AX
	MOVL	AX, 0(SP)		// a1
	MOVL	(2*4)(DX), AX
	MOVL	AX, 4(SP)		// a2
	MOVL	(3*4)(DX), AX
	MOVL	AX, 8(SP)		// a3
	MOVL	(0*4)(DX), AX
	CALL	AX			// fn
	MOVL	8(BP), BX
	MOVL	AX, (4*4)(BX)		// r1
	MOVL	DX, (5*4)(BX)		// r2
	TESTL	AX, AX
	JNE	ok
	ERRNO
	MOVL	8(BP), BX
	MOVL	AX, (6*4)(BX)		// err
ok:
	MOVL	$0, AX
	LEAVE

// syscall6 is syscall with six arguments:
// struct { fn, a1, a2, a3, a4, a5, a6, r1, r2, err uintptr }
TEXT runtime·syscall6(SB),NOSPLIT,$0
	ENTER(32)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	(1*4)(DX), AX
	MOVL	AX, 0(SP)
	MOVL	(2*4)(DX), AX
	MOVL	AX, 4(SP)
	MOVL	(3*4)(DX), AX
	MOVL	AX, 8(SP)
	MOVL	(4*4)(DX), AX
	MOVL	AX, 12(SP)
	MOVL	(5*4)(DX), AX
	MOVL	AX, 16(SP)
	MOVL	(6*4)(DX), AX
	MOVL	AX, 20(SP)
	MOVL	(0*4)(DX), AX
	CALL	AX
	MOVL	8(BP), BX
	MOVL	AX, (7*4)(BX)		// r1
	MOVL	DX, (8*4)(BX)		// r2
	CMPL	AX, $-1
	JNE	ok
	ERRNO
	MOVL	8(BP), BX
	MOVL	AX, (9*4)(BX)		// err
ok:
	MOVL	$0, AX
	LEAVE

TEXT runtime·syscall6X(SB),NOSPLIT,$0
	ENTER(32)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	(1*4)(DX), AX
	MOVL	AX, 0(SP)
	MOVL	(2*4)(DX), AX
	MOVL	AX, 4(SP)
	MOVL	(3*4)(DX), AX
	MOVL	AX, 8(SP)
	MOVL	(4*4)(DX), AX
	MOVL	AX, 12(SP)
	MOVL	(5*4)(DX), AX
	MOVL	AX, 16(SP)
	MOVL	(6*4)(DX), AX
	MOVL	AX, 20(SP)
	MOVL	(0*4)(DX), AX
	CALL	AX
	MOVL	8(BP), BX
	MOVL	AX, (7*4)(BX)		// r1
	MOVL	DX, (8*4)(BX)		// r2
	CMPL	AX, $-1
	JNE	ok
	CMPL	DX, $-1
	JNE	ok
	ERRNO
	MOVL	8(BP), BX
	MOVL	AX, (9*4)(BX)		// err
ok:
	MOVL	$0, AX
	LEAVE

// syscall10 is syscall with ten arguments:
// struct { fn, a1, ..., a10, r1, r2, err uintptr }
TEXT runtime·syscall10(SB),NOSPLIT,$0
	ENTER(48)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	(1*4)(DX), AX
	MOVL	AX, 0(SP)
	MOVL	(2*4)(DX), AX
	MOVL	AX, 4(SP)
	MOVL	(3*4)(DX), AX
	MOVL	AX, 8(SP)
	MOVL	(4*4)(DX), AX
	MOVL	AX, 12(SP)
	MOVL	(5*4)(DX), AX
	MOVL	AX, 16(SP)
	MOVL	(6*4)(DX), AX
	MOVL	AX, 20(SP)
	MOVL	(7*4)(DX), AX
	MOVL	AX, 24(SP)
	MOVL	(8*4)(DX), AX
	MOVL	AX, 28(SP)
	MOVL	(9*4)(DX), AX
	MOVL	AX, 32(SP)
	MOVL	(10*4)(DX), AX
	MOVL	AX, 36(SP)
	MOVL	(0*4)(DX), AX
	CALL	AX
	MOVL	8(BP), BX
	MOVL	AX, (11*4)(BX)		// r1
	MOVL	DX, (12*4)(BX)		// r2
	CMPL	AX, $-1
	JNE	ok
	ERRNO
	MOVL	8(BP), BX
	MOVL	AX, (13*4)(BX)		// err
ok:
	MOVL	$0, AX
	LEAVE

TEXT runtime·syscall10X(SB),NOSPLIT,$0
	ENTER(48)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	(1*4)(DX), AX
	MOVL	AX, 0(SP)
	MOVL	(2*4)(DX), AX
	MOVL	AX, 4(SP)
	MOVL	(3*4)(DX), AX
	MOVL	AX, 8(SP)
	MOVL	(4*4)(DX), AX
	MOVL	AX, 12(SP)
	MOVL	(5*4)(DX), AX
	MOVL	AX, 16(SP)
	MOVL	(6*4)(DX), AX
	MOVL	AX, 20(SP)
	MOVL	(7*4)(DX), AX
	MOVL	AX, 24(SP)
	MOVL	(8*4)(DX), AX
	MOVL	AX, 28(SP)
	MOVL	(9*4)(DX), AX
	MOVL	AX, 32(SP)
	MOVL	(10*4)(DX), AX
	MOVL	AX, 36(SP)
	MOVL	(0*4)(DX), AX
	CALL	AX
	MOVL	8(BP), BX
	MOVL	AX, (11*4)(BX)		// r1
	MOVL	DX, (12*4)(BX)		// r2
	CMPL	AX, $-1
	JNE	ok
	CMPL	DX, $-1
	JNE	ok
	ERRNO
	MOVL	8(BP), BX
	MOVL	AX, (13*4)(BX)		// err
ok:
	MOVL	$0, AX
	LEAVE

// dlsym returns the address, or 0.
TEXT runtime·dlsym_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// handle
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// name
	CALL	libc_dlsym(SB)
	LEAVE

// func qnxCpuid(eax, ecx uint32) (a, b, c, d uint32)
TEXT runtime·qnxCpuid(SB),NOSPLIT,$0-24
	MOVL	eax+0(FP), AX
	MOVL	ecx+4(FP), CX
	CPUID
	MOVL	AX, a+8(FP)
	MOVL	BX, b+12(FP)
	MOVL	CX, c+16(FP)
	MOVL	DX, d+20(FP)
	RET

// func qnxGO386Softfloat() bool
TEXT runtime·qnxGO386Softfloat(SB),NOSPLIT,$0-1
#ifdef GO386_softfloat
	MOVB	$1, ret+0(FP)
#else
	MOVB	$0, ret+0(FP)
#endif
	RET

// Kernel calls for the network poller (netpoll_qnx.go). The _r
// forms return -errno instead of setting errno.

TEXT runtime·channelCreate_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// flags
	CALL	libc_ChannelCreate_r(SB)
	LEAVE

TEXT runtime·connectAttach_trampoline(SB),NOSPLIT,$0
	ENTER(32)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// nd
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// pid
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// chid
	MOVL	12(DX), AX
	MOVL	AX, 12(SP)		// index
	MOVL	16(DX), AX
	MOVL	AX, 16(SP)		// flags
	CALL	libc_ConnectAttach_r(SB)
	LEAVE

TEXT runtime·msgReceivePulse_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// chid
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// pulse
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// bytes
	MOVL	12(DX), AX
	MOVL	AX, 12(SP)		// info
	CALL	libc_MsgReceivePulse_r(SB)
	LEAVE

TEXT runtime·msgSendPulse_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// coid
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// priority
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// code
	MOVL	12(DX), AX
	MOVL	AX, 12(SP)		// value
	CALL	libc_MsgSendPulse_r(SB)
	LEAVE

TEXT runtime·timerTimeout_trampoline(SB),NOSPLIT,$0
	ENTER(32)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// id
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// flags
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// notify
	MOVL	12(DX), AX
	MOVL	AX, 12(SP)		// ntime
	MOVL	16(DX), AX
	MOVL	AX, 16(SP)		// otime
	CALL	libc_TimerTimeout_r(SB)
	LEAVE

TEXT runtime·ionotify_trampoline(SB),NOSPLIT,$0
	ENTER(16)
	NOP	SP	// tell vet SP changed - stop checking offsets
	MOVL	0(DX), AX
	MOVL	AX, 0(SP)		// fd
	MOVL	4(DX), AX
	MOVL	AX, 4(SP)		// action
	MOVL	8(DX), AX
	MOVL	AX, 8(SP)		// flags
	MOVL	12(DX), AX
	MOVL	AX, 12(SP)		// event
	CALL	libc_ionotify(SB)
	MOVL	$0, BX
	CMPL	AX, $-1
	JNE	ionotifyok
	ERRNO
	MOVL	AX, BX
	MOVL	$-1, AX
ionotifyok:
	MOVL	8(BP), DX
	MOVL	AX, 16(DX)		// ret
	MOVL	BX, 20(DX)		// errno
	LEAVE
