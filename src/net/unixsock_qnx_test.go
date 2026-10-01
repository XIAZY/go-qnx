// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestQNXUnixAbsoluteName checks that a socket bound to an absolute
// path reports that path: QNX reports it without the leading slash,
// and syscall restores it.
func TestQNXUnixAbsoluteName(t *testing.T) {
	name := testUnixAddr(t)
	ln, err := Listen("unix", name)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if got := ln.Addr().String(); got != name {
		t.Errorf("Addr() = %q; want %q", got, name)
	}
	c, err := Dial("unix", name)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.RemoteAddr().String(); got != name {
		t.Errorf("RemoteAddr() = %q; want %q", got, name)
	}
}

// TestQNXUnixUnnamed checks that an unbound socket's name stays empty
// rather than becoming "/".
func TestQNXUnixUnnamed(t *testing.T) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		t.Fatal(err)
	}
	if u, ok := sa.(*syscall.SockaddrUnix); ok && u.Name != "" {
		t.Errorf("unbound socket name = %q; want \"\"", u.Name)
	}
}

// TestQNXUnixRelativeNameAtRoot records that QNX resolves a relative
// socket path from the root, not from the working directory. If this
// starts failing, QNX (or the port) has changed how relative names
// work, and the comments that rely on it need updating.
func TestQNXUnixRelativeNameAtRoot(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	base := filepath.Base(dir) + ".sock" // unique, and unlikely at the root
	atRoot := "/" + base
	if _, err := os.Stat(atRoot); err == nil {
		t.Skipf("%s already exists", atRoot)
	}
	ln, err := Listen("unix", base)
	if err != nil {
		t.Skipf("cannot bind %q: %v", base, err)
	}
	defer os.Remove(atRoot)
	defer ln.Close()

	if _, err := os.Stat(filepath.Join(dir, base)); err == nil {
		t.Errorf("socket created in the working directory; QNX used to create it at the root")
	}
	if _, err := os.Stat(atRoot); err != nil {
		t.Errorf("socket not created at %s: %v", atRoot, err)
	}
	if got := ln.Addr().String(); got != atRoot {
		t.Errorf("Addr() = %q; want %q", got, atRoot)
	}

	// Close unlinks the name where io-pkt created it, at the root, not
	// in the working directory: otherwise it would stay in io-pkt's
	// table of about 1000 names until a reboot.
	ln.Close()
	if _, err := os.Stat(atRoot); err == nil {
		t.Errorf("%s still exists after Close", atRoot)
	}
}

// TestQNXUnlinkWhileSocketCalls closes Unix listeners, which unlinks their
// names, and removes the names of Unix datagram sockets, while other
// goroutines make socket calls of their own. io-pkt stops answering for
// good if a Unix socket's name is unlinked while it serves another socket
// request (see internal/poll's socklock_qnx.go); without the socket lock,
// this pattern wedged it within a few dozen rounds. A failure is a hung
// test and a machine that must be rebooted.
func TestQNXUnlinkWhileSocketCalls(t *testing.T) {
	const rounds = 300
	dir := t.TempDir()
	errc := make(chan error, 4)
	for g := 0; g < 2; g++ {
		go func() {
			for i := 0; i < rounds; i++ {
				ln, err := Listen("unix", filepath.Join(dir, fmt.Sprintf("s%d.%d", g, i)))
				if err != nil {
					errc <- err
					return
				}
				if err := ln.Close(); err != nil {
					errc <- err
					return
				}
			}
			errc <- nil
		}()
	}
	go func() {
		for i := 0; i < rounds; i++ {
			ln, err := Listen("tcp", "127.0.0.1:0")
			if err != nil {
				errc <- err
				return
			}
			if err := ln.Close(); err != nil {
				errc <- err
				return
			}
		}
		errc <- nil
	}()
	go func() {
		for i := 0; i < rounds; i++ {
			name := filepath.Join(dir, fmt.Sprintf("g%d", i))
			c, err := ListenPacket("unixgram", name)
			if err != nil {
				errc <- err
				return
			}
			if err := c.Close(); err != nil {
				errc <- err
				return
			}
			if err := os.Remove(name); err != nil {
				errc <- err
				return
			}
		}
		errc <- nil
	}()
	for range 4 {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}
}
