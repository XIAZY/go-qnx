// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// QNX 6.5 has no issetugid. As on AIX, secure mode means the real and
// effective IDs differ.

var secureMode bool

func initSecureMode() {
	secureMode = !(getuid() == geteuid() && getgid() == getegid())
}

func isSecureMode() bool {
	return secureMode
}
