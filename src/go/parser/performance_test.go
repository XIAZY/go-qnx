// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"go/token"
	"internal/testenv"
	"os"
	"sync"
	"testing"
)

var (
	srcOnce sync.Once
	src     []byte
)

// benchSrc lazily loads the file the benchmarks parse. It is not read at
// package init because the source tree is not always present where the test
// binary runs (for example a wrapper that copies only the test binary to a
// device); testenv.MustHaveSource skips cleanly in that case.
func benchSrc(b *testing.B) []byte {
	testenv.MustHaveSource(b)
	srcOnce.Do(func() {
		data, err := os.ReadFile("../printer/nodes.go")
		if err != nil {
			b.Fatal(err)
		}
		src = data
	})
	return src
}

// readFile reads a file or panics; used by tests that read testdata.
func readFile(filename string) []byte {
	data, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	return data
}

func BenchmarkParse(b *testing.B) {
	src := benchSrc(b)
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		if _, err := ParseFile(token.NewFileSet(), "", src, ParseComments); err != nil {
			b.Fatalf("benchmark failed due to parse error: %s", err)
		}
	}
}

func BenchmarkParseOnly(b *testing.B) {
	src := benchSrc(b)
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		if _, err := ParseFile(token.NewFileSet(), "", src, ParseComments|SkipObjectResolution); err != nil {
			b.Fatalf("benchmark failed due to parse error: %s", err)
		}
	}
}

func BenchmarkResolve(b *testing.B) {
	src := benchSrc(b)
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		fset := token.NewFileSet()
		file, err := ParseFile(fset, "", src, SkipObjectResolution)
		if err != nil {
			b.Fatalf("benchmark failed due to parse error: %s", err)
		}
		b.StartTimer()
		handle := fset.File(file.Package)
		resolveFile(file, handle, nil)
	}
}
