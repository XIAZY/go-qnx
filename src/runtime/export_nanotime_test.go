// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !qnx

package runtime

// NanotimeCallsLibc reports whether nanotime reads the clock through
// libc in a way that depends on the machine, as on qnx. The systems
// where it always does are listed by GOOS where it matters
// (TestTimePprof).
func NanotimeCallsLibc() bool { return false }
