// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !purego

#include "textflag.h"

// SHA-256 block routine. See sha256block.go for the Go equivalent.
// sha256block_neon_arm.s has the one for a CPU with NEON.
//
// The eight working variables stay in R1-R8 and change places from one
// round to the next by the order in which the round macros are given
// them, not by moves. The message schedule is a ring of sixteen words on
// the stack. R0, R12 and R14 are scratch; R9, R10 and R11 are not used.

// The frame: w[16], then the data pointer and the end of the data.
#define W(i)	(4+4*((i)&15))(R13)
#define p_data	68(R13)
#define p_end	72(R13)

// w[i] = p[0]<<24 | p[1]<<16 | p[2]<<8 | p[3], and p += 4, with p in R0.
// ARMv7 reads a word at any address; older processors may not.
#ifdef GOARM_7
#define LOAD(i) \
	MOVW.P	4(R0), R12; \
	REV	R12, R12; \
	MOVW	R12, W(i)
#else
#define LOAD(i) \
	MOVBU	3(R0), R12; \
	MOVBU	2(R0), R14; \
	ORR	R14<<8, R12, R12; \
	MOVBU	1(R0), R14; \
	ORR	R14<<16, R12, R12; \
	MOVBU.P	4(R0), R14; \
	ORR	R14<<24, R12, R12; \
	MOVW	R12, W(i)
#endif

// w[i] = sigma1(w[i-2]) + w[i-7] + sigma0(w[i-15]) + w[i-16], left in R0:
//	sigma0(x) = x>>>7 ^ x>>>18 ^ x>>3
//	sigma1(x) = x>>>17 ^ x>>>19 ^ x>>10
#define SCHED(i) \
	MOVW	W(i-15), R0; \
	MOVW	W(i-2), R12; \
	MOVW	R0@>7, R14; \
	EOR	R0@>18, R14; \
	EOR	R0>>3, R14; \
	MOVW	R12@>17, R0; \
	EOR	R12@>19, R0; \
	EOR	R12>>10, R0; \
	MOVW	W(i-16), R12; \
	ADD	R14, R0; \
	MOVW	W(i-7), R14; \
	ADD	R12, R0; \
	ADD	R14, R0; \
	MOVW	R0, W(i)

// With w[i] in R0:
//	h += w[i] + k + Sigma1(e) + Ch(e, f, g)
//	d += h
//	h += Sigma0(a) + Maj(a, b, c)
// after which h is the next round's a and d its e.
//	Sigma1(e) = e>>>6 ^ e>>>11 ^ e>>>25 = (e ^ e>>>5 ^ e>>>19)>>>6
//	Sigma0(a) = a>>>2 ^ a>>>13 ^ a>>>22 = (a ^ a>>>11 ^ a>>>20)>>>2
//	Ch(e, f, g) = ((f ^ g) & e) ^ g
//	Maj(a, b, c) = ((a ^ b) & (b ^ c)) ^ b
#define ROUND(k, a, b, c, d, e, f, g, h) \
	MOVW	$k, R12; \
	ADD	R0, h; \
	ADD	R12, h; \
	EOR	e@>5, e, R0; \
	EOR	e@>19, R0; \
	EOR	f, g, R12; \
	ADD	R0@>6, h; \
	AND	e, R12; \
	EOR	g, R12; \
	ADD	R12, h; \
	EOR	a@>11, a, R0; \
	ADD	h, d; \
	EOR	a@>20, R0; \
	EOR	a, b, R12; \
	EOR	b, c, R14; \
	ADD	R0@>2, h; \
	AND	R14, R12; \
	EOR	b, R12; \
	ADD	R12, h

#define ROUND_00_15(i, k, a, b, c, d, e, f, g, h) \
	MOVW	W(i), R0; \
	ROUND(k, a, b, c, d, e, f, g, h)

#define ROUND_16_63(i, k, a, b, c, d, e, f, g, h) \
	SCHED(i); \
	ROUND(k, a, b, c, d, e, f, g, h)

// func blockARM(dig *Digest, p []byte)
TEXT ·blockARM(SB), 0, $72-16
	MOVW	p_base+4(FP), R0
	MOVW	p_len+8(FP), R12
	BIC	$63, R12
	ADD	R0, R12
	CMP	R0, R12
	BEQ	done
	MOVW	R12, p_end

	MOVW	dig+0(FP), R14
	MOVW	0(R14), R1
	MOVW	4(R14), R2
	MOVW	8(R14), R3
	MOVW	12(R14), R4
	MOVW	16(R14), R5
	MOVW	20(R14), R6
	MOVW	24(R14), R7
	MOVW	28(R14), R8

