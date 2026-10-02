// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

// The loader (libc.so.3) enters here with the initial process stack
//	argc, argv[0..argc-1], 0, envp..., 0, auxv...
// and, in R0, a function for the program to register with atexit.
// Like QNX's crt1.o, hand all of that to _init_libc before anything
// else runs: libc is not usable until it has been called. _init_libc
// takes argc, argv, envp and auxv in R0-R3 and the atexit function as a
// fifth argument on the stack.
TEXT _rt0_arm_qnx(SB),NOSPLIT|NOFRAME,$0
	MOVW	R0, R7			// save the atexit function
	MOVW	(R13), R5		// argc
	ADD	$4, R13, R6		// argv = &stack[1]
	ADD	R5<<2, R6, R2		// &argv[argc]
	ADD	$4, R2			// envp = &argv[argc+1]
	MOVW	R2, R3			// walk envp to find auxv
auxv:
	MOVW.P	4(R3), R12
	CMP	$0, R12
	BNE	auxv			// R3 = auxv, just past envp's 0

	MOVW	R5, R0			// argc
	MOVW	R6, R1			// argv
					// R2 = envp, R3 = auxv
	// The loader enters with an 8-byte-aligned stack (the ARM ABI entry
	// condition that QNX's crt1.o also relies on; it adds no alignment of
	// its own). SUB $8 keeps it aligned for the call, with the fifth
	// argument, the atexit function, at 0(SP).
	SUB	$8, R13
	MOVW	R7, 0(R13)
	BL	libc__init_libc(SB)
	ADD	$8, R13

	MOVW	R5, R0			// argc
	MOVW	R6, R1			// argv
	B	runtime·rt0_go(SB)

// With external linking the program starts in QNX's crt1.o, which calls
// _init_libc and then main(argc, argv, envp); the common ARM main (in
// asm_arm.s) jumps to rt0_go. libc must be initialised exactly once, so
// nothing here calls _init_libc again.

// _rt0_arm_qnx_lib is the entry for -buildmode=c-shared.
TEXT _rt0_arm_qnx_lib(SB),NOSPLIT,$0
	B	_rt0_arm_lib(SB)
