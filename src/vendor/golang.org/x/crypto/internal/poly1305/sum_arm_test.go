// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build gc && !purego

package poly1305

import (
	"bytes"
	"internal/cpu"
	"testing"
)

// TestUpdate checks MAC, which uses the NEON code if the CPU has it,
// against the generic code: for every length up to 700 bytes written in
// two parts, and for 16 KB, with keys and messages that are ordinary, all
// ones and all zeros.
func TestUpdate(t *testing.T) {
	t.Logf("NEON: %v", cpu.ARM.HasNEON)
	fill := map[string]func(i int) byte{
		"mixed": func(i int) byte { return byte(i*i + 7*i + 1) },
		"ones":  func(i int) byte { return 0xff },
		"zeros": func(i int) byte { return 0 },
	}
	for keyName, keyFill := range fill {
		var key [32]byte
		for i := range key {
			key[i] = keyFill(i + 3)
		}
		for msgName, msgFill := range fill {
			msg := make([]byte, 16384)
			for i := range msg {
				msg[i] = msgFill(i)
			}
			check := func(n int) {
				var want [TagSize]byte
				g := newMACGeneric(&key)
				g.Write(msg[:n])
				g.Sum(&want)

				m := New(&key)
				m.Write(msg[:n/3])
				m.Write(msg[n/3 : n])
				if got := m.Sum(nil); !bytes.Equal(got, want[:]) {
					t.Errorf("key %s, message %s, %d bytes: got %x, want %x", keyName, msgName, n, got, want)
				}
			}
			for n := 0; n <= 700; n++ {
				check(n)
			}
			check(len(msg))
		}
	}
}
