// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !qnx

package runtime

// cpuProfWeight returns how many samples the SIGPROF being handled
// stands for, 0 for none. Each SIGPROF is one sample except on qnx,
// whose profiler signals a thread once for all the periods of CPU time
// it used since the last signal.
//
//go:nosplit
func cpuProfWeight() uint64 { return 1 }
