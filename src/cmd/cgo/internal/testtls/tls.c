// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stddef.h>

#if defined(__QNX__)

// QNX has no ELF thread-local storage: there is no __tls_get_addr in its
// libc, which is why the Go runtime keeps g in a thread-control-block slot
// rather than in a __thread variable. A _Thread_local here would fail to
// link, so report the test as skippable instead.
const char *
checkTLS() {
	return "QNX has no ELF thread-local storage";
}

void
setTLS(int v)
{
}

int
getTLS()
{
	return 0;
}

#elif __STDC_VERSION__ >= 201112L && !defined(__STDC_NO_THREADS__)

// Mingw seems not to have threads.h, so we use the _Thread_local keyword rather
// than the thread_local macro.
static _Thread_local int tls;

const char *
checkTLS() {
	return NULL;
}

void
setTLS(int v)
{
	tls = v;
}

int
getTLS()
{
	return tls;
}

#else

const char *
checkTLS() {
	return "_Thread_local requires C11 and not __STDC_NO_THREADS__";
}

void
setTLS(int v) {
}

int
getTLS()
{
	return 0;
}

#endif
