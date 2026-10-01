// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package runtime_test

import (
	"runtime"
	"testing"
	"unsafe"
)

// Test that the error value returned by mmap is positive, as that is
// what the code in mem_bsd.go, mem_darwin.go, and mem_linux.go expects.
// See the uses of ENOMEM in sysMap in those files.
func TestMmapErrorSign(t *testing.T) {
	p, err := runtime.Mmap(nil, ^uintptr(0)&^(runtime.GetPhysPageSize()-1), 0, runtime.MAP_ANON|runtime.MAP_PRIVATE, -1, 0)

	if p != nil || err != runtime.ENOMEM {
		t.Errorf("mmap = %v, %v, want nil, %v", p, err, runtime.ENOMEM)
	}
}

func TestPhysPageSize(t *testing.T) {
	// Mmap fails if the address is not page aligned, so we can
	// use this to test if the page size is the true page size.
	ps := runtime.GetPhysPageSize()
	if runtime.GOOS == "qnx" {
		// QNX 6.5 accepts a MAP_FIXED address that is not page
		// aligned (a C mmap at base+2048 returns base+2048), so the
		// half-page check below does not hold there. The runtime
		// takes the page size from sysconf, so check it against the
		// MMU's instead: x86 pages are 4 KB, and mmap returns
		// page-aligned addresses.
		if runtime.GOARCH == "386" && ps != 4096 {
			t.Fatalf("physPageSize = %d, want 4096", ps)
		}
		for range 16 {
			b, err := runtime.Mmap(nil, ps, 0, runtime.MAP_ANON|runtime.MAP_PRIVATE, -1, 0)
			if err != 0 {
				t.Fatalf("Mmap: %v", err)
			}
			if uintptr(b)%ps != 0 {
				t.Errorf("Mmap returned %p, not aligned to physPageSize %d", b, ps)
			}
			runtime.Munmap(b, ps)
		}
	}

	// Get a region of memory to play with. This should be page-aligned.
	b, err := runtime.Mmap(nil, 2*ps, 0, runtime.MAP_ANON|runtime.MAP_PRIVATE, -1, 0)
	if err != 0 {
		t.Fatalf("Mmap: %v", err)
	}

	if runtime.GOOS == "aix" {
		// AIX does not allow mapping a range that is already mapped.
		runtime.Munmap(unsafe.Pointer(uintptr(b)), 2*ps)
	}

	// Mmap should fail at a half page into the buffer.
	if runtime.GOOS != "qnx" {
		_, err = runtime.Mmap(unsafe.Pointer(uintptr(b)+ps/2), ps, 0, runtime.MAP_ANON|runtime.MAP_PRIVATE|runtime.MAP_FIXED, -1, 0)
		if err == 0 {
			t.Errorf("Mmap should have failed with half-page alignment %d, but succeeded: %v", ps/2, err)
		}
	}

	// Mmap should succeed at a full page into the buffer.
	_, err = runtime.Mmap(unsafe.Pointer(uintptr(b)+ps), ps, 0, runtime.MAP_ANON|runtime.MAP_PRIVATE|runtime.MAP_FIXED, -1, 0)
	if err != 0 {
		t.Errorf("Mmap at full-page alignment %d failed: %v", ps, err)
	}
}
