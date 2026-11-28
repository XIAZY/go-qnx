// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !purego

package sha512

import (
	"crypto/internal/fips140deps/cpu"
	"crypto/internal/impl"
)

var useNEON = cpu.ARMHasNEON

func init() {
	impl.Register("sha512", "NEON", &useNEON)
}

// blockNEON is blockGeneric with NEON, in sha512block_arm.s.
//
//go:noescape
func blockNEON(dig *Digest, p []byte)

func block(dig *Digest, p []byte) {
	if useNEON {
		blockNEON(dig, p)
	} else {
		blockGeneric(dig, p)
	}
}
