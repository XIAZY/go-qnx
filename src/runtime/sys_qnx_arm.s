// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// System calls and other sys.stuff for ARM, QNX Neutrino 6.5.
// Everything goes through libc.so.3; this file holds the trampolines
// that convert from the Go to the C calling convention.
//
// A trampoline is called by libcCall or asmcgocall on the g0 stack with
// a pointer to its arguments in R0, and returns an int32 in R0. Each one
// keeps the argument pointer in R8 (a callee-saved register that libc
// preserves across the call), saves the stack pointer in R9, aligns the
// stack to 8 bytes before calling into libc as the C ABI requires, and
// reads errno right after a failing call.

#include "go_asm.h"
#include "go_tls.h"
#include "textflag.h"

// ERRNO leaves errno in R0. It clobbers R1-R3 and R12.
#define ERRNO \
	BL	libc___get_errno_ptr(SB); \
	MOVW	(R0), R0

// qnxtlsbad reports that the __reserved1 slot of a thread's control
// block, where g lives, was not zero before Go first used it, and exits.
TEXT runtime·qnxtlsbad(SB),NOSPLIT|NOFRAME,$0
	BIC	$0x7, R13
	MOVW	$2, R0			// stderr
	MOVW	$qnxtlsbadmsg<>(SB), R1
	MOVW	$59, R2			// len(qnxtlsbadmsg)
	BL	libc_write(SB)
	MOVW	$2, R0
	BL	libc__exit(SB)
	MOVW	$0, R1
	MOVW	R1, (R1)		// crash

DATA qnxtlsbadmsg<>+0x00(SB)/59, $"fatal error: QNX thread control block slot for g is in use\n"
GLOBL qnxtlsbadmsg<>(SB), RODATA, $59

// mstart_stub is the first function executed on a new thread started by
// pthread_create. It sets g and calls mstart.
// Note: called with the C calling convention, with the m in R0.
TEXT runtime·mstart_stub(SB),NOSPLIT,$0
	// We are already on m's g0 stack.

	// Save callee-save registers.
	MOVM.DB.W [R4-R11], (R13)

	MOVW	R0, R4			// m

	// Check that the control block's g slot is free.
	BL	libc___tls(SB)
	MOVW	(0x24)(R0), R1
	CMP	$0, R1
	BL.NE	runtime·qnxtlsbad(SB)

	MOVW	m_g0(R4), g
	BL	runtime·save_g(SB)

	BL	runtime·mstart(SB)

	// Restore callee-save registers.
	MOVM.IA.W (R13), [R4-R11]

	// Go is all done with this OS thread.
	// Tell pthread everything is ok (we never join with this thread, so
	// the value here doesn't really matter).
	MOVW	$0, R0
	RET

TEXT runtime·sigfwd(SB),NOSPLIT,$0-16
	MOVW	sig+4(FP), R0
	MOVW	info+8(FP), R1
	MOVW	ctx+12(FP), R2
	MOVW	fn+0(FP), R3
	MOVW	R13, R9
	SUB	$24, R13
	BIC	$0x7, R13		// align stack: handler might be a C function
	BL	(R3)
	MOVW	R9, R13
	RET

