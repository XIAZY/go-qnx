// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRootQNXLexicalDotDot checks that Root does not escape through ".."
// on QNX, which resolves ".." in a path lexically: with a => b/c, the
// kernel reads "a/../../x" as "x" in the root's parent, while walking the
// name through the symlink ends at root/x. Neither that nor a ".." after
// a missing directory may reach a file next to the root.
func TestRootQNXLexicalDotDot(t *testing.T) {
	dir := t.TempDir()
	rootDir := filepath.Join(dir, "root")
	if err := os.MkdirAll(filepath.Join(rootDir, "b", "c"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("b/c", filepath.Join(rootDir, "a")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "x")
	if err := os.WriteFile(outside, []byte("outside"), 0o666); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := os.WriteFile(filepath.Join(rootDir, "file"), nil, 0o666); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a/../../x", "missing/../../x", "a/missing/../../../x", "file/../../x"} {
		if fi, err := root.Stat(name); err == nil {
			t.Errorf("root.Stat(%q) = %v, want error (a file outside the root)", name, fi.Name())
		}
		if b, err := root.ReadFile(name); err == nil {
			t.Errorf("root.ReadFile(%q) = %q, want error", name, b)
		}
		// Creating is allowed where the walk resolves the name, as
		// long as that is inside the root.
		if f, err := root.Create(name); err == nil {
			f.Close()
			root.Remove(name)
		}
		if err := root.MkdirAll(name, 0o777); err == nil {
			root.Remove(name)
		}
		if _, err := os.Stat(outside); err != nil {
			t.Fatalf("after %q: file outside the root: %v", name, err)
		}
		if fi, err := os.Stat(outside); err == nil && fi.IsDir() {
			t.Fatalf("after %q: a directory was made outside the root", name)
		}
	}
	if b, err := os.ReadFile(outside); err != nil || string(b) != "outside" {
		t.Errorf("file outside the root: %q, %v; want \"outside\"", b, err)
	}

	// The walk's resolution is the one used: a/../x is b/x.
	if err := os.WriteFile(filepath.Join(rootDir, "b", "x"), []byte("b/x"), 0o666); err != nil {
		t.Fatal(err)
	}
	if b, err := root.ReadFile("a/../x"); err != nil || string(b) != "b/x" {
		t.Errorf("root.ReadFile(%q) = %q, %v; want \"b/x\"", "a/../x", b, err)
	}
}
