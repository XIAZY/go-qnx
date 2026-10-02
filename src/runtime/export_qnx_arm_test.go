// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// NanotimeCallsLibc reports whether nanotime reads the clock through
// libc. On qnx/arm it always does (clock_gettime).
func NanotimeCallsLibc() bool { return true }