// sigtramp is the signal handler, called by libc with the C calling
// convention: sigtramp(signo, info, context).
//
// QNX has no alternate signal stacks: the kernel builds the signal frame
// on the interrupted stack, which may be a goroutine stack with only
// stackSystem bytes to spare. So sigtramp does as little as possible
// there and moves to the M's gsignal stack before calling Go, unless it
// is not on a Go thread or is already on gsignal. All signals are
// blocked while it runs (setsig's sa_mask), so frames cannot nest.
//
// It preserves errno, as a signal handler must, and saves the VFP
// registers d0-d15 and FPSCR (and d16-d31 when the CPU has them) before
// calling Go and restores them after: QNX 6.5 does not reliably restore
// them when a handler returns, and Go's handler code uses VFP registers.
// The save area is on the stack sigtramp calls Go on, never on a
// goroutine stack.
#define SIG_VFP		16		// d0-d15, 128 bytes
#define SIG_FPSCR	144		// fpscr, 4 bytes
#define SIG_VFPU	152		// d16-d31, 128 bytes (8-aligned)
#define SIG_SP		280		// interrupted SP, where the saved registers are
#define SIG_ERRP	288		// &errno
#define SIG_ERRV	292		// saved errno value
#define SIG_FRAME	304
TEXT runtime·sigtramp(SB),NOSPLIT|TOPFRAME,$0
	// libc calls us as a C function, so it keeps live state in the
	// callee-saved registers across the call. Save and restore them, as
	// well as g (R10); the $0-frame prologue has saved LR.
	MOVM.DB.W [R4-R11], (R13)
	MOVW	R13, R7			// interrupted SP (the saved registers live here)
	MOVW	R0, R4			// signo
	MOVW	R1, R5			// info
	MOVW	R2, R6			// context

	BL	runtime·load_g(SB)	// recover g from the control block

	// Pick the stack to run Go on: the gsignal stack, unless g is unset
	// or we are already on it.
	MOVW	R7, R8			// default: the interrupted stack
	CMP	$0, g
	BEQ	havestack
	MOVW	g_m(g), R1
	CMP	$0, R1
	BEQ	havestack
	MOVW	m_gsignal(R1), R2
	CMP	$0, R2
	BEQ	havestack
	MOVW	(g_stack+stack_lo)(R2), R3
	CMP	R3, R7
	B.LO	usegsig			// R7 < lo: not on gsignal
	MOVW	(g_stack+stack_hi)(R2), R3
	CMP	R3, R7
	B.LO	havestack		// lo <= R7 < hi: already on gsignal
usegsig:
	MOVW	(g_stack+stack_hi)(R2), R8
havestack:
	SUB	$SIG_FRAME, R8
	BIC	$0x7, R8
	MOVW	R8, R13			// switch to the chosen stack

	MOVW	R7, SIG_SP(R13)

	// Save errno.
	BL	libc___get_errno_ptr(SB)
	MOVW	R0, SIG_ERRP(R13)
	MOVW	(R0), R1
	MOVW	R1, SIG_ERRV(R13)

	// Save the VFP registers when the CPU has them (regardless of how
	// Go was built): we may be interrupting libc code that uses VFP.
	MOVB	runtime·qnxHasFPU(SB), R0
	CMP	$0, R0
	BEQ	nofpsave
	MOVD	F0, (SIG_VFP+0*8)(R13)
	MOVD	F1, (SIG_VFP+1*8)(R13)
	MOVD	F2, (SIG_VFP+2*8)(R13)
	MOVD	F3, (SIG_VFP+3*8)(R13)
	MOVD	F4, (SIG_VFP+4*8)(R13)
	MOVD	F5, (SIG_VFP+5*8)(R13)
	MOVD	F6, (SIG_VFP+6*8)(R13)
	MOVD	F7, (SIG_VFP+7*8)(R13)
	MOVD	F8, (SIG_VFP+8*8)(R13)
	MOVD	F9, (SIG_VFP+9*8)(R13)
	MOVD	F10, (SIG_VFP+10*8)(R13)
	MOVD	F11, (SIG_VFP+11*8)(R13)
	MOVD	F12, (SIG_VFP+12*8)(R13)
	MOVD	F13, (SIG_VFP+13*8)(R13)
	MOVD	F14, (SIG_VFP+14*8)(R13)
	MOVD	F15, (SIG_VFP+15*8)(R13)
	WORD	$0xeef1ba10		// vmrs r11, fpscr
	MOVW	R11, SIG_FPSCR(R13)
	MOVB	runtime·qnxVFPd32(SB), R0
	CMP	$0, R0
	BEQ	nofpsave
	ADD	$SIG_VFPU, R13, R0
	WORD	$0xece00b20		// vstmia r0!, {d16-d31}
