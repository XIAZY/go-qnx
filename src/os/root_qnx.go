// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is root_js.go for QNX 6.5, which also has no openat: Root is
// implemented by resolving each path under the root with Lstat and
// Readlink, with the TOCTOU caveat noted below.

package os

import (
	"errors"
	"slices"
	"syscall"
)

// QNX resolves ".." in a path lexically, before following symlinks: with
// a => b/c, "a/../x" is "x", not "b/x". So a name checked by walking it
// component by component, as below, must not be handed to the kernel as
// it is: the kernel could resolve it to another file, even outside the
// root ("a/../../x" is outside, while the walk finds root/x). rootPath
// returns the path the walk resolved, with no symlink and no ".." left
// in its directories, which the kernel resolves the same way.
//
// Operations that follow a symlink in the final component use rootPath,
// which resolves it too: OpenFile, OpenRoot, Stat, Chmod, Chown and
// Chtimes. Operations on the final component itself use rootPathLstat,
// which leaves it as it is: Lstat, Readlink, Remove, RemoveAll, Rename,
// Link, Symlink, Lchown, Mkdir and MkdirAll.
//
// rootPathLstat leaves a final "." for the kernel, and QNX's rename accepts
// it: Rename("d/.", n) renames d, where Linux fails with EBUSY. That stays
// inside the root, as no ".." reaches the kernel. Rename(".", n) fails, as
// the root cannot move into itself.
//
// Due to the lack of openat, this is subject to TOCTOU races when
// symlinks change during the resolution process.

// rootPath checks that name does not escape the root, and returns the
// path to use for it.
func rootPath(r *Root, name string) (string, error) {
	return resolveInRoot(r, name, false)
}

// rootPathLstat is rootPath, without resolving a symlink in the final
// path component.
func rootPathLstat(r *Root, name string) (string, error) {
	return resolveInRoot(r, name, true)
}

// rootOpenPath is rootPath for OpenFile with flag. O_CREATE|O_EXCL never
// follows a symlink in the final component.
func rootOpenPath(r *Root, name string, flag int) (string, error) {
	if flag&(O_CREATE|O_EXCL) == O_CREATE|O_EXCL {
		return rootPathLstat(r, name)
	}
	return rootPath(r, name)
}

// rootMkdirAllRawFallback reports whether MkdirAll may use name as it is
// after checking it failed (with ENOTDIR, say), so that MkdirAll reports
// the error with its exact path. Only without "..": the kernel follows
// the same symlinks as the walk, but resolves ".." differently.
func rootMkdirAllRawFallback(name string) bool {
	parts, _, err := splitPathInRoot(name, nil, nil)
	if err != nil {
		return false
	}
	for _, p := range parts {
		if p == ".." {
			return false
		}
	}
	return true
}

// rootMkdirAllPath returns the path for MkdirAll. A symlink as the final
// component is followed, as os.MkdirAll does, if it leads to a directory
// inside the root (MkdirAll then has nothing to do); a dangling one is
// an error, rather than a directory made at its target.
func rootMkdirAllPath(r *Root, name string) (string, error) {
	lpath, err := rootPathLstat(r, name)
	if err != nil {
		return "", err
	}
	if fi, err := Lstat(lpath); err != nil || fi.Mode()&ModeSymlink == 0 {
		return lpath, nil
	}
	path, err := rootPath(r, name)
	if err != nil {
		return "", err
	}
	if fi, err := Stat(path); err != nil || !fi.IsDir() {
		return "", syscall.EEXIST
	}
	return path, nil
}

func checkPathEscapes(r *Root, name string) error {
	_, err := resolveInRoot(r, name, false)
	return err
}

func checkPathEscapesLstat(r *Root, name string) error {
	_, err := resolveInRoot(r, name, true)
	return err
}

func resolveInRoot(r *Root, name string, lstat bool) (string, error) {
	if r.root.closed.Load() {
		return "", ErrClosed
	}
	parts, endsInSlash, err := splitPathInRoot(name, nil, nil)
	if err != nil {
		return "", err
	}

	i := 0
	symlinks := 0
	base := r.root.name
	for i < len(parts) {
		if parts[i] == ".." {
			// Resolve one or more parent ("..") path components.
			end := i + 1
			for end < len(parts) && parts[end] == ".." {
				end++
			}
			count := end - i
			if count > i {
				return "", errPathEscapes
			}
			parts = slices.Delete(parts, i-count, end)
			i -= count
			base = r.root.name
			for j := range i {
				base = joinPath(base, parts[j])
			}
			continue
		}

		part := parts[i]
		if i == len(parts)-1 {
			if lstat && !endsInSlash {
				return withSlash(joinPath(base, part), endsInSlash), nil
			}
		}

		next := joinPath(base, part)
		fi, err := Lstat(next)
		if err != nil {
			if IsNotExist(err) {
				// The rest cannot be resolved. A ".." in it would
				// take the kernel back out of this missing
				// directory (maybe out of the root) where the
				// walk cannot follow: fail as a physical walk
				// would.
				for _, p := range parts[i+1:] {
					if p == ".." {
						return "", syscall.ENOENT
					}
				}
				for _, p := range parts[i+1:] {
					next = joinPath(next, p)
				}
				return withSlash(next, endsInSlash), nil
			}
			return "", underlyingError(err)
		}
		if fi.Mode()&ModeSymlink != 0 {
			link, err := Readlink(next)
			if err != nil {
				return "", errPathEscapes
			}
			symlinks++
			if symlinks > rootMaxSymlinks {
				return "", errors.New("too many symlinks")
			}
			newparts, newEndsInSlash, err := splitPathInRoot(link, parts[:i], parts[i+1:])
			if err != nil {
				return "", err
			}
			if i == len(parts)-1 && newEndsInSlash {
				endsInSlash = true
			}
			parts = newparts
			continue
		}
		if !fi.IsDir() && i < len(parts)-1 {
			return "", syscall.ENOTDIR
		}

		base = next
		i++
	}
	return withSlash(base, endsInSlash), nil
}

// withSlash returns path, with a trailing slash if slash is set, so that
// the kernel still requires a directory there.
func withSlash(path string, slash bool) string {
	if slash && !IsPathSeparator(path[len(path)-1]) {
		return path + "/"
	}
	return path
}
