// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
	"unsafe"
)

type sigctxt struct {
	info *siginfo
	ctxt unsafe.Pointer
}

//go:nosplit
//go:nowritebarrierrec
func (c *sigctxt) regs() *regs386 {
	return &(*ucontext)(c.ctxt).uc_mcontext.cpu
}

func (c *sigctxt) eax() uint32 { return c.regs().eax }
func (c *sigctxt) ebx() uint32 { return c.regs().ebx }
func (c *sigctxt) ecx() uint32 { return c.regs().ecx }
func (c *sigctxt) edx() uint32 { return c.regs().edx }
func (c *sigctxt) edi() uint32 { return c.regs().edi }
func (c *sigctxt) esi() uint32 { return c.regs().esi }
func (c *sigctxt) ebp() uint32 { return c.regs().ebp }
func (c *sigctxt) esp() uint32 { return c.regs().esp }

//go:nosplit
//go:nowritebarrierrec
func (c *sigctxt) eip() uint32 { return c.regs().eip }

func (c *sigctxt) eflags() uint32 { return c.regs().efl }
func (c *sigctxt) cs() uint32     { return c.regs().cs }

// QNX's context has no segment registers unless the headers are
// compiled with __SEGMENTS__, which libc is not.
func (c *sigctxt) fs() uint32 { return 0 }
func (c *sigctxt) gs() uint32 { return 0 }

func (c *sigctxt) sigcode() uint32 { return uint32(c.info.si_code) }
func (c *sigctxt) sigaddr() uint32 { return uint32(c.info.si_addr) }

func (c *sigctxt) set_eip(x uint32)     { c.regs().eip = x }
func (c *sigctxt) set_esp(x uint32)     { c.regs().esp = x }
func (c *sigctxt) set_sigcode(x uint32) { c.info.si_code = int32(x) }
func (c *sigctxt) set_sigaddr(x uint32) { c.info.si_addr = uintptr(x) }

// sigDelayedFault reports whether a synchronous fault signal reached
// sigtramp before the handler it interrupted had started, so that it
// must be let go.
//
// When a thread faults while an asynchronous signal (a preemption SIGURG,
// say) is pending for it, QNX 6.5 delivers the asynchronous signal first:
// it builds that signal's frame on the thread's stack, with the faulting
// instruction as the interrupted pc, and enters libc's __signalstub. If the
// fault signal is then blocked by that handler's mask, QNX kills the
// process; so setsig leaves the synchronous signals unblocked, and QNX
// delivers the fault at once, nested, with __signalstub's first instruction
// as its pc and the outer frame at its sp. That fault is not real: once the
// outer handler returns, the faulting instruction runs again and faults
// again, and that fault is handled normally. So it is ignored here.
//
// A fault signal can reach sigtramp at either end of __signalstub: at
// its first instruction, before the outer handler runs, with sp at the
// outer frame; or at its SignalReturn kernel call, after the outer
// handler has returned, with sp 0x14 bytes below the frame. The stub in
// QNX 6.5.0's libc.so.3:
//
//	+0x00  movl 0x2c(%esp), %eax    // entry: frame at sp, sigtramp at sp+0x28
//	       ...                      // save registers into the context
//	+0x1a  pushl %eax; pushl %esi; pushl (%esi)
//	+0x1e  call *0x28(%esi)         // sigtramp
//	+0x21  pushl %esi
//	       ...                      // reload registers from the context
//	+0x36  subl $4, %esp
//	+0x39  movl $0x1b, %eax         // __KER_SIGNAL_RETURN
//	+0x3e  int $0x28                // here: frame at sp+0x14, sigtramp at sp+0x3c
//
// Delayed faults were seen at the first point (TestTracebackInlined); the
// second has been seen with signals sent by kill. A delayed fault let go at
// the second point resumes the SignalReturn, which restores the outer
// frame's context, the faulting instruction.
//
// The outer frame is recognised by what QNX stores in it: the handler,
// sigtramp, at 0x28 from its start, and a pointer to its ucontext, which
// follows within the frame, at 0x2c. Only addresses inside the
// interrupted goroutine's stack are read. A real fault that looks like
// this, at every attempt, would loop; after 64 in a row it throws.
//
// Only faults (si_code > 0) are let go. A fault signal sent by kill or
// raise (si_code SI_USER) can arrive at the same points when the thread
// is inside the stub for another signal; it does not come again, so it
// is handled at once, with the outer frame's context as the interrupted
// one (TestSegv: letting it go lost it, and handling it in the stub's
// context gave "unknown pc"). Only the Go-side context pointer is
// switched, never the frames on the stack, so if the handler returns
// (a Notify'd SIGSEGV does), the kernel's SignalReturn for the nested
// delivery still restores the stub, which then runs the outer handler.
//
//go:nosplit
//go:nowritebarrierrec
func sigDelayedFault(sig uint32, c *sigctxt, gp *g) bool {
	switch sig {
	case _SIGSEGV, _SIGBUS, _SIGFPE, _SIGILL, _SIGTRAP:
	default:
		return false
	}
	if gp == nil || gp.m == nil {
		return false
	}
	mp := gp.m
	sp := uintptr(c.esp())
	frame := sp
	if !sigOuterFrame(frame, gp) {
		frame = sp + 0x14
		if !sigOuterFrame(frame, gp) {
			mp.delayedFaults = 0
			return false
		}
	}
	if int32(c.sigcode()) <= _SI_USER {
		// Not a fault: sent by kill or raise, it will not come again,
		// so it is handled now. Handle it as if it had interrupted the
		// code the outer frame interrupted, so that a traceback starts
		// there and not in __signalstub.
		mp.delayedFaults = 0
		// Stored as a uintptr: no write barrier in a signal handler,
		// and the ucontext is on a goroutine stack, not in the heap.
		*(*uintptr)(unsafe.Pointer(&c.ctxt)) = *(*uintptr)(unsafe.Pointer(frame + 0x2c))
		return false
	}
	mp.delayedFaults++
	if mp.delayedFaults > 64 {
		throw("runtime: synchronous fault repeats inside libc's signal stub")
	}
	return true
}

// sigOuterFrame reports whether a kernel signal frame for sigtramp starts
// at frame, inside gp's stack.
//
//go:nosplit
//go:nowritebarrierrec
func sigOuterFrame(frame uintptr, gp *g) bool {
	if frame < gp.stack.lo || frame+0x30 > gp.stack.hi {
		return false
	}
	if *(*uintptr)(unsafe.Pointer(frame + 0x28)) != abi.FuncPCABI0(sigtramp) {
		return false
	}
	ctx := *(*uintptr)(unsafe.Pointer(frame + 0x2c))
	return frame < ctx && ctx < frame+0x400
}