nofpsave:
	// Call sigtrampgo(signo, info, context). The arguments sit at
	// 4(R13), with 0(R13) reserved for the call, per the ARM ABI.
	MOVW	R4, 4(R13)
	MOVW	R5, 8(R13)
	MOVW	R6, 12(R13)
	BL	runtime·sigtrampgo(SB)

	// Restore the VFP registers.
	MOVB	runtime·qnxHasFPU(SB), R0
	CMP	$0, R0
	BEQ	nofprestore
	MOVD	(SIG_VFP+0*8)(R13), F0
	MOVD	(SIG_VFP+1*8)(R13), F1
	MOVD	(SIG_VFP+2*8)(R13), F2
	MOVD	(SIG_VFP+3*8)(R13), F3
	MOVD	(SIG_VFP+4*8)(R13), F4
	MOVD	(SIG_VFP+5*8)(R13), F5
	MOVD	(SIG_VFP+6*8)(R13), F6
	MOVD	(SIG_VFP+7*8)(R13), F7
	MOVD	(SIG_VFP+8*8)(R13), F8
	MOVD	(SIG_VFP+9*8)(R13), F9
	MOVD	(SIG_VFP+10*8)(R13), F10
	MOVD	(SIG_VFP+11*8)(R13), F11
	MOVD	(SIG_VFP+12*8)(R13), F12
	MOVD	(SIG_VFP+13*8)(R13), F13
	MOVD	(SIG_VFP+14*8)(R13), F14
	MOVD	(SIG_VFP+15*8)(R13), F15
	MOVW	SIG_FPSCR(R13), R11
	WORD	$0xeee1ba10		// vmsr fpscr, r11
	MOVB	runtime·qnxVFPd32(SB), R0
	CMP	$0, R0
	BEQ	nofprestore
	ADD	$SIG_VFPU, R13, R0
	WORD	$0xecf00b20		// vldmia r0!, {d16-d31}
nofprestore:
	// Restore errno.
	MOVW	SIG_ERRP(R13), R0
	MOVW	SIG_ERRV(R13), R1
	MOVW	R1, (R0)

	// Back to the interrupted stack, restore the saved registers, and
	// return (the prologue restores LR).
	MOVW	SIG_SP(R13), R13
	MOVM.IA.W (R13), [R4-R11]
	RET

TEXT runtime·pthread_attr_init_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// attr
	BL	libc_pthread_attr_init(SB)
	MOVW	R9, R13
	RET

TEXT runtime·pthread_attr_destroy_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// attr
	BL	libc_pthread_attr_destroy(SB)
	MOVW	R9, R13
	RET

TEXT runtime·pthread_attr_getstacksize_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// size
	MOVW	0(R0), R0		// attr
	BL	libc_pthread_attr_getstacksize(SB)
	MOVW	R9, R13
	RET

TEXT runtime·pthread_attr_setstacksize_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// size
	MOVW	0(R0), R0		// attr
	BL	libc_pthread_attr_setstacksize(SB)
	MOVW	R9, R13
	RET

TEXT runtime·pthread_attr_setdetachstate_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// state
	MOVW	0(R0), R0		// attr
	BL	libc_pthread_attr_setdetachstate(SB)
	MOVW	R9, R13
	RET

TEXT runtime·pthread_create_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	SUB	$16, R13
	BIC	$0x7, R13
	MOVW	0(R0), R1		// attr
	MOVW	4(R0), R2		// start
	MOVW	8(R0), R3		// arg
	MOVW	R13, R0			// &thread ID (discarded)
	BL	libc_pthread_create(SB)
	MOVW	R9, R13
	RET

TEXT runtime·pthread_self_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	BL	libc_pthread_self(SB)
	MOVW	R0, 0(R8)		// result
	MOVW	R9, R13
	RET

TEXT runtime·pthread_kill_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// sig
	MOVW	0(R0), R0		// thread
	BL	libc_pthread_kill(SB)
	MOVW	R9, R13
	RET

