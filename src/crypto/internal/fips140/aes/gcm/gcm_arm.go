// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !purego

package gcm

import (
	"crypto/internal/fips140/aes"
	"crypto/internal/fips140/subtle"
	"crypto/internal/fips140deps/byteorder"
	"crypto/internal/fips140deps/cpu"
	"crypto/internal/impl"
)

// The functions below are implemented in gcm_arm.s. They need NEON, which
// an ARMv7 CPU may lack.

// gcmInitNEON sets htable to the form of the hash key that gcmGhashNEON
// takes. h is the hash key as two big-endian 64-bit halves.
//
//go:noescape
func gcmInitNEON(htable *[16]byte, h *[2]uint64)

// gcmGhashNEON adds data to the GHASH state xi. The length of data must be
// a multiple of the block size and not zero.
//
//go:noescape
func gcmGhashNEON(xi, htable *[16]byte, data []byte)

var supportsNEON = cpu.ARMHasNEON

func init() {
	impl.Register("gcm", "NEON", &supportsNEON)
}

func checkGenericIsExpected() {
	if supportsNEON {
		panic("gcm: internal error: using generic implementation despite hardware support")
	}
}

type gcmPlatformData struct {
	htable [16]byte
}

// chunk is what the NEON code of package aes encrypts at a time.
const chunk = 8 * gcmBlockSize

// With NEON, no block goes through the generic AES code, which is not
// constant-time: the hash key and the tag mask are encrypted by the
// bit-sliced code as well.

func initGCM(g *GCM) {
	if !supportsNEON {
		return
	}
	var zeros, ks [chunk]byte
	aes.XORKeyStream8(&g.cipher, &ks, &zeros, &zeros)
	h := [2]uint64{byteorder.BEUint64(ks[:8]), byteorder.BEUint64(ks[8:])}
	gcmInitNEON(&g.htable, &h)
}

func seal(out []byte, g *GCM, nonce, plaintext, data []byte) {
	if !supportsNEON {
		sealGeneric(out, g, nonce, plaintext, data)
		return
	}

	var counter, tagMask [gcmBlockSize]byte
	deriveCounterNEON(g, &counter, nonce)

	// The tag mask is the key stream's first block, so it is encrypted
	// together with the first seven blocks of the plaintext.
	var first [chunk]byte
	n := copy(first[gcmBlockSize:], plaintext)
	gcmCounterCryptNEON(&g.cipher, first[:gcmBlockSize+n], first[:gcmBlockSize+n], &counter)
	copy(tagMask[:], first[:])
	copy(out, first[gcmBlockSize:gcmBlockSize+n])
	gcmCounterCryptNEON(&g.cipher, out[n:], plaintext[n:], &counter)

	var tag [gcmTagSize]byte
	gcmAuthNEON(&tag, g, &tagMask, out[:len(plaintext)], data)
	copy(out[len(plaintext):], tag[:])
}

func open(out []byte, g *GCM, nonce, ciphertext, data []byte) error {
	if !supportsNEON {
		return openGeneric(out, g, nonce, ciphertext, data)
	}

	tag := ciphertext[len(ciphertext)-g.tagSize:]
	ciphertext = ciphertext[:len(ciphertext)-g.tagSize]

	var counter, tagMask [gcmBlockSize]byte
	deriveCounterNEON(g, &counter, nonce)

	// As in seal. The first seven blocks are decrypted here, and reach
	// out only once the tag has been checked.
	var first [chunk]byte
	n := copy(first[gcmBlockSize:], ciphertext)
	gcmCounterCryptNEON(&g.cipher, first[:gcmBlockSize+n], first[:gcmBlockSize+n], &counter)
	copy(tagMask[:], first[:])

	var expectedTag [gcmTagSize]byte
	gcmAuthNEON(&expectedTag, g, &tagMask, ciphertext, data)
	if subtle.ConstantTimeCompare(expectedTag[:g.tagSize], tag) != 1 {
		return errOpen
	}

	copy(out, first[gcmBlockSize:gcmBlockSize+n])
	gcmCounterCryptNEON(&g.cipher, out[n:], ciphertext[n:], &counter)

	return nil
}

func deriveCounterNEON(g *GCM, counter *[gcmBlockSize]byte, nonce []byte) {
	if len(nonce) == gcmStandardNonceSize {
		copy(counter[:], nonce)
		counter[gcmBlockSize-1] = 1
		return
	}
	var lenBlock [gcmBlockSize]byte
	byteorder.BEPutUint64(lenBlock[8:], uint64(len(nonce))*8)
	ghashUpdateNEON(counter, &g.htable, nonce)
	gcmGhashNEON(counter, &g.htable, lenBlock[:])
}

func gcmAuthNEON(out *[gcmTagSize]byte, g *GCM, tagMask *[gcmBlockSize]byte, ciphertext, additionalData []byte) {
	var lenBlock, S [gcmBlockSize]byte
	byteorder.BEPutUint64(lenBlock[:8], uint64(len(additionalData))*8)
	byteorder.BEPutUint64(lenBlock[8:], uint64(len(ciphertext))*8)
	ghashUpdateNEON(&S, &g.htable, additionalData)
	ghashUpdateNEON(&S, &g.htable, ciphertext)
	gcmGhashNEON(&S, &g.htable, lenBlock[:])
	subtle.XORBytes(out[:], S[:], tagMask[:])
}

// ghashUpdateNEON adds data, padded with zeros to a whole number of
// blocks, to the GHASH state xi.
func ghashUpdateNEON(xi, htable *[16]byte, data []byte) {
	if n := len(data) &^ (gcmBlockSize - 1); n > 0 {
		gcmGhashNEON(xi, htable, data[:n])
		data = data[n:]
	}
	if len(data) > 0 {
		var last [gcmBlockSize]byte
		copy(last[:], data)
		gcmGhashNEON(xi, htable, last[:])
	}
}

// gcmCounterCryptNEON is gcmCounterCryptGeneric, eight blocks at a time.
func gcmCounterCryptNEON(b *aes.Block, out, src []byte, counter *[gcmBlockSize]byte) {
	if len(src) == 0 {
		return
	}

	var counters [chunk]byte
	for i := 0; i < chunk; i += gcmBlockSize {
		copy(counters[i:], counter[:])
	}
	ctr := byteorder.BEUint32(counter[gcmBlockSize-4:])
	next := func() {
		for i := gcmBlockSize - 4; i < chunk; i += gcmBlockSize {
			byteorder.BEPutUint32(counters[i:], ctr)
			ctr++
		}
	}

	for len(src) >= chunk {
		next()
		aes.XORKeyStream8(b, (*[chunk]byte)(out), (*[chunk]byte)(src), &counters)
		out = out[chunk:]
		src = src[chunk:]
	}
	if len(src) > 0 {
		// The counters past the end of src are encrypted and not used.
		ctr0 := ctr
		next()
		ctr = ctr0 + uint32((len(src)+gcmBlockSize-1)/gcmBlockSize)
		var buf [chunk]byte
		copy(buf[:], src)
		aes.XORKeyStream8(b, &buf, &buf, &counters)
		copy(out, buf[:len(src)])
	}
	byteorder.BEPutUint32(counter[gcmBlockSize-4:], ctr)
}
