// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sysrand

// QNX 6.5 has neither getrandom nor arc4random. /dev/urandom is served
// by the random resource manager.
func read(b []byte) error {
	return urandomRead(b)
}
