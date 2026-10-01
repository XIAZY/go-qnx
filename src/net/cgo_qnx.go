// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build cgo && !netgo

package net

/*
#cgo LDFLAGS: -lsocket
#include <netdb.h>
*/
import "C"

// QNX 6.5's getaddrinfo, getnameinfo and resolver (res_ninit and
// friends) live in libsocket, and it has no AI_V4MAPPED or AI_ALL.
const cgoAddrInfoFlags = C.AI_CANONNAME
