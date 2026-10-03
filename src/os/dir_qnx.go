// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build qnx

package os

import (
	"io"
	"runtime"
	"syscall"
	"unsafe"
)

// Auxiliary information if the File describes a directory.
//
// QNX enumerates a union directory — one served by several resource managers,
// such as / or /dev — in the client: libc's opendir connects to every server
// and merges the entries, while a read on a single descriptor reaches only the
// first server and so returns a partial listing. Directory reading therefore
// goes through libc's opendir/readdir_r/closedir. QNX's libc has no fdopendir,
// so readdir reopens the directory by name; because that is a second open, it
// verifies with fstat that the name still resolves to the same directory as
// the File's own descriptor, and returns an error rather than list the wrong
// one if a rename, or a relative path after a chdir, has changed it.
type dirInfo struct {
	dir uintptr // libc DIR*
}

func (d *dirInfo) close() {
	if d.dir == 0 {
		return
	}
	closedir(d.dir)
	d.dir = 0
}

// readdir_r writes the entry's name past Dirent.Name, which is a one-byte
// flexible array, so the entry must be backed by a buffer large enough for the
// name. QNX's NAME_MAX is 255 but it does not enforce it (a filesystem can
// return a longer component), so size for PATH_MAX, the bound on any single
// path component.
const nameMax = 1024 // PATH_MAX
const direntBufSize = int(unsafe.Offsetof(syscall.Dirent{}.Name)) + nameMax + 1

func (f *File) readdir(n int, mode readdirMode) (names []string, dirents []DirEntry, infos []FileInfo, err error) {
	// If this file has no dirinfo, create one.
	var d *dirInfo
	for {
		d = f.dirinfo.Load()
		if d != nil {
			break
		}
		dir, errno := opendir(f.name)
		if errno != nil {
			return nil, nil, nil, &PathError{Op: "opendir", Path: f.name, Err: errno}
		}
		if err := sameDir(f.pfd.Sysfd, f.name); err != nil {
			closedir(dir)
			return nil, nil, nil, &PathError{Op: "readdir", Path: f.name, Err: err}
		}
		d = &dirInfo{dir: dir}
		if f.dirinfo.CompareAndSwap(nil, d) {
			break
		}
		// We lost the race: try again.
		d.close()
	}

	size := n
	if size <= 0 {
		size = 100
		n = -1
	}

	buf := make([]byte, direntBufSize)
	entry := (*syscall.Dirent)(unsafe.Pointer(&buf[0]))
	var entptr *syscall.Dirent
	for len(names)+len(dirents)+len(infos) < size || n == -1 {
		if errno := readdir_r(d.dir, entry, &entptr); errno != 0 {
			if errno == syscall.EINTR {
				continue
			}
			return names, dirents, infos, &PathError{Op: "readdir", Path: f.name, Err: errno}
		}
		if entptr == nil { // end of directory
			break
		}
		if entry.Ino == 0 {
			continue
		}
		name := nameFromDirent(entry)
		if string(name) == "." || string(name) == ".." {
			continue
		}
		if mode == readdirName {
			names = append(names, string(name))
		} else if mode == readdirDirEntry {
			// QNX's dirent carries no type, so newUnixDirent determines it
			// with lstat.
			de, err := newUnixDirent(f, string(name), ^FileMode(0))
			if IsNotExist(err) {
				// File disappeared between readdir and stat.
				continue
			}
			if err != nil {
				// The entry exists (readdir returned it) but its type
				// can't be determined now: an active resource manager
				// under /dev, for example, answers EBUSY to lstat. List
				// it with an unknown (irregular) type rather than fail the
				// whole directory; Info reports the error if asked.
				de = &unixDirent{parent: f.name, name: string(name), typ: ModeIrregular}
			}
			dirents = append(dirents, de)
		} else {
			// Readdir needs a FileInfo for every entry, so a stat failure
			// stops it, as on every Unix. On QNX a live resource manager
			// under /dev (e.g. /dev/nws) answers EBUSY to lstat, so Readdir
			// of /dev reports that error where ReadDir lists the entry with
			// an unknown type.
			info, err := f.lstatat(string(name))
			if IsNotExist(err) {
				// File disappeared between readdir and stat.
				continue
			}
			if err != nil {
				return nil, nil, infos, err
			}
			infos = append(infos, info)
		}
		runtime.KeepAlive(f)
	}

	if n > 0 && len(names)+len(dirents)+len(infos) == 0 {
		return nil, nil, nil, io.EOF
	}
	return names, dirents, infos, nil
}

// nameFromDirent returns the NUL-terminated name stored past e.Name.
func nameFromDirent(e *syscall.Dirent) []byte {
	b := (*[nameMax + 1]byte)(unsafe.Pointer(&e.Name))[:]
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

// sameDir reports nil if the File's own descriptor and the name used to reopen
// it refer to the same directory (device and inode); otherwise an error, so
// that a reopen by name that would land on a different directory — after a
// rename, or for a relative name after a chdir — is reported rather than
// listed. QNX has no fdopendir, and its 6.5 libc has no dirfd, so the reopened
// handle's own descriptor can't be fstat'd; instead the name is stat'd and
// compared to the descriptor the File already holds, which is the directory
// opendir(name) just reopened.
func sameDir(fd int, name string) error {
	var a, b syscall.Stat_t
	// Stat the name first, right after opendir(name), so the two resolutions
	// of the name are adjacent; the File's descriptor is stable and can be
	// fstat'd after.
	if err := syscall.Stat(name, &a); err != nil {
		return err
	}
	if err := syscall.Fstat(fd, &b); err != nil {
		return err
	}
	if a.Dev != b.Dev || a.Ino != b.Ino {
		return syscall.ENOENT
	}
	return nil
}

// Implemented in syscall/syscall_qnx.go.

//go:linkname opendir syscall.opendir
func opendir(name string) (dir uintptr, err error)

//go:linkname readdir_r syscall.readdir_r
func readdir_r(dir uintptr, entry *syscall.Dirent, result **syscall.Dirent) (errno syscall.Errno)

//go:linkname closedir syscall.closedir
func closedir(dir uintptr) (err error)
