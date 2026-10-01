// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseShortIPv4(t *testing.T) {
	for _, tt := range []struct {
		in, want string
	}{
		{"127.1", "127.0.0.1"},
		{"10.1.2", "10.1.0.2"},
		{"10.65536", "10.1.0.0"},
		{"127.0.0.1", ""}, // not a short form; ParseAddr handles it
		{"127", ""},
		{"256.1", ""},
		{"1.16777216", ""},
		{"1.2.65536", ""},
		{"01.1", ""},
		{"1..2", ""},
		{"1.x", ""},
	} {
		ip, ok := parseShortIPv4(tt.in)
		got := ""
		if ok {
			got = ip.String()
		}
		if got != tt.want {
			t.Errorf("parseShortIPv4(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := parseHostsFileIP("127.1"); got != "127.0.0.1" {
		t.Errorf(`parseHostsFileIP("127.1") = %q, want "127.0.0.1"`, got)
	}
	// The API does not accept the short form, on QNX as elsewhere.
	if got := parseLiteralIP("127.1"); got != "" {
		t.Errorf(`parseLiteralIP("127.1") = %q, want ""`, got)
	}
}

func TestQNXHostsShortForm(t *testing.T) {
	defer func(orig string) { hostsFilePath = orig }(hostsFilePath)
	hostsFilePath = filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hostsFilePath, []byte("127.1\t\tlocalhost.localdomain localhost\n::1\t\tlocalhost.localdomain localhost\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	addrs, _ := lookupStaticHost("localhost")
	if !slices.Contains(addrs, "127.0.0.1") || !slices.Contains(addrs, "::1") {
		t.Errorf("localhost = %q, want 127.0.0.1 and ::1", addrs)
	}
	if names := lookupStaticAddr("127.1"); len(names) != 0 {
		t.Errorf("lookupStaticAddr(%q) = %q, want none", "127.1", names)
	}
	if names := lookupStaticAddr("127.0.0.1"); !slices.Contains(names, "localhost") {
		t.Errorf("lookupStaticAddr(%q) = %q, want localhost", "127.0.0.1", names)
	}
}