// The sem_* trampolines return 0 or errno.
TEXT runtime·sem_init_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// pshared
	MOVW	8(R0), R2		// value
	MOVW	0(R0), R0		// sem
	BL	libc_sem_init(SB)
	CMP	$0, R0
	BEQ	seminitok
	ERRNO
seminitok:
	MOVW	R9, R13
	RET

TEXT runtime·sem_wait_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// sem
	BL	libc_sem_wait(SB)
	CMP	$0, R0
	BEQ	semwaitok
	ERRNO
semwaitok:
	MOVW	R9, R13
	RET

TEXT runtime·sem_timedwait_monotonic_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// abstime
	MOVW	0(R0), R0		// sem
	BL	libc_sem_timedwait_monotonic(SB)
	CMP	$0, R0
	BEQ	semtwok
	ERRNO
semtwok:
	MOVW	R9, R13
	RET

TEXT runtime·sem_post_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// sem
	BL	libc_sem_post(SB)
	CMP	$0, R0
	BEQ	sempostok
	ERRNO
sempostok:
	MOVW	R9, R13
	RET

TEXT runtime·sem_destroy_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// sem
	BL	libc_sem_destroy(SB)
	CMP	$0, R0
	BEQ	semdestok
	ERRNO
semdestok:
	MOVW	R9, R13
	RET

TEXT runtime·sched_yield_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	BL	libc_sched_yield(SB)
	MOVW	R9, R13
	RET

TEXT runtime·exit_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// status
	BL	libc_exit(SB)
	MOVW	$0, R1
	MOVW	R1, (R1)		// crash
	MOVW	R9, R13
	RET

TEXT runtime·_exit_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// status
	BL	libc__exit(SB)
	MOVW	$0, R1
	MOVW	R1, (R1)		// crash
	MOVW	R9, R13
	RET

TEXT runtime·getpid_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	BL	libc_getpid(SB)
	MOVW	R0, 0(R8)		// result
	MOVW	R9, R13
	RET

TEXT runtime·raiseproc_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	BL	libc_getpid(SB)		// pid
	MOVW	0(R8), R1		// sig
	BL	libc_kill(SB)
	MOVW	R9, R13
	RET

TEXT runtime·getuid_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	BL	libc_getuid(SB)
	MOVW	R0, 0(R8)
	MOVW	R9, R13
	RET

TEXT runtime·geteuid_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	BL	libc_geteuid(SB)
	MOVW	R0, 0(R8)
	MOVW	R9, R13
	RET

TEXT runtime·getgid_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	BL	libc_getgid(SB)
	MOVW	R0, 0(R8)
	MOVW	R9, R13
	RET

TEXT runtime·getegid_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	BL	libc_getegid(SB)
	MOVW	R0, 0(R8)
	MOVW	R9, R13
	RET

// mmap's off_t is 32 bits here: the plain mmap symbol, not mmap64.
TEXT runtime·mmap_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	SUB	$16, R13
	BIC	$0x7, R13
	MOVW	16(R8), R4		// fd (on stack)
	MOVW	R4, 0(R13)
	MOVW	20(R8), R4		// offset (on stack)
	MOVW	R4, 4(R13)
	MOVW	4(R8), R1		// len
	MOVW	8(R8), R2		// prot
	MOVW	12(R8), R3		// flags
	MOVW	0(R8), R0		// addr
	BL	libc_mmap(SB)
	MOVW	$0, R1
	CMP	$-1, R0			// MAP_FAILED
	BNE	ok
	ERRNO
	MOVW	R0, R1			// errno
	MOVW	$0, R0
ok:
	MOVW	R0, 24(R8)		// ret1
	MOVW	R1, 28(R8)		// ret2 (errno)
	MOVW	R9, R13
	RET

TEXT runtime·munmap_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// len
	MOVW	0(R0), R0		// addr
	BL	libc_munmap(SB)
	CMP	$-1, R0			// return errno, or 0
	BNE	munmapok
	ERRNO
munmapok:
	MOVW	R9, R13
	RET

