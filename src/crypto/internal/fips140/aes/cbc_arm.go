// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !purego

package aes

// cbcDecrypt8NEON decrypts the eight blocks of src into dst, in CBC mode
// after iv, and sets iv to the last block of src. dst and src may be the
// same memory. bs is a bit-sliced schedule of the decryption round keys.
//
//go:noescape
func cbcDecrypt8NEON(bs *byte, rounds int, dst, src *[8 * BlockSize]byte, iv *[BlockSize]byte)

// cbcDecryptMinNEON is the least input that goes through the NEON code,
// which always decrypts eight blocks and first has to bit-slice the key
// schedule. On a BlackBerry Q10 four blocks take 3.0 µs with it and 3.5
// without, and three would take longer with it.
const cbcDecryptMinNEON = 4 * BlockSize

// Encryption chains from one block to the next, so it cannot be done eight
// blocks at a time.
func cryptBlocksEnc(b *Block, civ *[BlockSize]byte, dst, src []byte) {
	cryptBlocksEncGeneric(b, civ, dst, src)
}

func cryptBlocksDec(b *Block, civ *[BlockSize]byte, dst, src []byte) {
	if !supportsNEON || len(src) < cbcDecryptMinNEON {
		cryptBlocksDecGeneric(b, civ, dst, src)
		return
	}

	// The schedule is made for each call, as OpenSSL does, instead of
	// kept in every Block: it costs much less than eight blocks do.
	// bsaesKeyConvertNEON leaves 0x63 in each byte of the last round
	// key, which is where encryption wants it; decryption wants it in
	// the first.
	var bs [bsKeySize]byte
	bsaesKeyConvertNEON(&bs[0], &b.dec[0], b.rounds)
	last := bs[BlockSize+(b.rounds-1)*8*BlockSize:][:BlockSize]
	for i := range last {
		bs[i] ^= 0x63
		last[i] ^= 0x63
	}

	for len(src) >= 8*BlockSize {
		cbcDecrypt8NEON(&bs[0], b.rounds, (*[8 * BlockSize]byte)(dst), (*[8 * BlockSize]byte)(src), civ)
		dst, src = dst[8*BlockSize:], src[8*BlockSize:]
	}
	if len(src) < cbcDecryptMinNEON {
		if len(src) > 0 {
			cryptBlocksDecGeneric(b, civ, dst, src)
		}
		return
	}
	var buf [8 * BlockSize]byte
	copy(buf[:], src)
	iv := [BlockSize]byte(src[len(src)-BlockSize:])
	cbcDecrypt8NEON(&bs[0], b.rounds, &buf, &buf, civ)
	copy(dst, buf[:len(src)])
	*civ = iv
}
