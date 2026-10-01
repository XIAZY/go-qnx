// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tar

import (
	"syscall"
	"time"
)

// QNX 6.5 keeps whole seconds, as an unsigned 32-bit time_t.

func statAtime(st *syscall.Stat_t) time.Time {
	return time.Unix(int64(st.Atime), 0)
}

func statCtime(st *syscall.Stat_t) time.Time {
	return time.Unix(int64(st.Ctime), 0)
}