// posix_madvise returns an error number, not -1 and errno.
TEXT runtime·posix_madvise_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// len
	MOVW	8(R0), R2		// advice
	MOVW	0(R0), R0		// addr
	BL	libc_posix_madvise(SB)
	MOVW	R9, R13
	RET

TEXT runtime·open_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	SUB	$8, R13
	BIC	$0x7, R13
	MOVW	8(R0), R2		// mode (vararg, on stack)
	MOVW	R2, 0(R13)
	MOVW	4(R0), R1		// flags
	MOVW	0(R0), R0		// path
	BL	libc_open(SB)
	CMP	$-1, R0			// return the descriptor, or -errno
	BNE	openok
	ERRNO
	RSB	$0, R0, R0
openok:
	MOVW	R9, R13
	RET

TEXT runtime·close_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// fd
	BL	libc_close(SB)
	MOVW	R9, R13
	RET

// read and write return the count, or -errno.
TEXT runtime·read_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// buf
	MOVW	8(R0), R2		// count
	MOVW	0(R0), R0		// fd
	BL	libc_read(SB)
	CMP	$-1, R0
	BNE	rdok
	ERRNO
	RSB	$0, R0, R0
rdok:
	MOVW	R9, R13
	RET

TEXT runtime·write_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// buf
	MOVW	8(R0), R2		// count
	MOVW	0(R0), R0		// fd
	BL	libc_write(SB)
	CMP	$-1, R0
	BNE	wrok
	ERRNO
	RSB	$0, R0, R0
wrok:
	MOVW	R9, R13
	RET

// pipe returns 0 or errno.
TEXT runtime·pipe_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
				// R0 already points at the fds array
	BL	libc_pipe(SB)
	CMP	$0, R0
	BEQ	pipeok
	ERRNO
pipeok:
	MOVW	R9, R13
	RET

TEXT runtime·setitimer_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	MOVW	4(R8), R1		// new
	MOVW	8(R8), R2		// old
	MOVW	0(R8), R0		// which
	BL	libc_setitimer(SB)
	CMP	$0, R0
	BEQ	setitok
	ERRNO
	MOVW	R0, 12(R8)		// errno
setitok:
	MOVW	R9, R13
	RET

// clockId and clockTime read a thread's CPU-time clock for the profiler;
// the _r forms return -errno rather than setting errno.
TEXT runtime·clockId_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// tid
	MOVW	0(R0), R0		// pid
	BL	libc_ClockId_r(SB)
	MOVW	R9, R13
	RET

TEXT runtime·clockTime_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// new
	MOVW	8(R0), R2		// old
	MOVW	0(R0), R0		// id
	BL	libc_ClockTime_r(SB)
	MOVW	R9, R13
	RET

// clockCycles_trampoline stores the 64-bit result of ClockCycles, which
// libc returns in R0 (low) and R1 (high), at the pointer in R0. R4 is
// callee-saved in the C ABI, so it keeps the pointer across the call.
TEXT runtime·clockCycles_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	R0, R4
	BL	libc_ClockCycles(SB)
	MOVW	R0, 0(R4)
	MOVW	R1, 4(R4)
	MOVW	R9, R13
	RET

TEXT runtime·usleep_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// usec
	BL	libc_usleep(SB)
	MOVW	R9, R13
	RET

TEXT runtime·sysconf_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// name
	BL	libc_sysconf(SB)
	MOVW	R9, R13
	RET

TEXT runtime·fcntl_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	MOVW	4(R8), R1		// cmd
	MOVW	8(R8), R2		// arg
	MOVW	0(R8), R0		// fd
	BL	libc_fcntl(SB)
	MOVW	$0, R1
	CMP	$-1, R0
	BNE	fcntlok
	ERRNO
	MOVW	R0, R1			// errno
	MOVW	$-1, R0
fcntlok:
	MOVW	R0, 12(R8)		// ret
	MOVW	R1, 16(R8)		// errno
	MOVW	R9, R13
	RET

