// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build gc && !purego

package chacha20

import "internal/cpu"

// With NEON, three blocks at a time are computed by chacha_arm.s. This
// file is not in golang.org/x/crypto: it was added to the copy here.

const bufSize = 3 * blockSize

// xorKeyStream3NEON sets dst to src XOR the three blocks of key stream
// that start at state, whose counter it leaves as it is. dst and src may
// be the same memory.
//
//go:noescape
func xorKeyStream3NEON(dst, src *[bufSize]byte, state *[16]uint32)

func (c *Cipher) xorKeyStreamBlocks(dst, src []byte) {
	if !cpu.ARM.HasNEON {
		c.xorKeyStreamBlocksGeneric(dst, src)
		return
	}
	if len(dst) != len(src) || len(dst)%bufSize != 0 {
		panic("chacha20: internal error: wrong dst and/or src length")
	}
	state := [16]uint32{
		j0, j1, j2, j3,
		c.key[0], c.key[1], c.key[2], c.key[3],
		c.key[4], c.key[5], c.key[6], c.key[7],
		c.counter, c.nonce[0], c.nonce[1], c.nonce[2],
	}
	for len(src) > 0 {
		xorKeyStream3NEON((*[bufSize]byte)(dst), (*[bufSize]byte)(src), &state)
		state[12] += bufSize / blockSize
		dst, src = dst[bufSize:], src[bufSize:]
	}
	c.counter = state[12]
}
