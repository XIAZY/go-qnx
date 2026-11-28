// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build gc && !purego

package chacha20

import (
	"bytes"
	"internal/cpu"
	"testing"
)

func testCipher(t *testing.T, counter uint32) *Cipher {
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = byte(3*i + 1)
	}
	nonce := make([]byte, NonceSize)
	for i := range nonce {
		nonce[i] = byte(5*i + 2)
	}
	c, err := NewUnauthenticatedCipher(key, nonce)
	if err != nil {
		t.Fatal(err)
	}
	c.SetCounter(counter)
	return c
}

// TestXORKeyStreamBlocks checks xorKeyStreamBlocks, which uses the NEON
// code if the CPU has it, against the generic code.
func TestXORKeyStreamBlocks(t *testing.T) {
	t.Logf("NEON: %v", cpu.ARM.HasNEON)
	src := make([]byte, 5*bufSize)
	for i := range src {
		src[i] = byte(i*i + 7*i + 1)
	}
	for _, counter := range []uint32{0, 0xfe, 0xffffff00} {
		for n := bufSize; n <= len(src); n += bufSize {
			a, b := testCipher(t, counter), testCipher(t, counter)
			want := make([]byte, n)
			b.xorKeyStreamBlocksGeneric(want, src[:n])

			got := make([]byte, n)
			a.xorKeyStreamBlocks(got, src[:n])
			if !bytes.Equal(got, want) || a.counter != b.counter {
				t.Errorf("counter %#x, %d bytes: got %x counter %#x, want %x counter %#x", counter, n, got, a.counter, want, b.counter)
			}
			// dst and src may be the same memory.
			a = testCipher(t, counter)
			copy(got, src[:n])
			a.xorKeyStreamBlocks(got, got)
			if !bytes.Equal(got, want) {
				t.Errorf("counter %#x, %d bytes, in place: got %x, want %x", counter, n, got, want)
			}
		}
	}
}

// TestXORKeyStreamLengths checks XORKeyStream for lengths that are not
// whole buffers, in two calls, so that key stream is carried over.
func TestXORKeyStreamLengths(t *testing.T) {
	src := make([]byte, 3*bufSize+blockSize)
	for i := range src {
		src[i] = byte(i*i + 3*i + 5)
	}
	want := make([]byte, len(src))
	testCipher(t, 1).xorKeyStreamBlocksGeneric(want, src)
	for n := 1; n <= len(src); n++ {
		c := testCipher(t, 1)
		got := make([]byte, n)
		c.XORKeyStream(got[:n/3], src[:n/3])
		c.XORKeyStream(got[n/3:], src[n/3:n])
		if !bytes.Equal(got, want[:n]) {
			t.Fatalf("%d bytes: got %x, want %x", n, got, want[:n])
		}
	}
}
