// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "go_asm.h"
#include "go_tls.h"
#include "funcdata.h"
#include "textflag.h"

// QNX has no ELF thread-local storage and no user-readable CP15 thread
// register, so g cannot live in a TLS variable reached by MRC the way
// the other ARM systems do (see tls_arm.s). Instead g lives in R10 while
// Go code runs, and in the __reserved1 slot (offset 0x24) of the thread
// control block otherwise, exactly as on qnx/386. libc's __tls() returns
// the control block; it is pure and returns the same pointer whichever
// CPU runs the thread, so the signal trampoline can call it to recover g
// after libc has clobbered R10.

// save_g stores g into the thread control block, so that load_g can
// recover it after externally compiled (libc) code overwrites R10.
// __tls() follows the C ABI and so preserves R4-R11, g (R10) among them;
// it may clobber R0-R3 and R12. The callers (runtime.gogo, runtime.mcall)
// require that save_g leave every register but R0 and R11 unchanged, so
// R1-R3 and R12 are saved around the call. Returns with g in R0.
TEXT runtime·save_g(SB),NOSPLIT|NOFRAME,$0
	MOVM.DB.W	[R1-R3, R11, R12, R14], (R13)
	MOVW	R13, R11		// keep SP; goroutine stacks are only 4-aligned
	BIC	$0x7, R13		// the C ABI needs 8-byte alignment at the call
	BL	libc___tls(SB)		// R0 = thread control block; preserves R11
	MOVW	R11, R13		// restore SP
	MOVW	g, (0x24)(R0)		// control block __reserved1 = g
	MOVM.IA.W	(R13), [R1-R3, R11, R12, R14]
	MOVW	g, R0			// return g in R0
	RET

// load_g loads g from the thread control block, for use in the signal
// trampoline and after calls into libc that overwrote R10. It clobbers
// R0 and g, as its callers expect, and preserves the rest.
TEXT runtime·load_g(SB),NOSPLIT|NOFRAME,$0
	MOVM.DB.W	[R1-R3, R11, R12, R14], (R13)
	MOVW	R13, R11		// keep SP; the interrupted stack may be 4-aligned
	BIC	$0x7, R13		// the C ABI needs 8-byte alignment at the call
	BL	libc___tls(SB)		// R0 = thread control block; preserves R11
	MOVW	R11, R13		// restore SP
	MOVW	(0x24)(R0), g		// g = control block __reserved1
	MOVM.IA.W	(R13), [R1-R3, R11, R12, R14]
	RET

// _initcgo is called from rt0_go. For a pure-Go program _cgo_init is nil
// and it does nothing. The frame is 8 bytes (a dummy word plus the saved
// LR) so the stack is 8-byte aligned at the call into _cgo_init.
TEXT runtime·_initcgo(SB),NOSPLIT,$4
	MOVW	_cgo_init(SB), R4
	CMP	$0, R4
	B.EQ	nocgo
	MOVW	$0, R3			// arg 4: &tls_g, unused; g is in the TCB
	MOVW	$0, R2			// arg 3: TLS base, unused
	MOVW	$setg_gcc<>(SB), R1	// arg 2: setg
	MOVW	g, R0			// arg 1: g
	BL	(R4)			// will clobber R0-R3
nocgo:
	RET

// setg_gcc is called from gcc-compiled code to set g. It sets R10 and
// writes g through to the thread control block.
TEXT setg_gcc<>(SB),NOSPLIT,$0
	MOVW	R0, g
	B	runtime·save_g(SB)
