// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build gc && !purego

package poly1305

import (
	"internal/cpu"
	"math/bits"
)

// With NEON, whole pairs of blocks go through sum_arm.s. This file is not
// in golang.org/x/crypto: it was added to the copy here.

// updatePairsNEON does, for each pair of 16-byte blocks of msg, h = (h +
// block) * r, in each of two lanes at once: the first block of a pair is
// for the first lane and the second for the second. h and r are in limbs
// of 26 bits, and r[5:] is five times r[1:5]. The limbs of h may come
// back a little over 26 bits. pairs must not be 0.
//
//go:noescape
func updatePairsNEON(h *[5][2]uint32, r *[9][2]uint32, msg *byte, pairs int)

// neonMin is the least input that goes through the NEON code, which first
// has to square r and change the form of h. On a BlackBerry Q10 four
// blocks take 1.3 µs with it and 1.4 without.
//
// ponytail: r is squared again on every call; keep the square in the mac
// if many calls turn out to be this short.
const neonMin = 4 * TagSize

// mac is macGeneric with update in place of updateGeneric, as in sum_asm.go.
type mac struct{ macGeneric }

func (h *mac) Write(p []byte) (int, error) {
	nn := len(p)
	if h.offset > 0 {
		n := copy(h.buffer[h.offset:], p)
		if h.offset+n < TagSize {
			h.offset += n
			return nn, nil
		}
		p = p[n:]
		h.offset = 0
		update(&h.macState, h.buffer[:])
	}
	if n := len(p) - (len(p) % TagSize); n > 0 {
		update(&h.macState, p[:n])
		p = p[n:]
	}
	if len(p) > 0 {
		h.offset += copy(h.buffer[h.offset:], p)
	}
	return nn, nil
}

func (h *mac) Sum(out *[16]byte) {
	state := h.macState
	if h.offset > 0 {
		update(&state, h.buffer[:h.offset])
	}
	finalize(out, &state.h, &state.s)
}

func update(state *macState, msg []byte) {
	if cpu.ARM.HasNEON && len(msg) >= neonMin {
		n := len(msg) &^ (2*TagSize - 1)
		updateNEON(state, msg[:n])
		msg = msg[n:]
	}
	if len(msg) > 0 {
		updateGeneric(state, msg)
	}
}

// limbs splits lo + hi<<64 + top<<128 into five limbs of 26 bits, the last
// of which takes what is left.
func limbs(lo, hi, top uint64) [5]uint32 {
	const mask = 1<<26 - 1
	return [5]uint32{
		uint32(lo) & mask,
		uint32(lo>>26) & mask,
		uint32(lo>>52|hi<<12) & mask,
		uint32(hi>>14) & mask,
		uint32(hi>>40 | top<<24),
	}
}

// updateNEON is updateGeneric for a msg of whole pairs of blocks.
//
// With the blocks m[0] to m[2k-1], one lane starts at h + m[0] and the other
// at m[1]. Each is multiplied by r squared and takes the next block of its
// own, k-1 times. Then the first is multiplied by r squared and the second
// by r, and their sum is the new h.
func updateNEON(state *macState, msg []byte) {
	// r squared comes from the generic code, which multiplies h + 2^128
	// by r: h is set to r - 2^128, which is r + 3*2^128 - 5 mod 2^130 - 5.
	sq := macState{r: state.r}
	var b uint64
	sq.h[0], b = bits.Sub64(state.r[0], 5, 0)
	sq.h[1], b = bits.Sub64(state.r[1], 0, b)
	sq.h[2] = 3 - b
	var zero [TagSize]byte
	updateGeneric(&sq, zero[:])
	r1 := limbs(state.r[0], state.r[1], 0)
	r2 := limbs(sq.h[0], sq.h[1], sq.h[2])

	// The multipliers: r squared in both lanes, and r squared and r.
	var both, last [9][2]uint32
	for i := range 5 {
		both[i] = [2]uint32{r2[i], r2[i]}
		last[i] = [2]uint32{r2[i], r1[i]}
	}
	for i := 1; i < 5; i++ {
		both[4+i] = [2]uint32{5 * r2[i], 5 * r2[i]}
		last[4+i] = [2]uint32{5 * r2[i], 5 * r1[i]}
	}

	var h [5][2]uint32
	for i, l := range limbs(state.h[0], state.h[1], state.h[2]) {
		h[i][0] = l
	}
	pairs := len(msg) / (2 * TagSize)
	if pairs > 1 {
		updatePairsNEON(&h, &both, &msg[0], pairs-1)
	}
	updatePairsNEON(&h, &last, &msg[(pairs-1)*2*TagSize], 1)

	// Add the lanes and go back to limbs of 64 bits.
	var t [5]uint64
	for i := range t {
		t[i] = uint64(h[i][0]) + uint64(h[i][1])
	}
	h0, c := bits.Add64(t[0]+t[1]<<26, t[2]<<52, 0)
	h1, c := bits.Add64(t[2]>>12+t[3]<<14, t[4]<<40, c)
	h2 := t[4]>>24 + c
	// What is above 2^130 comes back in five times over, which leaves
	// h2 at most 4, as updateGeneric does.
	h0, c = bits.Add64(h0, (h2>>2)*5, 0)
	h1, c = bits.Add64(h1, 0, c)
	state.h = [3]uint64{h0, h1, h2&3 + c}
}
