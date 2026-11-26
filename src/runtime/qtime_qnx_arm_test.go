// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"runtime"
	"sync"
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

// TestQNXCpupage runs goroutines on several threads for 2 s, so that
// save_g and load_g, which read the control block directly when on, are
// exercised on every goroutine switch: a wrong g would crash the run.
func TestQNXCpupage(t *testing.T) {
	on := runtime.QNXCpupageOn()
	t.Logf("save_g/load_g read the control block directly: %v", on)
	if !on {
		t.Log("not on: this runs the libc __tls path")
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))

	stop := make(chan struct{})
	time.AfterFunc(2*time.Second, func() { close(stop) })
	var wg sync.WaitGroup
	var sink [8][]byte
	chans := make([]chan int, 8)
	for i := range chans {
		chans[i] = make(chan int, 1)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			in, out := chans[i], chans[(i+1)%8]
			for n := 0; ; n++ {
				select {
				case v := <-in:
					sink[i] = make([]byte, 64+n%256)
					runtime.Gosched()
					select {
					case out <- v + 1:
					case <-stop:
						return
					}
				case <-stop:
					return
				}
			}
		}()
	}
	for i := 0; i < 8; i += 2 { // four tokens go round the ring
		chans[i] <- 0
	}
	wg.Wait()

	// Locked goroutines keep their thread, and each must see its own g's
	// state however the others move between threads.
	var lwg sync.WaitGroup
	errs := make(chan string, 4)
	for i := 0; i < 4; i++ {
		lwg.Add(1)
		go func() {
			defer lwg.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			x := i * 1000
			deadline := time.Now().Add(500 * time.Millisecond)
			for n := 0; time.Now().Before(deadline); n++ {
				x++
				runtime.Gosched()
				buf := make([]byte, 128)
				buf[0] = byte(x)
				if x != i*1000+n+1 || buf[0] != byte(x) {
					errs <- "locked goroutine's state changed"
					return
				}
			}
		}()
	}
	lwg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}
