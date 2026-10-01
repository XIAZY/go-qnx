// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cgotest

import "testing"

func testSigaltstack(t *testing.T) {
	t.Skip("QNX 6.5 has no sigaltstack")
}
