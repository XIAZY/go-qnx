// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix && !qnx

package runtime

// sigDelayedFault is only needed on qnx (see os_qnx.go).
//
//go:nosplit
//go:nowritebarrierrec
func sigDelayedFault(sig uint32, c *sigctxt, gp *g) bool {
	return false
}
