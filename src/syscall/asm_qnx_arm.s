// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

// func childcall(fn, a1, a2, a3 uintptr) (r1 uintptr, err Errno)
//
// childcall calls the libc function fn on the current stack. It is only
// for the vfork child (see exec_qnx.go), which runs on a stack of its own
// and must not switch to g0 the way libcCall does. err is errno when fn
// returned -1.
TEXT ·childcall(SB),NOSPLIT,$8-24
	// 0(R13) is the saved return address; the $8 frame's locals are at
	// 4(R13) and 8(R13).
	MOVW	R4, 4(R13)		// preserve the caller's R4 (frame still addressable)
	MOVW	a1+4(FP), R0
	MOVW	a2+8(FP), R1
	MOVW	a3+12(FP), R2
	MOVW	fn+0(FP), R3
	MOVW	R13, R4			// R4 = frame SP
	BIC	$0x7, R13		// the C ABI needs 8-byte alignment at the call
	BL	(R3)
	MOVW	$0, R1			// err = 0
	CMP	$-1, R0
	BNE	ok
	MOVW	R0, 8(R4)		// stash r1 (-1) across the errno call (local, not the LR)
	BL	libc___get_errno_ptr(SB)
	MOVW	(R0), R1		// err = errno
	MOVW	8(R4), R0		// r1
ok:
	MOVW	R4, R13			// restore SP, then write the results and R4
	MOVW	R0, r1+16(FP)
	MOVW	R1, err+20(FP)
	MOVW	4(R13), R4
	RET

TEXT ·libc_sigaction_trampoline(SB),NOSPLIT,$0-0
	B	libc_sigaction(SB)

TEXT ·libc_pthread_sigmask_trampoline(SB),NOSPLIT,$0-0
	B	libc_pthread_sigmask(SB)
