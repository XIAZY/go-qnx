// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

// The loader (libc.so.3) enters here with the initial process stack
//	argc, argv[0..argc-1], 0, envp..., 0, auxv...
// and, in DX, a function for the program to register with atexit.
// Like QNX's crt1.o, hand all of that to _init_libc before anything
// else runs: libc is not usable until it has been called.
TEXT _rt0_386_qnx(SB),NOSPLIT|NOFRAME,$0
	MOVL	0(SP), SI		// argc
	LEAL	4(SP), CX		// argv
	LEAL	4(CX)(SI*4), DI		// envp = &argv[argc+1]
	MOVL	DI, AX
auxv:
	MOVL	0(AX), BX
	ADDL	$4, AX
	TESTL	BX, BX
	JNE	auxv			// AX = auxv, just past envp's 0

	MOVL	SP, BP			// _init_libc preserves BP
	SUBL	$20, SP
	ANDL	$~15, SP
	MOVL	SI, 0(SP)		// argc
	MOVL	CX, 4(SP)		// argv
	MOVL	DI, 8(SP)		// envp
	MOVL	AX, 12(SP)		// auxv
	MOVL	DX, 16(SP)		// exit function
	CALL	libc__init_libc(SB)
	MOVL	BP, SP
	JMP	_rt0_386(SB)

TEXT _rt0_386_qnx_lib(SB),NOSPLIT,$0
	JMP	_rt0_386_lib(SB)

// With external linking the program starts in QNX's crt1.o, which
// calls _init_libc and then main(argc, argv, envp). libc must be
// initialised exactly once, so main goes straight to rt0_go.
TEXT main(SB),NOSPLIT,$0
	// Remove the return address from the stack.
	// rt0_go doesn't expect it to be there.
	ADDL	$4, SP
	JMP	runtime·rt0_go(SB)

