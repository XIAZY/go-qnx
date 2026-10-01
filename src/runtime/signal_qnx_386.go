// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

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
