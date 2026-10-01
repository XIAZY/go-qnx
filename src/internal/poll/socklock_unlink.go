// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !qnx && !plan9

package poll

import "syscall"

// UnlinkSocket removes the name of a Unix socket.
func UnlinkSocket(path string) error {
	return syscall.Unlink(path)
}
