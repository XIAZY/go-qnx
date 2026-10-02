// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"bufio"
	"internal/poll"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestQNXBlockopMask checks that the calls bracketed against io-pkt's
// blockop crash block asynchronous signals while they run, leave the
// synchronous ones deliverable, and restore the thread's mask afterwards,
// on success and on error. No socket name is created: io-pkt keeps every
// name until it is removed, and removing one has its own hazard.
func TestQNXBlockopMask(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	before := runtime.QNXThreadSigmask()

	runtime.QNXBlockopBegin()
	during := runtime.QNXThreadSigmask()
	runtime.QNXBlockopEnd()
	for _, s := range []syscall.Signal{syscall.SIGPROF, syscall.SIGURG, syscall.SIGINT, syscall.SIGCHLD} {
		if !runtime.QNXSigBit(during, int(s)) {
			t.Errorf("%v not blocked inside the bracket", s)
		}
	}
	for _, s := range []syscall.Signal{syscall.SIGSEGV, syscall.SIGBUS, syscall.SIGFPE, syscall.SIGILL, syscall.SIGTRAP} {
		if runtime.QNXSigBit(during, int(s)) {
			t.Errorf("%v blocked inside the bracket", s)
		}
	}

	check := func(what string) {
		t.Helper()
		if got := runtime.QNXThreadSigmask(); got != before {
			t.Errorf("%s: thread mask %x afterwards, want %x", what, got, before)
		}
		if n := runtime.QNXBlockops(); n != 0 {
			t.Errorf("%s: %d bracketed calls in flight afterwards", what, n)
		}
	}

	dir := t.TempDir()
	s, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(s)
	err = syscall.Bind(s, &syscall.SockaddrUnix{Name: filepath.Join(dir, "missing", "sock")})
	if err == nil {
		t.Fatal("bind into a missing directory succeeded")
	}
	check("unix bind (error)")
	in, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(in)
	if err := syscall.Bind(in, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		t.Fatal(err)
	}
	check("inet bind")

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fds[0])
	defer syscall.Close(fds[1])
	f := filepath.Join(dir, "file")
	if err := os.WriteFile(f, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	pass, err := os.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	defer pass.Close()
	if err := syscall.Sendmsg(fds[0], []byte{0}, syscall.UnixRights(int(pass.Fd())), nil, 0); err != nil {
		t.Fatal(err)
	}
	check("sendmsg with SCM_RIGHTS")
	// Receive the descriptor, so that none is left in flight when the
	// sockets are closed: io-pkt discards those through the same path.
	oob := make([]byte, syscall.CmsgSpace(4))
	_, oobn, _, _, err := syscall.Recvmsg(fds[1], make([]byte, 1), oob, 0)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(msgs) != 1 {
		t.Fatalf("control messages %v, %v", msgs, err)
	}
	got, err := syscall.ParseUnixRights(&msgs[0])
	if err != nil || len(got) != 1 {
		t.Fatalf("rights %v, %v", got, err)
	}
	syscall.Close(got[0])
	if err := syscall.Sendmsg(-1, []byte{0}, syscall.UnixRights(int(pass.Fd())), nil, 0); err == nil {
		t.Fatal("sendmsg on a bad descriptor succeeded")
	}
	check("sendmsg with SCM_RIGHTS (error)")

	if err := poll.UnlinkSocket(f); err != nil {
		t.Fatal(err)
	}
	check("UnlinkSocket")
	if err := poll.UnlinkSocket(f); err == nil {
		t.Fatal("second unlink succeeded")
	}
	check("UnlinkSocket (error)")
}

// TestQNXBlockopExit checks that exit waits for a bracketed call in
// flight, and that it waits a bounded time, about a second, for one that
// never returns. The child reports when it is about to exit; the parent times
// the rest.
func TestQNXBlockopExit(t *testing.T) {
	if mode := os.Getenv("GO_QNX_BLOCKOP_EXIT"); mode != "" {
		qnxBlockopExitChild(mode)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode     string
		min, max time.Duration
	}{
		{"none", 0, 200 * time.Millisecond},
		{"300ms", 250 * time.Millisecond, 800 * time.Millisecond},
		// The exiting goroutine holds the only P: the call must
		// stop counting when it returns, not when its goroutine
		// next runs.
		{"300ms-1P", 250 * time.Millisecond, 800 * time.Millisecond},
		{"never", 700 * time.Millisecond, 3 * time.Second},
	} {
		cmd := exec.Command(exe, "-test.run=^TestQNXBlockopExit$")
		cmd.Env = append(os.Environ(), "GO_QNX_BLOCKOP_EXIT="+tc.mode)
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(out).ReadString('\n')
		start := time.Now()
		if err != nil || line != "exiting\n" {
			cmd.Wait()
			t.Fatalf("%s: child said %q, %v", tc.mode, line, err)
		}
		err = cmd.Wait()
		d := time.Since(start)
		if err != nil {
			t.Errorf("%s: %v", tc.mode, err)
		}
		t.Logf("%s: exit took %v", tc.mode, d)
		if d < tc.min || d > tc.max {
			t.Errorf("%s: exit took %v, want %v to %v", tc.mode, d, tc.min, tc.max)
		}
	}
}

func qnxBlockopExitChild(mode string) {
	in := make(chan bool)
	switch mode {
	case "300ms", "300ms-1P":
		if mode == "300ms-1P" {
			runtime.GOMAXPROCS(1)
		}
		go func() {
			runtime.QNXBlockopBegin()
			in <- true
			runtime.QNXBlockopCallSleep(300000)
			runtime.QNXBlockopEnd()
		}()
		<-in
		time.Sleep(10 * time.Millisecond) // let the call start
	case "never":
		go func() {
			runtime.QNXBlockopBegin()
			in <- true
			runtime.QNXBlockopCallHang()
		}()
		<-in
		time.Sleep(10 * time.Millisecond)
	}
	os.Stdout.WriteString("exiting\n")
	os.Exit(0)
}
