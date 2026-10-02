// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func init() {
	// A helper process that exits at once, as true(1) does, for the
	// exec tests: not every QNX system has /bin/true.
	if os.Getenv("GO_QNX_TRUE_HELPER") == "1" {
		os.Exit(0)
	}
	// A helper process for TestQNXSpawnExtraFilesOnly and
	// TestQNXSpawnClosedSlot: print which of the
	// descriptors listed in the variable are open.
	if list := os.Getenv("GO_QNX_FDS_HELPER"); list != "" {
		var st syscall.Stat_t
		for _, f := range strings.Fields(list) {
			var fd int
			fmt.Sscan(f, &fd)
			if syscall.Fstat(fd, &st) == nil {
				fmt.Println(fd)
			}
		}
		os.Exit(0)
	}
	// Helper processes for TestQNXSpawnSignals. "report": say whether
	// SIGHUP was ignored when the process started. "ignore DIR": ignore
	// SIGHUP, start a "report" child in DIR and pass on what it says.
	switch h := os.Getenv("GO_QNX_SIGHUP_HELPER"); {
	case h == "report":
		fmt.Println("sighup ignored:", signal.Ignored(syscall.SIGHUP))
		os.Exit(0)
	case strings.HasPrefix(h, "ignore "):
		signal.Ignore(syscall.SIGHUP)
		out, err := sighupChild(strings.TrimPrefix(h, "ignore "), "report")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(out)
		os.Exit(0)
	}
}

// trueCommand returns a command that runs this test binary as a
// process that exits at once with status 0, ignoring its arguments.
func trueCommand(t *testing.T, args ...string) *exec.Cmd {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "GO_QNX_TRUE_HELPER=1")
	return cmd
}

// sighupChild runs this test binary as the helper process given by
// mode, in dir ("" starts it with spawn, "/" with vfork).
func sighupChild(dir, mode string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GO_QNX_SIGHUP_HELPER="+mode)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// QNX 6.5 allows 8 supplementary groups (NGROUPS_MAX). The vfork child
// must report setgroups' EINVAL for more through the error pipe, like
// any other failure before exec.
func TestQNXSetgroupsLimit(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("setgroups needs root")
	}
	for _, n := range []int{8, 9} {
		groups := make([]uint32, n)
		for i := range groups {
			groups[i] = uint32(i)
		}
		cmd := exec.Command("/bin/sh", "-c", "exit 0")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: 0, Gid: 0, Groups: groups},
		}
		err := cmd.Run()
		switch {
		case n <= 8 && err != nil:
			t.Errorf("%d groups: %v", n, err)
		case n > 8 && !errors.Is(err, syscall.EINVAL):
			t.Errorf("%d groups: got %v, want EINVAL", n, err)
		}
	}
}

// QNX's execve builds its spawn message, which holds every argument and
// environment string, on the stack; in a vfork child that is the child's
// own stack in the parent's memory. Exec with a large environment and
// argument list while other goroutines allocate, and check that the
// parent survives with an intact heap.
func TestQNXExecLargeEnvironment(t *testing.T) {
	env := append(os.Environ(), "BIG="+strings.Repeat("x", 256<<10), "GO_QNX_TRUE_HELPER=1")
	args := make([]string, 64<<10/8)
	for i := range args {
		args[i] = "1234567" // 7 bytes and a NUL
	}

	// Objects the parent checks afterwards: a child that wrote outside
	// its stack would have scribbled on the parent's heap.
	canaries := make([][]byte, 256)
	for i := range canaries {
		canaries[i] = []byte(strings.Repeat(string(rune('a'+i%26)), 4096))
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = make([]byte, 64<<10)
				runtime.Gosched()
			}
		}()
	}
	for range 100 {
		cmd := trueCommand(t, args...)
		cmd.Env = env
		if err := cmd.Run(); err != nil {
			close(stop)
			wg.Wait()
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
	runtime.GC()
	for i, c := range canaries {
		want := strings.Repeat(string(rune('a'+i%26)), 4096)
		if string(c) != want {
			t.Fatalf("canary %d was overwritten", i)
		}
	}
}

// QNX's vfork copies the descriptor table one descriptor at a time and
// fails with EBADF if another thread closes a descriptor meanwhile.
// Close and vfork are ordered by a lock, so execs must succeed while
// other goroutines open and close descriptors as fast as they can.
func TestQNXExecWhileClosing(t *testing.T) {
	// Without the lock, on a 4-CPU BlackBerry 10 device 10 runs of this
	// test failed at exec 2 to 48 (1 to 24 vforks): about 1 vfork in 6
	// failed, so the short count misses the bug less than 1 time in 100.
	// On one CPU the race almost never happens (20,000 unguarded execs
	// on a one-CPU QNX 6.5 found none), so no count catches it there.
	n := 2000
	if testing.Short() {
		n = 50
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				f, err := os.Open("/dev/null")
				if err == nil {
					f.Close()
				}
				if r, w, err := os.Pipe(); err == nil {
					r.Close()
					w.Close()
				}
				if s, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0); err == nil {
					syscall.Close(s)
				}
			}
		}()
	}
	defer func() { close(stop); wg.Wait() }()
	for i := range n {
		cmd := trueCommand(t)
		if i%2 == 1 {
			cmd.Dir = "/" // vfork, not spawn
		}
		if err := cmd.Run(); err != nil {
			t.Fatalf("exec %d of %d (Dir %q): %v", i+1, n, cmd.Dir, err)
		}
	}
}

