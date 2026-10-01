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

//go:noinline
func nilStore(p *int) { *p = 3 }

// faultAndRecover stores through a nil pointer and recovers from the
// panic.
func faultAndRecover() (ok bool) {
	defer func() { ok = recover() != nil }()
	nilStore(nil)
	return false
}

// TestFaultDuringPreemption faults on purpose, over and over, while
// preemption signals keep arriving. On QNX 6.5 a fault that arrives
// with a SIGURG pending is delivered after the SIGURG handler has been
// entered, and used to kill the process (see sigDelayedFault).
func TestFaultDuringPreemption(t *testing.T) {
	d := 10 * time.Second
	if testing.Short() {
		d = 2 * time.Second
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
	n := 0
	for deadline := time.Now().Add(d); time.Now().Before(deadline); n++ {
		if !faultAndRecover() {
			t.Fatal("nil store did not panic")
		}
	}
	t.Logf("%d faults recovered", n)
}
