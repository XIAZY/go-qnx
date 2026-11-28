// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !purego

package sha256

import "testing"

// TestBlockARM checks the assembly, with the NEON schedule if the CPU has
// NEON and without, against blockGeneric, for one to five blocks that
// start at every offset from a word boundary.
func TestBlockARM(t *testing.T) {
	t.Logf("NEON: %v", useNEON)
	impls := map[string]func(*Digest, []byte){"ARM": blockARM}
	if useNEON {
		impls["NEON"] = blockNEON
	}
	for name, block := range impls {
		testBlock(t, name, block)
	}
}

func testBlock(t *testing.T, name string, block func(*Digest, []byte)) {
	buf := make([]byte, 5*chunk+4)
	for i := range buf {
		buf[i] = byte(i*i + 7*i + 1)
	}
	for off := 0; off < 4; off++ {
		for n := chunk; n <= 5*chunk; n += chunk {
			var want, got Digest
			for i := range want.h {
				want.h[i] = uint32(i+1) * 0x9e3779b9
			}
			got.h = want.h
			blockGeneric(&want, buf[off:off+n])
			block(&got, buf[off:off+n])
			if got.h != want.h {
				t.Errorf("%s, offset %d, %d bytes: got %x, want %x", name, off, n, got.h, want.h)
			}
		}
	}
}