// clock_gettime returns 0 or -errno.
TEXT runtime·clock_gettime_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// tp
	MOVW	0(R0), R0		// clock_id
	BL	libc_clock_gettime(SB)
	CMP	$-1, R0
	BNE	clockok
	ERRNO
	RSB	$0, R0, R0
clockok:
	MOVW	R9, R13
	RET

TEXT runtime·sigaction_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// new
	MOVW	8(R0), R2		// old
	MOVW	0(R0), R0		// sig
	BL	libc_sigaction(SB)
	CMP	$-1, R0
	BNE	2(PC)
	MOVW	R0, (R0)		// crash (R0 == -1)
	MOVW	R9, R13
	RET

// sigprocmask is per thread: pthread_sigmask. It returns an error number,
// and can only fail on bad arguments.
TEXT runtime·sigprocmask_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// new
	MOVW	8(R0), R2		// old
	MOVW	0(R0), R0		// how
	BL	libc_pthread_sigmask(SB)
	CMP	$0, R0
	BEQ	sigpmok
	MOVW	$0, R1			// crash on failure
	MOVW	R1, (R1)
sigpmok:
	MOVW	R9, R13
	RET

// vforkexec_trampoline: see syscall_rawVforkExec. Its arguments are
// struct { child, arg, stk uintptr; pid int; err uintptr }.
// R4-R11 are callee-saved in the C ABI, so vfork preserves them in both
// processes; the child uses only them until it is on its own stack.
TEXT runtime·vforkexec_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8			// args
	BIC	$0x7, R13
	MOVW	0(R8), R4		// child function
	MOVW	4(R8), R5		// its argument
	MOVW	8(R8), R6		// top of the child's stack
	BL	libc_vfork(SB)
	CMP	$0, R0
	BNE	parent
	// Child. From here on nothing may be written to the parent's stack.
	// The child function is Go code (qnxChildExec), so its argument goes
	// on the stack at 4(SP), with 0(SP) the link slot, per the ARM ABI.
	MOVW	R6, R13			// the child's own stack
	BIC	$0x7, R13
	SUB	$8, R13
	MOVW	R5, 4(R13)		// arg
	BL	(R4)			// does not return
	MOVW	$0, R1
	MOVW	R1, (R1)
parent:
	MOVW	$0, R1
	CMP	$-1, R0
	BNE	vfok
	ERRNO
	MOVW	R0, R1			// errno
	MOVW	$0, R0
vfok:
	MOVW	R0, 12(R8)		// pid
	MOVW	R1, 16(R8)		// err
	MOVW	R9, R13
	RET

