// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"internal/filepathlite"
	"internal/syscall/unix"
)

func executable() (string, error) {
	path, err := unix.Cmdname()
	if err != nil {
		return "", err
	}
	return filepathlite.Clean(path), nil
}
