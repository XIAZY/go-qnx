// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"internal/syscall/unix"
	"os"
	"time"
)

// csResolve returns QNX's _CS_RESOLVE; tests replace it.
var csResolve = func() string { return unix.Confstr(unix.CS_RESOLVE) }

// openResolvConf opens the resolver configuration. On QNX the system's
// is normally not /etc/resolv.conf but the configuration string
// _CS_RESOLVE, which QNX's resolver uses instead of the file when it is
// set: resolv.conf with its lines separated by spaces and the spaces
// within each line replaced by underscores, as in
// "nameserver_10.0.2.3 domain_example.com".
func openResolvConf(name string) (*file, string, error) {
	if name == "/etc/resolv.conf" {
		if s := csResolve(); s != "" {
			var data []byte
			for _, line := range getFields(s) {
				for i := 0; i < len(line); i++ {
					c := line[i]
					if c == '_' {
						c = ' '
					}
					data = append(data, c)
				}
				data = append(data, '\n')
			}
			return &file{data: data, atEOF: true}, s, nil
		}
	}
	f, err := open(name)
	return f, "", err
}

// qnxResolvConfChanged reports whether the resolver configuration has
// changed since dc was read. _CS_RESOLVE has no modification time, so
// compare its value; it can change at any time (dhcp.client sets it).
func qnxResolvConfChanged(name string, dc *dnsConfig) bool {
	if name == "/etc/resolv.conf" {
		s := csResolve()
		if s != "" || dc.resolveConf != "" {
			return s != dc.resolveConf
		}
	}
	var mtime time.Time
	if fi, err := os.Stat(name); err == nil {
		mtime = fi.ModTime()
	}
	return !mtime.Equal(dc.mtime)
}
