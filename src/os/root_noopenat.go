// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (js && wasm) || plan9 || qnx

package os

import (
	"errors"
	"internal/filepathlite"
	"internal/stringslite"
	"sync/atomic"
	"syscall"
	"time"
)

// root implementation for platforms with no openat.
// Currently plan9 and js.
type root struct {
	name   string
	closed atomic.Bool
}

// openRootNolog is OpenRoot.
func openRootNolog(name string) (*Root, error) {
	r, err := newRoot(name)
	if err != nil {
		return nil, &PathError{Op: "open", Path: name, Err: err}
	}
	return r, nil
}

// openRootInRoot is Root.OpenRoot.
func openRootInRoot(r *Root, name string) (*Root, error) {
	path, err := rootPath(r, name)
	if err != nil {
		return nil, &PathError{Op: "openat", Path: name, Err: err}
	}
	r, err = newRoot(path)
	if err != nil {
		return nil, &PathError{Op: "openat", Path: name, Err: err}
	}
	return r, nil
}

// newRoot returns a new Root.
// If fd is not a directory, it closes it and returns an error.
func newRoot(name string) (*Root, error) {
	fi, err := Stat(name)
	if err != nil {
		return nil, err.(*PathError).Err
	}
	if !fi.IsDir() {
		return nil, errors.New("not a directory")
	}
	return &Root{&root{name: name}}, nil
}

func (r *root) Close() error {
	// For consistency with platforms where Root.Close closes a handle,
	// mark the Root as closed and return errors from future calls.
	r.closed.Store(true)
	return nil
}

func (r *root) Name() string {
	return r.name
}

// rootOpenFileNolog is Root.OpenFile.
func rootOpenFileNolog(r *Root, name string, flag int, perm FileMode) (*File, error) {
	path, err := rootOpenPath(r, name, flag)
	if err != nil {
		return nil, &PathError{Op: "openat", Path: name, Err: err}
	}
	f, err := openFileNolog(path, flag, perm)
	if err != nil {
		return nil, &PathError{Op: "openat", Path: name, Err: underlyingError(err)}
	}
	return f, nil
}

func rootStat(r *Root, name string, lstat bool) (FileInfo, error) {
	var fi FileInfo
	var path string
	var err error
	if lstat {
		path, err = rootPathLstat(r, name)
		if err == nil {
			fi, err = Lstat(path)
		}
	} else {
		path, err = rootPath(r, name)
		if err == nil {
			fi, err = Stat(path)
		}
	}
	if err != nil {
		return nil, &PathError{Op: "statat", Path: name, Err: underlyingError(err)}
	}
	if fs, ok := fi.(*fileStat); ok {
		// The name of the file as the caller gave it, not of what
		// path resolved to.
		fs.name = filepathlite.Base(joinPath(r.root.name, name))
	}
	return fi, nil
}

func rootChmod(r *Root, name string, mode FileMode) error {
	path, err := rootPath(r, name)
	if err != nil {
		return &PathError{Op: "chmodat", Path: name, Err: err}
	}
	if err := Chmod(path, mode); err != nil {
		return &PathError{Op: "chmodat", Path: name, Err: underlyingError(err)}
	}
	return nil
}

func rootChown(r *Root, name string, uid, gid int) error {
	path, err := rootPath(r, name)
	if err != nil {
		return &PathError{Op: "chownat", Path: name, Err: err}
	}
	if err := Chown(path, uid, gid); err != nil {
		return &PathError{Op: "chownat", Path: name, Err: underlyingError(err)}
	}
	return nil
}

func rootLchown(r *Root, name string, uid, gid int) error {
	path, err := rootPathLstat(r, name)
	if err != nil {
		return &PathError{Op: "lchownat", Path: name, Err: err}
	}
	if err := Lchown(path, uid, gid); err != nil {
		return &PathError{Op: "lchownat", Path: name, Err: underlyingError(err)}
	}
	return nil
}

func rootChtimes(r *Root, name string, atime time.Time, mtime time.Time) error {
	path, err := rootPath(r, name)
	if err != nil {
		return &PathError{Op: "chtimesat", Path: name, Err: err}
	}
	if err := Chtimes(path, atime, mtime); err != nil {
		return &PathError{Op: "chtimesat", Path: name, Err: underlyingError(err)}
	}
	return nil
}

func rootMkdir(r *Root, name string, perm FileMode) error {
	path, err := rootPathLstat(r, name)
	if err != nil {
		return &PathError{Op: "mkdirat", Path: name, Err: err}
	}
	if name == "" {
		return &PathError{Op: "mkdirat", Path: name, Err: syscall.ENOENT}
	}
	if err := Mkdir(path, perm); err != nil {
		return &PathError{Op: "mkdirat", Path: name, Err: underlyingError(err)}
	}
	return nil
}

