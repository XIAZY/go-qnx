// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package runtime_test

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNetpollBusyP checks that a goroutine blocked on a descriptor is
// woken while every P is busy, so that no M ever blocks in netpoll and
// only the non-blocking polls from sysmon and the scheduler can see the
// descriptor become ready. On qnx a netpoll(0) that never polled starved
// such goroutines indefinitely.
func TestNetpollBusyP(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	var stop atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < runtime.GOMAXPROCS(0)+1; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
			}
		}()
	}
	defer func() {
		stop.Store(true)
		wg.Wait()
	}()

	done := make(chan error, 1)
	go func() {
		var b [1]byte
		_, err := r.Read(b[:])
		done <- err
	}()
	time.Sleep(10 * time.Millisecond) // let the reader block
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("pipe reader not woken within 30s while all Ps were busy")
	}
}
