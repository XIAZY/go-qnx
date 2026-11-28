// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !purego

package aes

import (
	"crypto/internal/fips140/subtle"
	"crypto/internal/fips140deps/byteorder"
	"crypto/internal/fips140deps/cpu"
	"crypto/internal/impl"
)

// With NEON, eight blocks at a time are encrypted by a bit-sliced
// implementation in aes_arm.s, which takes its own form of the key
// schedule. It serves the counter modes; a single block still goes
// through the generic code.

// bsKeySize is the size of a bit-sliced key schedule of 14 rounds: the
// first and last round keys take 16 bytes and the others 128.
const bsKeySize = 16 + 13*128 + 16

//go:noescape
func bsaesKeyConvertNEON(dst *byte, enc *uint32, rounds int)

// xorKeyStream8NEON sets dst to src XOR the encryption of counters.
//
//go:noescape
func xorKeyStream8NEON(bs *byte, rounds int, dst, src, counters *[8 * BlockSize]byte)

var supportsNEON = cpu.ARMHasNEON

func init() {
	impl.Register("aes", "NEON", &supportsNEON)
}

type block struct {
	blockExpanded
	bs    [bsKeySize]byte
	hasBS bool // bs was set, which it is only if NEON was on in newBlock
}

func newBlock(c *Block, key []byte) *Block {
	newBlockExpanded(&c.blockExpanded, key)
	if supportsNEON {
		bsaesKeyConvertNEON(&c.bs[0], &c.enc[0], c.rounds)
		c.hasBS = true
	}
	return c
}

func encryptBlock(c *Block, dst, src []byte) {
	encryptBlockGeneric(&c.blockExpanded, dst, src)
}

func decryptBlock(c *Block, dst, src []byte) {
	decryptBlockGeneric(&c.blockExpanded, dst, src)
}

// The generic code is expected even with NEON, for single blocks.
func checkGenericIsExpected() {}

// XORKeyStream8 sets dst to src XOR the encryption of the eight blocks in
// counters. dst and src may be the same memory. It is for package gcm.
func XORKeyStream8(b *Block, dst, src, counters *[8 * BlockSize]byte) {
	if supportsNEON && b.hasBS {
		xorKeyStream8NEON(&b.bs[0], b.rounds, dst, src, counters)
		return
	}
	var ks [8 * BlockSize]byte
	for i := 0; i < len(ks); i += BlockSize {
		encryptBlock(b, ks[i:i+BlockSize], counters[i:i+BlockSize])
	}
	subtle.XORBytes(dst[:], src[:], ks[:])
}

func ctrBlocks1(b *Block, dst, src *[BlockSize]byte, ivlo, ivhi uint64) {
	ctrBlocks(b, dst[:], src[:], ivlo, ivhi)
}

func ctrBlocks2(b *Block, dst, src *[2 * BlockSize]byte, ivlo, ivhi uint64) {
	ctrBlocksNEON(b, dst[:], src[:], ivlo, ivhi)
}

func ctrBlocks4(b *Block, dst, src *[4 * BlockSize]byte, ivlo, ivhi uint64) {
	ctrBlocksNEON(b, dst[:], src[:], ivlo, ivhi)
}

func ctrBlocks8(b *Block, dst, src *[8 * BlockSize]byte, ivlo, ivhi uint64) {
	ctrBlocksNEON(b, dst[:], src[:], ivlo, ivhi)
}

// ctrBlocksNEON is ctrBlocks for two to eight blocks. The NEON code always
// encrypts eight counters, which costs less than two blocks do in the
// generic code.
func ctrBlocksNEON(b *Block, dst, src []byte, ivlo, ivhi uint64) {
	if !supportsNEON || !b.hasBS {
		ctrBlocks(b, dst, src, ivlo, ivhi)
		return
	}
	var counters [8 * BlockSize]byte
	for i := 0; i < len(counters); i += BlockSize {
		byteorder.BEPutUint64(counters[i:], ivhi)
		byteorder.BEPutUint64(counters[i+8:], ivlo)
		ivlo, ivhi = add128(ivlo, ivhi, 1)
	}
	if len(src) == len(counters) {
		xorKeyStream8NEON(&b.bs[0], b.rounds, (*[8 * BlockSize]byte)(dst), (*[8 * BlockSize]byte)(src), &counters)
		return
	}
	var buf [8 * BlockSize]byte
	copy(buf[:], src)
	xorKeyStream8NEON(&b.bs[0], b.rounds, &buf, &buf, &counters)
	copy(dst, buf[:len(src)])
}
