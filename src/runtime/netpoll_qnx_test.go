// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
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
	// a few bytes stay free. The pipe is full when one write has waited
	// 50 ms, so the deadline is set afresh for each write: one deadline
	// for the whole loop can pass before the pipe fills on a slow machine
	// (it did under emulation, at 4,142 bytes).
	msg := make([]byte, 19)
	n := 0
	for {
		w.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
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

// TestNetpollPipePingPong passes a byte back and forth over two pipes
// many times. A round trip whose read waits after a "met" arm goes
// through the TRANARM and recheck path; the recheck makes the reader
// ready inside netpoll, and netpoll must not then block in its receive
// with that goroutine in hand. It did, and the round trip stalled until
// some unrelated pulse arrived: forever, when no timer is running.
//
// So the test runs no Go timer at all: a watchdog on its own thread,
// sleeping with Nanosleep, fails the test if a round trip takes longer
// than 2 seconds (a normal one takes a few milliseconds at most).
//
// Without the fix it failed 10 runs out of 10 on a BlackBerry 10 device,
// whose CLOCK_MONOTONIC ticks every millisecond, at round trip 17 to
// 1235 (9 of the 10 within the short count), and 0 out of 10 on a
// one-CPU QNX 6.5 machine, where nanotime has sub-microsecond
// resolution and a recheck is rarely already due when netpoll starts.
func TestNetpollPipePingPong(t *testing.T) {
	r1, w1, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r1.Close()
	defer w1.Close()
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	defer w2.Close()

	go func() {
		buf := make([]byte, 1)
		for {
			if _, err := io.ReadFull(r1, buf); err != nil {
				return
			}
			if _, err := w2.Write(buf); err != nil {
				return
			}
		}
	}()

	n := 3000
	if testing.Short() {
		n = 1000
	}
	var trip, started atomic.Int64 // round trip in progress, and its start
	var done atomic.Bool
	watchdogDone := make(chan struct{})
	go func() {
		defer close(watchdogDone)
		runtime.LockOSThread()
		ts := syscall.Timespec{Nsec: 100e6}
		for !done.Load() {
			syscall.Nanosleep(&ts, nil)
			if s := started.Load(); s != 0 && time.Now().UnixNano()-s > 2e9 {
				panic(fmt.Sprintf("TestNetpollPipePingPong: round trip %d of %d stalled for over 2 s", trip.Load(), n))
			}
		}
	}()

	buf := make([]byte, 1)
	for i := range n {
		trip.Store(int64(i + 1))
		started.Store(time.Now().UnixNano())
		if _, err := w1.Write(buf); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(r2, buf); err != nil {
			t.Fatal(err)
		}
		started.Store(0)
	}
	done.Store(true)
	<-watchdogDone
}
