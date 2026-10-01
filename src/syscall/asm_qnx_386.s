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
TEXT ·childcall(SB),NOSPLIT,$16-24
	MOVL	a1+4(FP), AX
	MOVL	AX, 0(SP)
	MOVL	a2+8(FP), AX
	MOVL	AX, 4(SP)
	MOVL	a3+12(FP), AX
	MOVL	AX, 8(SP)
	MOVL	fn+0(FP), AX
	CALL	AX
	MOVL	AX, r1+16(FP)
	MOVL	$0, err+20(FP)
	CMPL	AX, $-1
	JNE	ok
	CALL	libc___get_errno_ptr(SB)
	MOVL	(AX), AX
	MOVL	AX, err+20(FP)
ok:
	RET

TEXT ·libc_sigaction_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_sigaction(SB)

TEXT ·libc_pthread_sigmask_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_pthread_sigmask(SB)
