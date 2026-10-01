// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

// SetVforkFailHook sets a function called before each vfork of an exec;
// while it returns true, the vfork fails with EBADF without being made.
func SetVforkFailHook(f func() bool) { vforkFailHook = f }
