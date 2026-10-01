// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package syscall_test

import (
	"internal/testenv"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"testing"
)

func TestOpenFileLimit(t *testing.T) {
	// For open file count,
	// macOS sets the default soft limit to 256 and no hard limit.
	// CentOS and Fedora set the default soft limit to 1024,
	// with hard limits of 4096 and 524288, respectively.
	// Check that we can open 1200 files, which proves
	// that the rlimit is being raised appropriately on those systems.
	fileCount := 1200

	// OpenBSD has a default soft limit of 512 and hard limit of 1024.
	if runtime.GOOS == "openbsd" {
		fileCount = 768
	}

	// QNX 6.5's default soft and hard limits are both 1000 (procnto -F
	// changes them), so there is nothing to raise, and 1200 files
	// cannot be opened. TestOpenFileLimitRaised checks the raise there.
	if runtime.GOOS == "qnx" {
		fileCount = 900
	}

	var files []*os.File
	for i := 0; i < fileCount; i++ {
		f, err := os.Open("rlimit.go")
		if err != nil {
			t.Error(err)
			break
		}
		files = append(files, f)
	}

	for _, f := range files {
		f.Close()
	}
}

// TestOpenFileLimitRaised checks that a Go program raises a soft
// RLIMIT_NOFILE below its hard limit, on systems where the default soft
// limit is already at the hard limit (QNX 6.5: both 1000). It lowers the
// soft limit, runs itself, and has the child open more files than the
// lowered limit allows.
func TestOpenFileLimitRaised(t *testing.T) {
	if os.Getenv("GO_WANT_OPEN_FILES") != "" {
		n, _ := strconv.Atoi(os.Getenv("GO_WANT_OPEN_FILES"))
		var files []*os.File
		defer func() {
			for _, f := range files {
				f.Close()
			}
		}()
		for i := 0; i < n; i++ {
			f, err := os.Open("rlimit.go")
			if err != nil {
				t.Fatalf("opening file %d: %v", i, err)
			}
			files = append(files, f)
		}
		return
	}
	if runtime.GOOS != "qnx" {
		t.Skip("covered by TestOpenFileLimit")
	}
	testenv.MustHaveExec(t)

	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		t.Fatal(err)
	}
	if lim.Max < 600 {
		t.Skipf("hard limit %d is too low", lim.Max)
	}
	low := lim
	low.Cur = 256
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &low); err != nil {
		t.Fatal(err)
	}
	defer syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim)

	n := min(lim.Max-100, 900)
	cmd := testenv.Command(t, testenv.Executable(t), "-test.run=^TestOpenFileLimitRaised$")
	cmd.Env = append(cmd.Environ(), "GO_WANT_OPEN_FILES="+strconv.FormatUint(uint64(n), 10))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child with soft limit %d opening %d files: %v\n%s", low.Cur, n, err, out)
	}
}
