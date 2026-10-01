// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build dragonfly || freebsd || netbsd || openbsd || qnx || solaris

package runtime

import (
	"unsafe"
)

// QNX commits memory for an anonymous mapping when it is made, even a
// PROT_NONE one, unless it is asked for MAP_LAZY. On a 127 MB QNX 6.5
// machine a 256 MB PROT_NONE reservation failed with ENOMEM, and Go's
// address space reservations used up memory that was never touched.
// Reservations are therefore lazy on QNX. Mappings that are about to be
// used stay eager, so that running out of memory is an ENOMEM from mmap
// (reported as "out of memory") rather than a SIGBUS on first touch.
const _qnxMAP_LAZY = 0x80

// lazyFlags returns the flags for an anonymous mapping that should not
// take memory until it is touched.
//
//go:nosplit
func lazyFlags() int32 {
	flags := int32(_MAP_ANON | _MAP_PRIVATE)
	if GOOS == "qnx" {
		flags |= _qnxMAP_LAZY
	}
	return flags
}

// Don't split the stack as this function may be invoked without a valid G,
// which prevents us from allocating more stack.
//
//go:nosplit
func sysAllocOS(n uintptr, _ string) unsafe.Pointer {
	v, err := mmap(nil, n, _PROT_READ|_PROT_WRITE, _MAP_ANON|_MAP_PRIVATE, -1, 0)
	if err != 0 {
		return nil
	}
	return v
}

func sysUnusedOS(v unsafe.Pointer, n uintptr) {
	if GOOS == "qnx" {
		// posix_madvise(POSIX_MADV_DONTNEED) succeeds on QNX 6.5 but
		// releases nothing: after touching 40 MB, free memory stayed at
		// 36 MB of 127 MB after posix_madvise, and was back at 76 MB
		// once the range was mapped again. So replace the pages with a
		// new lazy mapping: like MADV_DONTNEED on Linux, they then read
		// as zero and take memory again only when touched.
		mmap(v, n, _PROT_READ|_PROT_WRITE, lazyFlags()|_MAP_FIXED, -1, 0)
		return
	}
	if debug.madvdontneed != 0 {
		madvise(v, n, _MADV_DONTNEED)
	} else {
		madvise(v, n, _MADV_FREE)
	}
}

func sysUsedOS(v unsafe.Pointer, n uintptr) {
	if GOOS == "qnx" {
		// Take the memory back now, as Windows does, so that running
		// out is reported here rather than as a SIGBUS on a lazy page
		// later. The range holds no live data: the heap passes only
		// free pages, whose contents need not be kept (freed pages are
		// zeroed before reuse unless they were never used, and fresh
		// pages are zero anyway), and other callers pass memory that
		// was just mapped. Release it first so that the new mapping
		// does not need the old pages' memory as well.
		mmap(v, n, _PROT_READ|_PROT_WRITE, lazyFlags()|_MAP_FIXED, -1, 0)
		p, err := mmap(v, n, _PROT_READ|_PROT_WRITE, _MAP_ANON|_MAP_FIXED|_MAP_PRIVATE, -1, 0)
		if err == _ENOMEM {
			throw("runtime: out of memory")
		}
		if p != v || err != 0 {
			print("runtime: mmap(", v, ", ", n, ") returned ", p, ", ", err, "\n")
			throw("runtime: cannot commit pages")
		}
	}
}

func sysHugePageOS(v unsafe.Pointer, n uintptr) {
}

func sysNoHugePageOS(v unsafe.Pointer, n uintptr) {
}

func sysHugePageCollapseOS(v unsafe.Pointer, n uintptr) {
}

// Don't split the stack as this function may be invoked without a valid G,
// which prevents us from allocating more stack.
//
//go:nosplit
func sysFreeOS(v unsafe.Pointer, n uintptr) {
	munmap(v, n)
}

func sysFaultOS(v unsafe.Pointer, n uintptr) {
	mmap(v, n, _PROT_NONE, lazyFlags()|_MAP_FIXED, -1, 0)
}

// Indicates not to reserve swap space for the mapping.
const _sunosMAP_NORESERVE = 0x40

func sysReserveOS(v unsafe.Pointer, n uintptr, _ string) unsafe.Pointer {
	flags := lazyFlags()
	if GOOS == "solaris" || GOOS == "illumos" {
		// Be explicit that we don't want to reserve swap space
		// for PROT_NONE anonymous mappings. This avoids an issue
		// wherein large mappings can cause fork to fail.
		flags |= _sunosMAP_NORESERVE
	}
	p, err := mmap(v, n, _PROT_NONE, flags, -1, 0)
	if err != 0 {
		return nil
	}
	return p
}

const _sunosEAGAIN = 11
const _ENOMEM = 12

func sysMapOS(v unsafe.Pointer, n uintptr, _ string) {
	p, err := mmap(v, n, _PROT_READ|_PROT_WRITE, _MAP_ANON|_MAP_FIXED|_MAP_PRIVATE, -1, 0)
	if err == _ENOMEM || ((GOOS == "solaris" || GOOS == "illumos") && err == _sunosEAGAIN) {
		throw("runtime: out of memory")
	}
	if p != v || err != 0 {
		print("runtime: mmap(", v, ", ", n, ") returned ", p, ", ", err, "\n")
		throw("runtime: cannot map pages in arena address space")
	}
}

func needZeroAfterSysUnusedOS() bool {
	return true
}