// vfork also fails with EBADF while other threads keep opening files, in
// runs of up to a few milliseconds. An exec with Dir set, which uses
// vfork, must ride out such a run (cmd/go starts the compiler that way
// while it opens files on other threads), and must still give up when
// vfork keeps failing. The runs are simulated: they are too rare to
// provoke reliably in a test.
func TestQNXExecDirVforkFailures(t *testing.T) {
	defer syscall.SetVforkFailHook(nil)
	run := func(fail time.Duration) (time.Duration, error) {
		var start time.Time
		syscall.SetVforkFailHook(func() bool {
			if start.IsZero() {
				start = time.Now()
			}
			return time.Since(start) < fail
		})
		cmd := trueCommand(t)
		cmd.Dir = "/" // vfork, not spawn
		t0 := time.Now()
		err := cmd.Run()
		return time.Since(t0), err
	}
	// A 20 ms run of failures: retries spread over time outlast it.
	if d, err := run(20 * time.Millisecond); err != nil {
		t.Errorf("exec through 20 ms of vfork failures: %v after %v", err, d)
	}
	// Failures that never end: give up with EBADF after about a second.
	d, err := run(time.Hour)
	if !errors.Is(err, syscall.EBADF) {
		t.Errorf("exec with vfork always failing: got %v, want EBADF", err)
	}
	if d < time.Second || d > 5*time.Second {
		t.Errorf("exec with vfork always failing gave up after %v, want 1 to 5 s", d)
	}
}

// On qnx a child without Dir, Chroot, Credential or tty settings is
// started with spawn, which gives it exactly its Stdin, Stdout, Stderr
// and ExtraFiles: descriptors 0 to 3 here, and nothing else, whether or
// not the parent's other descriptors are close-on-exec.
func TestQNXSpawnExtraFilesOnly(t *testing.T) {
	// Run twice: with the test's descriptors at whatever numbers are free,
	// and pushed above 10 on purpose. (An earlier version asked /bin/sh,
	// which, like other Korn shells, cannot redirect from descriptors 10
	// and up, and reported those as missing.)
	for _, high := range []bool{false, true} {
		if high {
			for range 12 {
				f, err := os.Open("/dev/null")
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
			}
		}
		testSpawnExtraFilesOnly(t)
	}
}

func testSpawnExtraFilesOnly(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	// Descriptors the child must not get: one close-on-exec (as all of
	// Go's are), one not.
	cloexec, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer cloexec.Close()
	inherit, err := syscall.Dup(int(cloexec.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(inherit)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	list := fmt.Sprintf("0 1 2 3 %d %d", cloexec.Fd(), inherit)
	for _, dir := range []string{"", "/"} {
		cmd := exec.Command(exe)
		cmd.Env = append(os.Environ(), "GO_QNX_FDS_HELPER="+list)
		cmd.Dir = dir
		cmd.ExtraFiles = []*os.File{r}
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("Dir %q: %v", dir, err)
		}
		got := strings.Fields(string(out))
		want := []string{"0", "1", "2", "3"}
		if dir == "/" {
			// vfork: a descriptor without close-on-exec is inherited.
			want = append(want, fmt.Sprint(inherit))
		}
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("Dir %q, descriptors %s: child has %v, want %v", dir, list, got, want)
		}
	}
}

// A nil entry in ExtraFiles is a descriptor closed in the child, on
// both paths: spawn's SPAWN_FDCLOSED and vfork's close. Slots 3, 10
// and 12 are closed here, and the parent has descriptors with those
// numbers open and not close-on-exec, so an ignored SPAWN_FDCLOSED
// would show. (Slots 0 to 2 can't be checked this way: a Go child
// opens /dev/null on any of them it finds closed.)
func TestQNXSpawnClosedSlot(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	null, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	for {
		fd, err := syscall.Dup(int(null.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		defer syscall.Close(fd)
		if fd >= 14 {
			break
		}
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Child slots 3 to 13.
	closed := map[int]bool{3: true, 10: true, 12: true}
	extra := make([]*os.File, 11)
	var list, want []string
	for i := range extra {
		if !closed[3+i] {
			extra[i] = r
			want = append(want, fmt.Sprint(3+i))
		}
		list = append(list, fmt.Sprint(3+i))
	}
	for _, dir := range []string{"", "/"} {
		cmd := exec.Command(exe)
		cmd.Env = append(os.Environ(), "GO_QNX_FDS_HELPER="+strings.Join(list, " "))
		cmd.Dir = dir
		cmd.ExtraFiles = extra
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("Dir %q: %v", dir, err)
		}
		if got := strings.Fields(string(out)); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("Dir %q, slots 3, 10 and 12 closed: child has %v open of %v, want %v", dir, got, list, want)
		}
	}
}

// A signal Go handles comes back to its default action in the child; a
// signal the program ignores stays ignored, as across execve. SIGHUP,
// because a Go child reports an inherited SIG_IGN only for SIGHUP and
// SIGINT (see sigInstallGoHandler in the runtime). The ignoring parent
// is a helper process: signal.Reset does not undo signal.Ignore, so
// ignoring SIGHUP here would change the test process for good.
func TestQNXSpawnSignals(t *testing.T) {
	for _, dir := range []string{"", "/"} {
		if signal.Ignored(syscall.SIGHUP) {
			t.Log("SIGHUP was ignored when the test started (nohup?); not checking the handled case")
		} else if got, err := sighupChild(dir, "report"); err != nil {
			t.Fatalf("Dir %q: %v", dir, err)
		} else if got != "sighup ignored: false" {
			t.Errorf("Dir %q, SIGHUP handled by Go in the parent: child says %q", dir, got)
		}
		if got, err := sighupChild("", "ignore "+dir); err != nil {
			t.Fatalf("Dir %q: %v", dir, err)
		} else if got != "sighup ignored: true" {
			t.Errorf("Dir %q, SIGHUP ignored in the parent: child says %q", dir, got)
		}
	}
}
