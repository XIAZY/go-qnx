// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"internal/testenv"
	mbig "math/big"
	"math/rand/v2"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestQNXTSCToNs checks the multiply-and-shift conversion against exact
// arithmetic, over the whole range of TSC deltas.
func TestQNXTSCToNs(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, freq := range []uint64{1e6, 3579545, 1e9, 1193182000, 2594024200, 2623481096, 4e9, 1e10} {
		mult, shift, ok := runtime.TSCParams(freq)
		if !ok {
			t.Fatalf("TSCParams(%d) not ok", freq)
		}
		if shift > 32 {
			t.Fatalf("TSCParams(%d): shift %d > 32", freq, shift)
		}
		// One second of cycles converts to one second, to within the
		// rounding of mult: one part in mult.
		if ns := runtime.TSCToNs(freq, mult, shift); ns > 1e9 || 1e9-ns > 1e9/uint64(mult)+1 {
			t.Errorf("freq %d: %d cycles = %d ns, want 1e9", freq, freq, ns)
		}
		ds := []uint64{0, 1, 1<<32 - 1, 1 << 32, 1<<32 + 1, 1<<63 - 1, 1 << 63, 1<<64 - 1}
		for range 10000 {
			ds = append(ds, r.Uint64()>>r.UintN(64))
		}
		for _, d := range ds {
			want := new(mbig.Int).SetUint64(d)
			want.Mul(want, new(mbig.Int).SetUint64(uint64(mult)))
			want.Rsh(want, uint(shift))
			if !want.IsUint64() {
				continue // beyond 2^64 ns, about 584 years
			}
			if got := runtime.TSCToNs(d, mult, shift); got != want.Uint64() {
				t.Fatalf("freq %d: TSCToNs(%d, %d, %d) = %d, want %d", freq, d, mult, shift, got, want.Uint64())
			}
		}
	}
	if _, _, ok := runtime.TSCParams(0); ok {
		t.Errorf("TSCParams(0) ok")
	}
}

func TestQNXTSCRateOff(t *testing.T) {
	for _, tt := range []struct {
		dn, dm int64
		off    bool
	}{
		{10e9, 10e9, false},
		{10e9, 9.89e9, false}, // QNX's tick clock losing 1.1%
		{10.4e9, 10e9, false},
		{9.6e9, 10e9, false},
		{10.6e9, 10e9, true},
		{9.4e9, 10e9, true},
		{20e9, 10e9, true},
	} {
		if got := runtime.TSCRateOff(tt.dn, tt.dm); got != tt.off {
			t.Errorf("TSCRateOff(%d, %d) = %v, want %v", tt.dn, tt.dm, got, tt.off)
		}
	}
}

// checkNanotimeMonotonic reads nanotime from n threads at once, for d,
// and fails if any read is below a value another read had already
// returned.
func checkNanotimeMonotonic(t *testing.T, n int, d time.Duration, during func()) {
	var max atomic.Int64
	var backward atomic.Int64
	var reads atomic.Int64
	var stop atomic.Bool
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			var k int64
			for !stop.Load() {
				prev := max.Load()
				v := runtime.Nanotime()
				if v < prev {
					backward.Add(1)
					t.Errorf("nanotime %d after %d had been returned", v, prev)
					return
				}
				for {
					m := max.Load()
					if v <= m || max.CompareAndSwap(m, v) {
						break
					}
				}
				k++
			}
			reads.Add(k)
		}()
	}
	if during != nil {
		during()
	}
	time.Sleep(d)
	stop.Store(true)
	wg.Wait()
	t.Logf("%d reads on %d threads, %d backward", reads.Load(), n, backward.Load())
}

// TestQNXNanotimeMonotonic checks that nanotime never goes backward
// across threads, which run on different CPUs as the scheduler moves them.
func TestQNXNanotimeMonotonic(t *testing.T) {
	if !runtime.QNXTSCOn() {
		t.Log("nanotime is not using the TSC on this machine")
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	d := 5 * time.Second
	if testing.Short() {
		d = time.Second
	}
	checkNanotimeMonotonic(t, 4, d, nil)
}

// TestQNXTSCGuard injects a TSC rate twice the real one and checks that
// sysmon's check switches nanotime to CLOCK_MONOTONIC with no backward
// step. The switch is permanent, so it runs in a child process.
func TestQNXTSCGuard(t *testing.T) {
	if os.Getenv("GO_QNX_TSC_GUARD_CHILD") == "1" {
		qnxTSCGuardChild(t)
		return
	}
	if !runtime.QNXTSCOn() {
		t.Skip("nanotime is not using the TSC on this machine")
	}
	testenv.MustHaveExec(t)
	cmd := testenv.Command(t, testenv.Executable(t), "-test.run=^TestQNXTSCGuard$", "-test.v")
	cmd.Env = append(os.Environ(), "GO_QNX_TSC_GUARD_CHILD=1")
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	if !strings.Contains(string(out), "switched to CLOCK_MONOTONIC") {
		t.Fatal("child did not report the switch")
	}
}

func qnxTSCGuardChild(t *testing.T) {
	if !runtime.QNXTSCOn() {
		t.Skip("nanotime is not using the TSC")
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	checkNanotimeMonotonic(t, 4, 500*time.Millisecond, func() {
		runtime.QNXTSCInjectRate(2, 1, 50e6) // a 50 ms window
		deadline := time.Now().Add(5 * time.Second)
		for runtime.QNXTSCOn() && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
			runtime.QNXCheckTSC()
		}
	})
	if runtime.QNXTSCOn() {
		t.Fatal("rate twice the real one, but nanotime still uses the TSC")
	}
	t.Log("switched to CLOCK_MONOTONIC")
}
