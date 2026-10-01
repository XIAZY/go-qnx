// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall_test

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// TestRestartUnderSignals checks that syscall's wrappers retry calls a
// signal interrupts. QNX has no SA_RESTART: under a stream of signals,
// socket, bind, listen, getsockname and fcntl fail with EINTR unless
// retried.
func TestRestartUnderSignals(t *testing.T) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	defer signal.Stop(c)
	go func() {
		for range c {
		}
	}()

	done := make(chan struct{})
	defer close(done)
	go func() {
		pid := syscall.Getpid()
		for {
			select {
			case <-done:
				return
			default:
			}
			syscall.Kill(pid, syscall.SIGUSR1)
			time.Sleep(50 * time.Microsecond)
		}
	}()

	n := 3000
	if testing.Short() {
		n = 1000
	}
	check := func(what string, err error) {
		t.Helper()
		if err == syscall.EINTR {
			t.Fatalf("%s: %v", what, err)
		}
	}
	for i := 0; i < n; i++ {
		s, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
		check("socket", err)
		if err != nil {
			t.Fatal(err)
		}
		check("setsockopt", syscall.SetsockoptInt(s, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1))
		check("bind", syscall.Bind(s, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}))
		_, err = syscall.Getsockname(s)
		check("getsockname", err)
		check("fcntl", syscall.SetNonblock(s, true))
		check("listen", syscall.Listen(s, 5))
		syscall.Close(s)
	}
}
