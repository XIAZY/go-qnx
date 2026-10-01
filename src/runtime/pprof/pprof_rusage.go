// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package pprof

import (
	"fmt"
	"io"
	"runtime"
	"syscall"
)

// Adds MaxRSS to platforms that are supported.
func addMaxRSS(w io.Writer) {
	var rssToBytes uintptr
	switch runtime.GOOS {
	case "aix", "android", "dragonfly", "freebsd", "linux", "netbsd", "openbsd":
		rssToBytes = 1024
	case "darwin", "ios":
		rssToBytes = 1
	case "illumos", "solaris":
		rssToBytes = uintptr(syscall.Getpagesize())
	case "qnx":
		// QNX 6.5's getrusage leaves ru_maxrss 0: a C program read 0
		// even after touching 20 MiB. Leave the line out rather
		// than report a false 0.
		return
	default:
		panic("unsupported OS")
	}

	var rusage syscall.Rusage
	err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage)
	if err == nil {
		fmt.Fprintf(w, "# MaxRSS = %d\n", uintptr(rusage.Maxrss)*rssToBytes)
	}
}
