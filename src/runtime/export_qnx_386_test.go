// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// These exports cover the TSC-based nanotime, which is x86 only; the
// other QNX architectures read nanotime from CLOCK_MONOTONIC. See
// tsc_qnx_386.go.

func TSCToNs(d uint64, mult, shift uint32) uint64 { return tscToNs(d, mult, shift) }

func TSCParams(freq uint64) (mult, shift uint32, ok bool) { return tscParams(freq) }

func TSCRateOff(dn, dm int64) bool { return tscRateOff(dn, dm) }

// QNXTSCOn reports whether nanotime reads the TSC.
func QNXTSCOn() bool { return qnxTSC.state.Load() == tscOn }

// QNXTSCInjectRate multiplies the TSC path's rate by num/den, continuing
// from the current nanotime so that the change itself goes nowhere
// backward, and makes sysmon's check use windows of window ns from now.
func QNXTSCInjectRate(num, den uint32, window int64) {
	qnxTSC.state.Store(tscSwitching)
	snap := qnxTSC.baseNs + int64(tscToNs(uint64(cputicks())-qnxTSC.base, qnxTSC.mult, qnxTSC.shift))
	qnxTSC.base = uint64(cputicks())
	qnxTSC.baseNs = snap
	qnxTSC.mult = uint32(uint64(qnxTSC.mult) * uint64(num) / uint64(den))
	qnxTSC.minWindow = window
	qnxTSC.checkNs, qnxTSC.checkMono = snap, clockMonotonic()
	qnxTSC.strikes = 0
	qnxTSC.state.Store(tscOn)
}

// QNXCheckTSC runs sysmon's check of the TSC rate.
func QNXCheckTSC() { qnxCheckTSC() }
