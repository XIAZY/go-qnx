// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"runtime"
	"testing"
	"time"
)

// TestQNXQtime checks nanotime and walltime, when they read the system
// page, against libc's clocks: they must agree to within 2 ms and never
// go backward.
func TestQNXQtime(t *testing.T) {
	nsOn, wallOn := runtime.QNXQtimeOn(), runtime.QNXWalltimeOn()
	t.Logf("nanotime from system page: %v, walltime from system page: %v", nsOn, wallOn)
	if !nsOn {
		t.Skip("nanotime reads the system page: no (qnxInitTSC found no matching qtime entry)")
	}
	const tol = 2e6
	abs := func(x int64) int64 {
		if x < 0 {
			return -x
		}
		return x
	}

	t.Run("compare", func(t *testing.T) {
		last := runtime.Nanotime()
		lastWall := runtime.Walltime()
		for i := 0; i < 1000; i++ {
			time.Sleep(2 * time.Millisecond)
			n := runtime.Nanotime()
			m := runtime.ClockMonotonic()
			if n < last {
				t.Fatalf("sample %d: nanotime went back from %d to %d", i, last, n)
			}
			last = n
			if d := m - n; abs(d) > tol {
				t.Fatalf("sample %d: nanotime %d, CLOCK_MONOTONIC %d: differ by %d ns", i, n, m, d)
			}
			if wallOn {
				w := runtime.Walltime()
				r := runtime.LibcRealtime()
				if w < lastWall {
					t.Fatalf("sample %d: walltime went back from %d to %d", i, lastWall, w)
				}
				lastWall = w
				if d := r - w; abs(d) > tol {
					t.Fatalf("sample %d: walltime %d, CLOCK_REALTIME %d: differ by %d ns", i, w, r, d)
				}
				if d := time.Now().UnixNano() - r; abs(d) > tol {
					t.Fatalf("sample %d: time.Now %d ns from CLOCK_REALTIME", i, d)
				}
			}
		}
	})

	t.Run("tight", func(t *testing.T) {
		last := runtime.Nanotime()
		for i := 0; i < 1e6; i++ {
			n := runtime.Nanotime()
			if n < last {
				t.Fatalf("read %d: nanotime went back from %d to %d", i, last, n)
			}
			last = n
		}
	})
}
