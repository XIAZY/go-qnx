// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// NanotimeCallsLibc reports whether time.Now reads a clock through libc's
// clock_gettime: when either system page clock was not found to match it
// (see qnxInitTSC).
func NanotimeCallsLibc() bool { return qnxQtime == 0 || !qnxQtimeWall }

// QNXQtimeOn reports whether nanotime reads the system page.
func QNXQtimeOn() bool { return qnxQtime != 0 }

// QNXWalltimeOn reports whether walltime reads the system page.
func QNXWalltimeOn() bool { return qnxQtimeWall }

// QNXCpupageOn reports whether save_g and load_g read the control block
// directly rather than calling libc's __tls.
func QNXCpupageOn() bool { return qnxCpupagePtr != 0 }

// ClockMonotonic is libc's clock_gettime(CLOCK_MONOTONIC), in ns.
func ClockMonotonic() int64 { return clockMonotonic() }

// LibcRealtime is libc's clock_gettime(CLOCK_REALTIME), in ns.
func LibcRealtime() int64 {
	sec, nsec := libcWalltime()
	return sec*1e9 + int64(nsec)
}

// Walltime is walltime, in ns.
func Walltime() int64 {
	sec, nsec := walltime()
	return sec*1e9 + int64(nsec)
}
