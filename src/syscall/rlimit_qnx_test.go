// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall_test

import (
	"syscall"
	"testing"
)

// TestRlimitInfinity checks that RLIM_INFINITY is the value Getrlimit
// returns for a resource without a limit, and that it can be stored in
// an Rlimit. QNX's ((rlim_t)-3) is not the all-ones value other systems
// use, so a constant copied from them would compile and never match.
func TestRlimitInfinity(t *testing.T) {
	lim := syscall.Rlimit{Cur: syscall.RLIM_INFINITY, Max: syscall.RLIM_INFINITY}
	if lim.Cur != 1<<64-3 {
		t.Fatalf("RLIM_INFINITY = %#x, want %#x", lim.Cur, uint64(1<<64-3))
	}
	// A CPU-time limit is almost never set; skip if this environment has one.
	if err := syscall.Getrlimit(syscall.RLIMIT_CPU, &lim); err != nil {
		t.Fatalf("Getrlimit(RLIMIT_CPU): %v", err)
	}
	switch {
	case lim.Max == syscall.RLIM_INFINITY:
	case lim.Max >= 1<<63:
		// One of QNX's special values, but not the one we call infinity.
		t.Fatalf("Getrlimit(RLIMIT_CPU) hard limit = %#x, RLIM_INFINITY = %#x", lim.Max, uint64(syscall.RLIM_INFINITY))
	default:
		t.Skipf("RLIMIT_CPU is limited here (hard limit %d)", lim.Max)
	}
	if lim.Cur != syscall.RLIM_INFINITY && lim.Cur >= 1<<63 {
		t.Errorf("Getrlimit(RLIMIT_CPU) soft limit = %#x, RLIM_INFINITY = %#x", lim.Cur, uint64(syscall.RLIM_INFINITY))
	}
}
