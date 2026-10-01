// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !qnx

package runtime

// netpollWaitDone is only needed on qnx (see netpoll_qnx.go).
func netpollWaitDone(pd *pollDesc, mode int) {}
