// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !qnx

package poll

// The socket lock is only needed on qnx; see socklock_qnx.go.

func (fd *FD) sockRLock()   {}
func (fd *FD) sockRUnlock() {}

// SockRLock does nothing: see socklock_qnx.go.
func SockRLock() {}

// SockRUnlock does nothing: see socklock_qnx.go.
func SockRUnlock() {}
