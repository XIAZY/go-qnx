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

// TestPollReaderAndWriter has a reader and a writer wait on one socket
// at the same time, round after round, while the peer makes it readable
// and writable in turn. On qnx one armed event serves
// both waiters, and every pulse wakes both: a waiter whose arm was used
// up by the other's event wakes, sees EAGAIN and arms again. This is a
// liveness test: a waiter left without an arm would sleep forever.
func TestPollReaderAndWriter(t *testing.T) {
	ln := newLocalListener(t, "tcp")
	defer ln.Close()
	peerc := make(chan Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			t.Error(err)
			close(peerc)
			return
		}
		peerc <- c
	}()
	c, err := Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	peer := <-peerc
	if peer == nil {
		return
	}
	defer peer.Close()

	// Fill c's send buffer so that its writer has to wait.
	c.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	chunk := make([]byte, 64<<10)
	for {
		if _, err := c.Write(chunk); err != nil {
			break
		}
	}
	c.SetWriteDeadline(time.Time{})

	rounds := 10000
	if testing.Short() {
		rounds = 1000
	}
	// A round takes about 0.1 s on a nested VM: stop early, saying so,
	// rather than run into the test timeout.
	stop := time.Time{}
	if d, ok := t.Deadline(); ok {
		stop = d.Add(-time.Minute)
	}
	buf := make([]byte, 256<<10)
	for i := 0; i < rounds; i++ {
		done := make(chan string, 2)
		go func() {
			var b [1]byte
			if _, err := io.ReadFull(c, b[:]); err != nil {
				done <- "read: " + err.Error()
				return
			}
			done <- ""
		}()
		go func() {
			if _, err := c.Write(chunk[:1]); err != nil {
				done <- "write: " + err.Error()
				return
			}
			done <- ""
		}()
		// Wake the two in an order that varies with i.
		if i%2 == 0 {
			peer.Write([]byte{1})
		}
		peer.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		peer.Read(buf)
		if i%2 == 1 {
			peer.Write([]byte{1})
		}
		for j := 0; j < 2; j++ {
			select {
			case msg := <-done:
				if msg != "" {
					t.Fatalf("round %d: %s", i, msg)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("round %d: a reader or writer on one socket is still waiting after 10s", i)
			}
		}
		if !stop.IsZero() && time.Now().After(stop) {
			t.Logf("stopping after %d rounds, near the test deadline", i+1)
			break
		}
		// Refill what the peer drained, so the next writer waits again.
		c.SetWriteDeadline(time.Now().Add(5 * time.Millisecond))
		for {
			if _, err := c.Write(chunk); err != nil {
				break
			}
		}
		c.SetWriteDeadline(time.Time{})
	}
}
