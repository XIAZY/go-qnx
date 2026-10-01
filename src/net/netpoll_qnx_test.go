// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"io"
	"os"
	"testing"
	"time"
)

// TestPollServerOrder checks that sockets work when the poller holds,
// before them, descriptors of one server separated by another's: a
// pipe, /dev/urandom, then another pipe. QNX 6.5's poll reports
// POLLNVAL for the first socket after such a set, and loses its events,
// unless the runtime keeps each server's descriptors together.
func TestPollServerOrder(t *testing.T) {
	r1, w1, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r1.Close()
	defer w1.Close()
	rnd, err := os.Open("/dev/urandom")
	if err != nil {
		t.Skip(err)
	}
	defer rnd.Close()
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	defer w2.Close()

	// Keep a read pending on a pipe, as a program capturing its output
	// would, so that the pipe is armed while the sockets are polled.
	go io.Copy(io.Discard, r2)

	ln, err := Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	accepted := make(chan Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			t.Error(err)
			close(accepted)
			return
		}
		accepted <- c
	}()

	d := Dialer{Timeout: 10 * time.Second}
	c, err := d.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s, ok := <-accepted
	if !ok {
		return
	}
	defer s.Close()

	s.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := c.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	if _, err := io.ReadFull(s, b[:]); err != nil {
		t.Fatalf("read on accepted socket: %v", err)
	}
}
