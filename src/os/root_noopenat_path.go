// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (js && wasm) || plan9

package os

// rootPath checks that name does not escape the root, and returns the
// path to use for it. See root_qnx.go for a system where the path the
// check resolved must be used instead of name.
func rootPath(r *Root, name string) (string, error) {
	if err := checkPathEscapes(r, name); err != nil {
		return "", err
	}
	return joinPath(r.root.name, name), nil
}

// rootMkdirAllRawFallback reports whether MkdirAll may use name as it is
// when checking it failed for a reason other than an escape, so that
// MkdirAll reports the error.
func rootMkdirAllRawFallback(name string) bool {
	return true
}

// rootMkdirAllPath is rootPathLstat for MkdirAll.
func rootMkdirAllPath(r *Root, name string) (string, error) {
	return rootPathLstat(r, name)
}

// rootOpenPath is rootPath for OpenFile with flag.
func rootOpenPath(r *Root, name string, flag int) (string, error) {
	return rootPath(r, name)
}

// rootPathLstat is rootPath, without resolving a symlink in the final
// path component.
func rootPathLstat(r *Root, name string) (string, error) {
	if err := checkPathEscapesLstat(r, name); err != nil {
		return "", err
	}
	return joinPath(r.root.name, name), nil
}
