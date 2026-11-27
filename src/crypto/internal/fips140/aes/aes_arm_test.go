// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !purego

package aes

import (
	"bytes"
	"testing"
)

// TestXORKeyStream8 checks XORKeyStream8, with the NEON code if the CPU
// has it and without, against single blocks from the generic code.
func TestXORKeyStream8(t *testing.T) {
	for _, keySize := range []int{16, 24, 32} {
		key := make([]byte, keySize)
		var src, counters, want [8 * BlockSize]byte
		for i := range key {
			key[i] = byte(3*i + keySize)
		}
		for i := range src {
			src[i] = byte(7 * i)
			counters[i] = byte(i*i + 1)
		}
		b, err := New(key)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(want); i += BlockSize {
			encryptBlockGeneric(&b.blockExpanded, want[i:i+BlockSize], counters[i:i+BlockSize])
		}
		for i := range want {
			want[i] ^= src[i]
		}

		for _, neon := range []bool{b.hasBS, false} {
			b.hasBS = neon
			var got [8 * BlockSize]byte
			XORKeyStream8(b, &got, &src, &counters)
			if !bytes.Equal(got[:], want[:]) {
				t.Errorf("key size %d, NEON %v: got %x, want %x", keySize, neon, got, want)
			}
			// dst and src may be the same memory.
			got = src
			XORKeyStream8(b, &got, &got, &counters)
			if !bytes.Equal(got[:], want[:]) {
				t.Errorf("key size %d, NEON %v, in place: got %x, want %x", keySize, neon, got, want)
			}
		}
	}
}
