// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !purego

package sha256

import (
	"crypto/internal/fips140deps/cpu"
	"crypto/internal/impl"
)

var useNEON = cpu.ARMHasNEON

func init() {
	impl.Register("sha256", "NEON", &useNEON)
}

// blockARM is blockGeneric in assembly, for any ARM processor.
//
//go:noescape
func blockARM(dig *Digest, p []byte)

// blockNEON is blockARM with the message schedule in NEON.
//
//go:noescape
func blockNEON(dig *Digest, p []byte)

func block(dig *Digest, p []byte) {
	if useNEON {
		blockNEON(dig, p)
	} else {
		blockARM(dig, p)
	}
}
