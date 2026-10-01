// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build qnx

#include <sys/storage.h>
#include "libcgo.h"

static void
threadentry_platform(void)
{
	// QNX 6.5 has no ELF thread-local storage, so g lives in the
	// __reserved1 field of the thread's control block (see
	// runtime/os_qnx.go). As in runtime·mstart_stub, refuse to run
	// if something else already uses that slot.
	if (__tls()->__reserved1 != 0) {
		fatalf("QNX thread control block slot for g is in use");
	}
}

void (*x_cgo_threadentry_platform)(void) = threadentry_platform;
