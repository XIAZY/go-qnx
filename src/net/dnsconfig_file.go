// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows && !qnx

package net

func openResolvConf(name string) (*file, string, error) {
	f, err := open(name)
	return f, "", err
}
