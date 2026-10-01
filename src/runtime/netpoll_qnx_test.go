// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"errors"
	"io"
	"os"
	"runtime"
	"testing"
	"time"
)

// TestNetpollPipeMetButFull covers a pipe whose server reports OUTPUT met
// while a write still gets EAGAIN: the pipe has a few bytes free, fewer
// than the write, which is atomic. Arming with POLLARM alone answers
// "met" again at once, and the writer spins on self-pulses, starving the
// process (it measured 90,000 a second); arming with TRANARM alone
// misses a reader that drained before the arm. The poller uses TRANARM
// with a 1 ms recheck. The test checks both halves: no spin while the
// pipe stays full, and the writer finishes once a reader drains it.
func TestNetpollPipeMetButFull(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	// Fill the pipe with 19-byte writes. 19 divides no power of two, so
	// a few bytes stay free.
	msg := make([]byte, 19)
	w.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
	n := 0
	for {
		if _, err := w.Write(msg); err != nil {
			if !errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatal(err)
			}
			break
		}
		n += len(msg)
	}
	t.Logf("pipe holds %d bytes", n)

	self0, tran0, recheck0 := runtime.NetpollPulseCounts()
	w.SetWriteDeadline(time.Now().Add(10 * time.Second))
	done := make(chan error, 1)
	go func() {
		_, err := w.Write(msg)
		done <- err
	}()
	const wait = 200 * time.Millisecond
	time.Sleep(wait) // the pipe stays full; the writer arms and waits
	self, tran, recheck := runtime.NetpollPulseCounts()
	self, tran, recheck = self-self0, tran-tran0, recheck-recheck0
	t.Logf("while full, %v: %d self-pulses, %d TRANARMs, %d rechecks", wait, self, tran, recheck)
	select {
	case err := <-done:
		t.Fatalf("write finished with the pipe full: %v", err)
	default:
	}
	// While the pipe stays full the writer's arms alternate: POLLARM
	// answered "met" (a self-pulse), then TRANARM with a recheck 1 ms
	// later. That is at most one self-pulse and one recheck a
	// millisecond; the spin was 90 self-pulses a millisecond.
	if limit := uint64(4 * wait / time.Millisecond); self+recheck > limit {
		t.Errorf("%d self-pulses and %d rechecks in %v, want under %d in all: the writer spins", self, recheck, wait, limit)
	}
	if tran == 0 || recheck == 0 {
		t.Errorf("%d TRANARMs and %d rechecks, want both: the met-but-full path was not taken", tran, recheck)
	}

	start := time.Now()
	buf := make([]byte, n+len(msg))
	if _, err := io.ReadAtLeast(r, buf, n); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("write after the drain: %v", err)
	}
	t.Logf("write finished %v after the drain began", time.Since(start))
}