func rootMkdirAll(r *Root, name string, perm FileMode) error {
	// We only check for errPathEscapes here.
	// For errors such as ENOTDIR (a non-directory file appeared somewhere along the path),
	// we let MkdirAll generate the error.
	// MkdirAll will return a PathError referencing the exact location of the error,
	// and we want to preserve that property.
	path, err := rootMkdirAllPath(r, name)
	if err == errPathEscapes || err == syscall.EEXIST {
		return &PathError{Op: "mkdirat", Path: name, Err: err}
	}
	if name == "" {
		return &PathError{Op: "mkdirat", Path: name, Err: syscall.ENOENT}
	}
	prefix := r.root.name + string(PathSeparator)
	if err != nil {
		if !rootMkdirAllRawFallback(name) {
			return &PathError{Op: "mkdirat", Path: name, Err: err}
		}
		path = prefix + name
	}
	if err := MkdirAll(path, perm); err != nil {
		if pe, ok := err.(*PathError); ok {
			pe.Op = "mkdirat"
			pe.Path = stringslite.TrimPrefix(pe.Path, prefix)
			return pe
		}
		return &PathError{Op: "mkdirat", Path: name, Err: underlyingError(err)}
	}
	return nil
}

func rootRemove(r *Root, name string) error {
	path, err := rootPathLstat(r, name)
	if err != nil {
		return &PathError{Op: "removeat", Path: name, Err: err}
	}
	if endsWithDot(name) {
		// We don't want to permit removing the root itself, so check for that.
		if filepathlite.Clean(name) == "." {
			return &PathError{Op: "removeat", Path: name, Err: errPathEscapes}
		}
	}
	if err := Remove(path); err != nil {
		return &PathError{Op: "removeat", Path: name, Err: underlyingError(err)}
	}
	return nil
}

func rootRemoveAll(r *Root, name string) error {
	// Consistency with os.RemoveAll: Strip trailing /s from the name,
	// so RemoveAll("not_a_directory/") succeeds.
	for len(name) > 0 && IsPathSeparator(name[len(name)-1]) {
		name = name[:len(name)-1]
	}
	if endsWithDot(name) {
		// Consistency with os.RemoveAll: Return EINVAL when trying to remove .
		return &PathError{Op: "RemoveAll", Path: name, Err: syscall.EINVAL}
	}
	path, err := rootPathLstat(r, name)
	if err != nil {
		if err == syscall.ENOTDIR {
			// Some intermediate path component is not a directory.
			// RemoveAll treats this as success (since the target doesn't exist).
			return nil
		}
		return &PathError{Op: "RemoveAll", Path: name, Err: err}
	}
	if err := RemoveAll(path); err != nil {
		return &PathError{Op: "RemoveAll", Path: name, Err: underlyingError(err)}
	}
	return nil
}

func rootReadlink(r *Root, name string) (string, error) {
	path, err := rootPathLstat(r, name)
	if err != nil {
		return "", &PathError{Op: "readlinkat", Path: name, Err: err}
	}
	name, err = Readlink(path)
	if err != nil {
		return "", &PathError{Op: "readlinkat", Path: name, Err: underlyingError(err)}
	}
	return name, nil
}

func rootRename(r *Root, oldname, newname string) error {
	oldpath, err := rootPathLstat(r, oldname)
	if err != nil {
		return &PathError{Op: "renameat", Path: oldname, Err: err}
	}
	newpath, err := rootPathLstat(r, newname)
	if err != nil {
		return &PathError{Op: "renameat", Path: newname, Err: err}
	}
	err = Rename(oldpath, newpath)
	if err != nil {
		return &LinkError{"renameat", oldname, newname, underlyingError(err)}
	}
	return nil
}

func rootLink(r *Root, oldname, newname string) error {
	fullOldName, err := rootPathLstat(r, oldname)
	if err != nil {
		return &PathError{Op: "linkat", Path: oldname, Err: err}
	}
	if fs, err := Lstat(fullOldName); err == nil && fs.Mode()&ModeSymlink != 0 {
		return &PathError{Op: "linkat", Path: oldname, Err: errors.New("cannot create a hard link to a symlink")}
	}
	newpath, err := rootPathLstat(r, newname)
	if err != nil {
		return &PathError{Op: "linkat", Path: newname, Err: err}
	}
	err = Link(fullOldName, newpath)
	if err != nil {
		return &LinkError{"linkat", oldname, newname, underlyingError(err)}
	}
	return nil
}

func rootSymlink(r *Root, oldname, newname string) error {
	newpath, err := rootPathLstat(r, newname)
	if err != nil {
		return &PathError{Op: "symlinkat", Path: newname, Err: err}
	}
	err = Symlink(oldname, newpath)
	if err != nil {
		return &LinkError{"symlinkat", oldname, newname, underlyingError(err)}
	}
	return nil
}