loop:
	LOAD(0)
	LOAD(1)
	LOAD(2)
	LOAD(3)
	LOAD(4)
	LOAD(5)
	LOAD(6)
	LOAD(7)
	LOAD(8)
	LOAD(9)
	LOAD(10)
	LOAD(11)
	LOAD(12)
	LOAD(13)
	LOAD(14)
	LOAD(15)
	MOVW	R0, p_data

	ROUND_00_15(0, 0x428a2f98, R1, R2, R3, R4, R5, R6, R7, R8)
	ROUND_00_15(1, 0x71374491, R8, R1, R2, R3, R4, R5, R6, R7)
	ROUND_00_15(2, 0xb5c0fbcf, R7, R8, R1, R2, R3, R4, R5, R6)
	ROUND_00_15(3, 0xe9b5dba5, R6, R7, R8, R1, R2, R3, R4, R5)
	ROUND_00_15(4, 0x3956c25b, R5, R6, R7, R8, R1, R2, R3, R4)
	ROUND_00_15(5, 0x59f111f1, R4, R5, R6, R7, R8, R1, R2, R3)
	ROUND_00_15(6, 0x923f82a4, R3, R4, R5, R6, R7, R8, R1, R2)
	ROUND_00_15(7, 0xab1c5ed5, R2, R3, R4, R5, R6, R7, R8, R1)
	ROUND_00_15(8, 0xd807aa98, R1, R2, R3, R4, R5, R6, R7, R8)
	ROUND_00_15(9, 0x12835b01, R8, R1, R2, R3, R4, R5, R6, R7)
	ROUND_00_15(10, 0x243185be, R7, R8, R1, R2, R3, R4, R5, R6)
	ROUND_00_15(11, 0x550c7dc3, R6, R7, R8, R1, R2, R3, R4, R5)
	ROUND_00_15(12, 0x72be5d74, R5, R6, R7, R8, R1, R2, R3, R4)
	ROUND_00_15(13, 0x80deb1fe, R4, R5, R6, R7, R8, R1, R2, R3)
	ROUND_00_15(14, 0x9bdc06a7, R3, R4, R5, R6, R7, R8, R1, R2)
	ROUND_00_15(15, 0xc19bf174, R2, R3, R4, R5, R6, R7, R8, R1)
	ROUND_16_63(16, 0xe49b69c1, R1, R2, R3, R4, R5, R6, R7, R8)
	ROUND_16_63(17, 0xefbe4786, R8, R1, R2, R3, R4, R5, R6, R7)
	ROUND_16_63(18, 0x0fc19dc6, R7, R8, R1, R2, R3, R4, R5, R6)
	ROUND_16_63(19, 0x240ca1cc, R6, R7, R8, R1, R2, R3, R4, R5)
	ROUND_16_63(20, 0x2de92c6f, R5, R6, R7, R8, R1, R2, R3, R4)
	ROUND_16_63(21, 0x4a7484aa, R4, R5, R6, R7, R8, R1, R2, R3)
	ROUND_16_63(22, 0x5cb0a9dc, R3, R4, R5, R6, R7, R8, R1, R2)
	ROUND_16_63(23, 0x76f988da, R2, R3, R4, R5, R6, R7, R8, R1)
	ROUND_16_63(24, 0x983e5152, R1, R2, R3, R4, R5, R6, R7, R8)
	ROUND_16_63(25, 0xa831c66d, R8, R1, R2, R3, R4, R5, R6, R7)
	ROUND_16_63(26, 0xb00327c8, R7, R8, R1, R2, R3, R4, R5, R6)
	ROUND_16_63(27, 0xbf597fc7, R6, R7, R8, R1, R2, R3, R4, R5)
	ROUND_16_63(28, 0xc6e00bf3, R5, R6, R7, R8, R1, R2, R3, R4)
	ROUND_16_63(29, 0xd5a79147, R4, R5, R6, R7, R8, R1, R2, R3)
	ROUND_16_63(30, 0x06ca6351, R3, R4, R5, R6, R7, R8, R1, R2)
	ROUND_16_63(31, 0x14292967, R2, R3, R4, R5, R6, R7, R8, R1)
	ROUND_16_63(32, 0x27b70a85, R1, R2, R3, R4, R5, R6, R7, R8)
	ROUND_16_63(33, 0x2e1b2138, R8, R1, R2, R3, R4, R5, R6, R7)
	ROUND_16_63(34, 0x4d2c6dfc, R7, R8, R1, R2, R3, R4, R5, R6)
	ROUND_16_63(35, 0x53380d13, R6, R7, R8, R1, R2, R3, R4, R5)
	ROUND_16_63(36, 0x650a7354, R5, R6, R7, R8, R1, R2, R3, R4)
	ROUND_16_63(37, 0x766a0abb, R4, R5, R6, R7, R8, R1, R2, R3)
	ROUND_16_63(38, 0x81c2c92e, R3, R4, R5, R6, R7, R8, R1, R2)
	ROUND_16_63(39, 0x92722c85, R2, R3, R4, R5, R6, R7, R8, R1)
	ROUND_16_63(40, 0xa2bfe8a1, R1, R2, R3, R4, R5, R6, R7, R8)
	ROUND_16_63(41, 0xa81a664b, R8, R1, R2, R3, R4, R5, R6, R7)
	ROUND_16_63(42, 0xc24b8b70, R7, R8, R1, R2, R3, R4, R5, R6)
	ROUND_16_63(43, 0xc76c51a3, R6, R7, R8, R1, R2, R3, R4, R5)
	ROUND_16_63(44, 0xd192e819, R5, R6, R7, R8, R1, R2, R3, R4)
	ROUND_16_63(45, 0xd6990624, R4, R5, R6, R7, R8, R1, R2, R3)
	ROUND_16_63(46, 0xf40e3585, R3, R4, R5, R6, R7, R8, R1, R2)
	ROUND_16_63(47, 0x106aa070, R2, R3, R4, R5, R6, R7, R8, R1)
	ROUND_16_63(48, 0x19a4c116, R1, R2, R3, R4, R5, R6, R7, R8)
	ROUND_16_63(49, 0x1e376c08, R8, R1, R2, R3, R4, R5, R6, R7)
	ROUND_16_63(50, 0x2748774c, R7, R8, R1, R2, R3, R4, R5, R6)
	ROUND_16_63(51, 0x34b0bcb5, R6, R7, R8, R1, R2, R3, R4, R5)
	ROUND_16_63(52, 0x391c0cb3, R5, R6, R7, R8, R1, R2, R3, R4)
	ROUND_16_63(53, 0x4ed8aa4a, R4, R5, R6, R7, R8, R1, R2, R3)
	ROUND_16_63(54, 0x5b9cca4f, R3, R4, R5, R6, R7, R8, R1, R2)
	ROUND_16_63(55, 0x682e6ff3, R2, R3, R4, R5, R6, R7, R8, R1)
	ROUND_16_63(56, 0x748f82ee, R1, R2, R3, R4, R5, R6, R7, R8)
	ROUND_16_63(57, 0x78a5636f, R8, R1, R2, R3, R4, R5, R6, R7)
	ROUND_16_63(58, 0x84c87814, R7, R8, R1, R2, R3, R4, R5, R6)
	ROUND_16_63(59, 0x8cc70208, R6, R7, R8, R1, R2, R3, R4, R5)
	ROUND_16_63(60, 0x90befffa, R5, R6, R7, R8, R1, R2, R3, R4)
	ROUND_16_63(61, 0xa4506ceb, R4, R5, R6, R7, R8, R1, R2, R3)
	ROUND_16_63(62, 0xbef9a3f7, R3, R4, R5, R6, R7, R8, R1, R2)
	ROUND_16_63(63, 0xc67178f2, R2, R3, R4, R5, R6, R7, R8, R1)

	MOVW	dig+0(FP), R14
	MOVW	0(R14), R0
	ADD	R0, R1
	MOVW	R1, 0(R14)
	MOVW	4(R14), R0
	ADD	R0, R2
	MOVW	R2, 4(R14)
	MOVW	8(R14), R0
	ADD	R0, R3
	MOVW	R3, 8(R14)
	MOVW	12(R14), R0
	ADD	R0, R4
	MOVW	R4, 12(R14)
	MOVW	16(R14), R0
	ADD	R0, R5
	MOVW	R5, 16(R14)
	MOVW	20(R14), R0
	ADD	R0, R6
	MOVW	R6, 20(R14)
	MOVW	24(R14), R0
	ADD	R0, R7
	MOVW	R7, 24(R14)
	MOVW	28(R14), R0
	ADD	R0, R8
	MOVW	R8, 28(R14)

	MOVW	p_data, R0
	MOVW	p_end, R12
	CMP	R0, R12
	BNE	loop

done:
	RET
