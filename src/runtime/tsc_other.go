// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !qnx

package runtime

// qnxCheckTSC is sysmon's check of nanotime's TSC rate on qnx
// (see tsc_qnx.go).
func qnxCheckTSC() {}
