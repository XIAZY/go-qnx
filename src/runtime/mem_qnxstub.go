// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(dragonfly || freebsd || netbsd || openbsd || qnx || solaris)

package runtime

import "unsafe"

// qnxSysUsed is only used on qnx (see mem_bsd.go).
func qnxSysUsed(v unsafe.Pointer, n uintptr, released bool) {}
