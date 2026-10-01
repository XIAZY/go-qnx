// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

TEXT ·libc_eaccess_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_eaccess(SB)
TEXT ·libc_confstr_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_confstr(SB)
TEXT ·libc__cmdname_trampoline(SB),NOSPLIT,$0-0
	JMP	libc__cmdname(SB)
TEXT ·libc_getifaddrs_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_getifaddrs(SB)
TEXT ·libc_freeifaddrs_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_freeifaddrs(SB)
