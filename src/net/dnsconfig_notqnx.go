// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !qnx

package net

// qnxResolvConfChanged is only called on qnx; see dnsconfig_qnx.go.
func qnxResolvConfChanged(name string, dc *dnsConfig) bool { return true }
