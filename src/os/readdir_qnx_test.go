// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build qnx

package os_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// lsNames returns the entries of dir as the C library sees them, via ls, or
// nil if ls can't be found or run. ls and ReadDir both use libc's readdir, so
// they must agree; a plain read(2) on the directory descriptor would not,
// which is the bug this guards.
func lsNames(t *testing.T, dir string) map[string]bool {
	bin := ""
	for _, p := range []string{"/bin/ls", "/usr/bin/ls", "/proc/boot/ls"} {
		if _, err := os.Stat(p); err == nil {
			bin = p
			break
		}
	}
	if bin == "" {
		if p, err := exec.LookPath("ls"); err == nil {
			bin = p
		} else {
			return nil
		}
	}
	out, err := exec.Command(bin, "-a", dir).Output()
	if err != nil {
		t.Logf("ls %q: %v", dir, err)
		return nil
	}
	m := map[string]bool{}
	for _, n := range strings.Fields(string(out)) {
		if n == "." || n == ".." {
			continue
		}
		m[n] = true
	}
	return m
}

// TestReaddirUnionQNX checks that ReadDir lists a union directory (/ and /dev,
// each served by several resource managers) in full and without duplicates.
// QNX merges the servers' entries only through libc's opendir, which os uses;
// a read(2) on a single descriptor would see only the first server.
func TestReaddirUnionQNX(t *testing.T) {
	for _, dir := range []string{"/", "/dev"} {
		ents, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir(%q): %v", dir, err)
		}
		got := map[string]bool{}
		for _, e := range ents {
			if got[e.Name()] {
				t.Errorf("ReadDir(%q): duplicate entry %q", dir, e.Name())
			}
			got[e.Name()] = true
		}
		ls := lsNames(t, dir)
		if ls == nil {
			t.Logf("ReadDir(%q): %d entries; ls unavailable, not compared", dir, len(got))
			continue
		}
		// These directories are unions and, especially /dev, churn between the
		// ReadDir and the ls call, so don't require an exact match. Require a
		// large overlap: the truncation bug returned only the first server's
		// entries (e.g. 11 of 31 for /, 0 of many for /dev), so a listing that
		// shares at least half of ls's entries is not truncated.
		overlap := 0
		for n := range ls {
			if got[n] {
				overlap++
			}
		}
		if len(ls) > 0 && overlap*2 < len(ls) {
			t.Errorf("ReadDir(%q) shares only %d of ls's %d entries (has %d); looks truncated",
				dir, overlap, len(ls), len(got))
		}
	}
}

// TestReaddirReopenVerifyQNX checks the dev+inode verification that guards the
// reopen-by-name ReadDir uses (QNX has no fdopendir): a File whose name still
// names its directory lists; one whose name resolves elsewhere errors rather
// than listing the wrong directory.
func TestReaddirReopenVerifyQNX(t *testing.T) {
	// Correct name: lists.
	f, err := os.Open("/")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	fd, err := syscall.Dup(int(f.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	nf := os.NewFile(uintptr(fd), "/")
	if ents, err := nf.ReadDir(-1); err != nil {
		t.Errorf("NewFile(fd of /, %q).ReadDir: %v", "/", err)
	} else if len(ents) == 0 {
		t.Errorf("NewFile(fd of /, %q).ReadDir: empty", "/")
	}
	nf.Close()

	// Wrong name: the fd is / but the name says /dev, so the reopen lands on
	// a different directory; ReadDir must error, not list it.
	fd2, err := syscall.Dup(int(f.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	nf2 := os.NewFile(uintptr(fd2), "/dev")
	if _, err := nf2.ReadDir(-1); err == nil {
		t.Errorf("NewFile(fd of /, %q).ReadDir: want error, got a listing", "/dev")
	}
	nf2.Close()
}

// TestReaddirRelativeChdirQNX checks that a directory opened by a relative path
// does not list the wrong directory after a chdir away: the reopen by name
// resolves elsewhere, and ReadDir returns an error.
func TestReaddirRelativeChdirQNX(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	if err := os.Mkdir(filepath.Join(d1, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(d1)
	f, err := os.Open("sub") // relative to d1
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Chdir(d2); err != nil {
		t.Fatal(err)
	}
	// "sub" now resolves under d2, where it does not exist.
	if _, err := f.ReadDir(-1); err == nil {
		t.Errorf("ReadDir of a relative directory after chdir away: want error, got a listing")
	}
}

// TestReaddirFileInfoDevQNX records whether Readdir, the []FileInfo form,
// succeeds on /dev. Unlike ReadDir it needs a FileInfo for every entry, so a
// stat failure stops it, as on every platform; on QNX a live resource manager
// under /dev can answer EBUSY to lstat, so this can fail where ReadDir does
// not. The test documents the behavior rather than requiring either outcome.
func TestReaddirFileInfoDevQNX(t *testing.T) {
	f, err := os.Open("/dev")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	infos, err := f.Readdir(-1)
	if err != nil {
		t.Logf("Readdir(-1) on /dev stopped after %d entries: %v "+
			"(a resource manager answered a stat error; ReadDir lists it with an unknown type instead)",
			len(infos), err)
	} else {
		t.Logf("Readdir(-1) on /dev: %d entries, no error", len(infos))
	}
}
