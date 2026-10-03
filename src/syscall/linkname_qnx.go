// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

import _ "unsafe"

// used by os (directory reading; QNX has no fdopendir, so os reopens by name)
//go:linkname opendir
//go:linkname readdir_r
//go:linkname closedir
