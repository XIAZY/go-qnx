// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// floatSums adds k to the k'th of eight float64 accumulators n times.
// The accumulators live in X registers for the whole loop, so a signal
// handler that returns with different X registers makes a sum wrong.
//
//go:noinline
func floatSums(n int) (s [8]float64) {
	var x0, x1, x2, x3, x4, x5, x6, x7 float64
	for i := 0; i < n; i++ {
		x0 += 0
		x1 += 1
		x2 += 2
		x3 += 3
		x4 += 4
		x5 += 5
		x6 += 6
		x7 += 7
	}
	return [8]float64{x0, x1, x2, x3, x4, x5, x6, x7}
}

// TestSignalFloatRegisters checks that the X registers of code
// interrupted by a signal are intact when the handler returns. QNX's
// kernel may not restore them after a handler, and Go's handler uses them
// (memmove, float conversions), so sigtramp saves and restores them
// itself. Preemption signals come from a goroutine that never yields and
// from runtime.GC in a loop.
func TestSignalFloatRegisters(t *testing.T) {
	d := 10 * time.Second
	if testing.Short() {
		d = time.Second
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(2))
	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for !stop.Load() {
		}
	}()
	go func() {
		defer wg.Done()
		for !stop.Load() {
			runtime.GC()
		}
	}()
	defer func() {
		stop.Store(true)
		wg.Wait()
	}()

	const n = 1 << 20
	for deadline := time.Now().Add(d); time.Now().Before(deadline); {
		s := floatSums(n)
		for k, v := range s {
			if want := float64(k * n); v != want {
				t.Fatalf("sum %d = %v, want %v", k, v, want)
			}
		}
	}
}