// syscall calls a libc function on behalf of the syscall package.
// struct { fn, a1, a2, a3, r1, r2, err uintptr }. 32-bit result; AX==-1
// means failure.
TEXT runtime·syscall(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	MOVW	4(R8), R0		// a1
	MOVW	8(R8), R1		// a2
	MOVW	12(R8), R2		// a3
	MOVW	0(R8), R3		// fn
	BL	(R3)
	MOVW	R0, 16(R8)		// r1
	MOVW	R1, 20(R8)		// r2
	CMP	$-1, R0
	BNE	scok
	ERRNO
	MOVW	R0, 24(R8)		// err
scok:
	MOVW	$0, R0
	MOVW	R9, R13
	RET

// syscallX is like syscall but expects a 64-bit result in R1:R0 and
// tests both halves for -1.
TEXT runtime·syscallX(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	MOVW	4(R8), R0		// a1
	MOVW	8(R8), R1		// a2
	MOVW	12(R8), R2		// a3
	MOVW	0(R8), R3		// fn
	BL	(R3)
	MOVW	R0, 16(R8)		// r1
	MOVW	R1, 20(R8)		// r2
	CMP	$-1, R0
	BNE	scXok
	CMP	$-1, R1
	BNE	scXok
	ERRNO
	MOVW	R0, 24(R8)		// err
scXok:
	MOVW	$0, R0
	MOVW	R9, R13
	RET

// syscallPtr is like syscall but treats a NULL result as failure.
TEXT runtime·syscallPtr(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	MOVW	4(R8), R0		// a1
	MOVW	8(R8), R1		// a2
	MOVW	12(R8), R2		// a3
	MOVW	0(R8), R3		// fn
	BL	(R3)
	MOVW	R0, 16(R8)		// r1
	MOVW	R1, 20(R8)		// r2
	CMP	$0, R0
	BNE	scPok
	ERRNO
	MOVW	R0, 24(R8)		// err
scPok:
	MOVW	$0, R0
	MOVW	R9, R13
	RET

// syscall6: struct { fn, a1..a6, r1, r2, err uintptr }.
TEXT runtime·syscall6(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	SUB	$16, R13
	BIC	$0x7, R13
	MOVW	20(R8), R4		// a5
	MOVW	R4, 0(R13)
	MOVW	24(R8), R4		// a6
	MOVW	R4, 4(R13)
	MOVW	4(R8), R0		// a1
	MOVW	8(R8), R1		// a2
	MOVW	12(R8), R2		// a3
	MOVW	16(R8), R3		// a4
	MOVW	0(R8), R12		// fn
	BL	(R12)
	MOVW	R0, 28(R8)		// r1
	MOVW	R1, 32(R8)		// r2
	CMP	$-1, R0
	BNE	sc6ok
	ERRNO
	MOVW	R0, 36(R8)		// err
sc6ok:
	MOVW	$0, R0
	MOVW	R9, R13
	RET

TEXT runtime·syscall6X(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	SUB	$16, R13
	BIC	$0x7, R13
	MOVW	20(R8), R4		// a5
	MOVW	R4, 0(R13)
	MOVW	24(R8), R4		// a6
	MOVW	R4, 4(R13)
	MOVW	4(R8), R0		// a1
	MOVW	8(R8), R1		// a2
	MOVW	12(R8), R2		// a3
	MOVW	16(R8), R3		// a4
	MOVW	0(R8), R12		// fn
	BL	(R12)
	MOVW	R0, 28(R8)		// r1
	MOVW	R1, 32(R8)		// r2
	CMP	$-1, R0
	BNE	sc6Xok
	CMP	$-1, R1
	BNE	sc6Xok
	ERRNO
	MOVW	R0, 36(R8)		// err
sc6Xok:
	MOVW	$0, R0
	MOVW	R9, R13
	RET

// syscall10: struct { fn, a1..a10, r1, r2, err uintptr }.
TEXT runtime·syscall10(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	SUB	$32, R13
	BIC	$0x7, R13
	MOVW	20(R8), R4		// a5
	MOVW	R4, 0(R13)
	MOVW	24(R8), R4		// a6
	MOVW	R4, 4(R13)
	MOVW	28(R8), R4		// a7
	MOVW	R4, 8(R13)
	MOVW	32(R8), R4		// a8
	MOVW	R4, 12(R13)
	MOVW	36(R8), R4		// a9
	MOVW	R4, 16(R13)
	MOVW	40(R8), R4		// a10
	MOVW	R4, 20(R13)
	MOVW	4(R8), R0		// a1
	MOVW	8(R8), R1		// a2
	MOVW	12(R8), R2		// a3
	MOVW	16(R8), R3		// a4
	MOVW	0(R8), R12		// fn
	BL	(R12)
	MOVW	R0, 44(R8)		// r1
	MOVW	R1, 48(R8)		// r2
	CMP	$-1, R0
	BNE	sc10ok
	ERRNO
	MOVW	R0, 52(R8)		// err
sc10ok:
	MOVW	$0, R0
	MOVW	R9, R13
	RET

TEXT runtime·syscall10X(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	SUB	$32, R13
	BIC	$0x7, R13
	MOVW	20(R8), R4		// a5
	MOVW	R4, 0(R13)
	MOVW	24(R8), R4		// a6
	MOVW	R4, 4(R13)
	MOVW	28(R8), R4		// a7
	MOVW	R4, 8(R13)
	MOVW	32(R8), R4		// a8
	MOVW	R4, 12(R13)
	MOVW	36(R8), R4		// a9
	MOVW	R4, 16(R13)
	MOVW	40(R8), R4		// a10
	MOVW	R4, 20(R13)
	MOVW	4(R8), R0		// a1
	MOVW	8(R8), R1		// a2
	MOVW	12(R8), R2		// a3
	MOVW	16(R8), R3		// a4
	MOVW	0(R8), R12		// fn
	BL	(R12)
	MOVW	R0, 44(R8)		// r1
	MOVW	R1, 48(R8)		// r2
	CMP	$-1, R0
	BNE	sc10Xok
	CMP	$-1, R1
	BNE	sc10Xok
	ERRNO
	MOVW	R0, 52(R8)		// err
sc10Xok:
	MOVW	$0, R0
	MOVW	R9, R13
	RET

// dlsym returns the address, or 0.
TEXT runtime·dlsym_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// name
	MOVW	0(R0), R0		// handle
	BL	libc_dlsym(SB)
	MOVW	R9, R13
	RET

// Kernel calls for the network poller (netpoll_qnx.go). The _r forms
// return -errno instead of setting errno.

TEXT runtime·channelCreate_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	0(R0), R0		// flags
	BL	libc_ChannelCreate_r(SB)
	MOVW	R9, R13
	RET

TEXT runtime·connectAttach_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	SUB	$8, R13
	BIC	$0x7, R13
	MOVW	16(R0), R4		// flags (on stack)
	MOVW	R4, 0(R13)
	MOVW	4(R0), R1		// pid
	MOVW	8(R0), R2		// chid
	MOVW	12(R0), R3		// index
	MOVW	0(R0), R0		// nd
	BL	libc_ConnectAttach_r(SB)
	MOVW	R9, R13
	RET

TEXT runtime·msgReceivePulse_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// pulse
	MOVW	8(R0), R2		// bytes
	MOVW	12(R0), R3		// info
	MOVW	0(R0), R0		// chid
	BL	libc_MsgReceivePulse_r(SB)
	MOVW	R9, R13
	RET

TEXT runtime·msgSendPulse_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	BIC	$0x7, R13
	MOVW	4(R0), R1		// priority
	MOVW	8(R0), R2		// code
	MOVW	12(R0), R3		// value
	MOVW	0(R0), R0		// coid
	BL	libc_MsgSendPulse_r(SB)
	MOVW	R9, R13
	RET

TEXT runtime·timerTimeout_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	SUB	$8, R13
	BIC	$0x7, R13
	MOVW	16(R0), R4		// otime (on stack)
	MOVW	R4, 0(R13)
	MOVW	4(R0), R1		// flags
	MOVW	8(R0), R2		// notify
	MOVW	12(R0), R3		// ntime
	MOVW	0(R0), R0		// id
	BL	libc_TimerTimeout_r(SB)
	MOVW	R9, R13
	RET

TEXT runtime·ionotify_trampoline(SB),NOSPLIT,$0
	MOVW	R13, R9
	MOVW	R0, R8
	BIC	$0x7, R13
	MOVW	4(R8), R1		// action
	MOVW	8(R8), R2		// flags
	MOVW	12(R8), R3		// event
	MOVW	0(R8), R0		// fd
	BL	libc_ionotify(SB)
	MOVW	$0, R1
	CMP	$-1, R0
	BNE	ionok
	ERRNO
	MOVW	R0, R1			// errno
	MOVW	$-1, R0
ionok:
	MOVW	R0, 16(R8)		// ret
	MOVW	R1, 20(R8)		// errno
	MOVW	R9, R13
	RET

// publicationBarrier is a store/store barrier (ARMv7+ DMB ST via
// armPublicationBarrier). On ARM it is provided per-OS; see asm_arm.s.
TEXT ·publicationBarrier(SB),NOSPLIT|NOFRAME,$0-0
	B	runtime·armPublicationBarrier(SB)
