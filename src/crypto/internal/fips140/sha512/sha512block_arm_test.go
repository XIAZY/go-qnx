// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !purego

package sha512

import "testing"

// TestBlockNEON checks blockNEON against blockGeneric, for one to five
// blocks that start at every offset from a word boundary.
func TestBlockNEON(t *testing.T) {
	t.Logf("NEON: %v", useNEON)
	if !useNEON {
		t.Skip("no NEON")
	}
	buf := make([]byte, 5*chunk+8)
	for i := range buf {
		buf[i] = byte(i*i + 7*i + 1)
	}
	for off := 0; off < 8; off++ {
		for n := chunk; n <= 5*chunk; n += chunk {
			var want, got Digest
			for i := range want.h {
				want.h[i] = uint64(i+1) * 0x9e3779b97f4a7c15
			}
			got.h = want.h
			blockGeneric(&want, buf[off:off+n])
			blockNEON(&got, buf[off:off+n])
			if got.h != want.h {
				t.Errorf("offset %d, %d bytes: got %x, want %x", off, n, got.h, want.h)
			}
		}
	}
}
