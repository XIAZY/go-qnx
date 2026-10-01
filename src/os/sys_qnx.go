// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import "syscall"

func hostname() (name string, err error) {
	var un syscall.Utsname
	if err := syscall.Uname(&un); err != nil {
		return "", NewSyscallError("uname", err)
	}
	var buf [len(un.Nodename)]byte
	n := 0
	for i, b := range un.Nodename[:] {
		if b == 0 {
			break
		}
		buf[i] = byte(b)
		n++
	}
	return string(buf[:n]), nil
}
