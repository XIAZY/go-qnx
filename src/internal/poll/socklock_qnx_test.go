// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package poll_test

import (
	"internal/poll"
	"syscall"
	"testing"
)

func int8s(s string) []int8 {
	b := make([]int8, len(s)+1)
	for i := range len(s) {
		b[i] = int8(s[i])
	}
	return b
}

func TestReleaseMajor(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want int
	}{
		{"6.5.0", 6},
		{"8.0.0", 8},
		{"10.2", 10},
		{"", -1},
		{"x", -1},
	} {
		if got := poll.ReleaseMajor(int8s(tt.in)); got != tt.want {
			t.Errorf("ReleaseMajor(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// The socket lock is taken below release 8 (QNX 6.5) and not from 8 on
// (BlackBerry 10 reports 8.0.0).
func TestSockLockNeeded(t *testing.T) {
	var u syscall.Utsname
	if err := syscall.Uname(&u); err != nil {
		t.Fatal(err)
	}
	var rel []byte
	for _, c := range u.Release {
		if c == 0 {
			break
		}
		rel = append(rel, byte(c))
	}
	want := poll.ReleaseMajor(u.Release[:]) < 8
	if got := poll.SockLockNeeded(); got != want {
		t.Errorf("release %q: SockLockNeeded() = %v, want %v", rel, got, want)
	}
	t.Logf("release %q: socket lock taken: %v", rel, want)
}
