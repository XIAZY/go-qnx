// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestQNXResolveConf(t *testing.T) {
	cs := ""
	defer func(f func() string) { csResolve = f }(csResolve)
	csResolve = func() string { return cs }

	// _CS_RESOLVE, when set, replaces /etc/resolv.conf: lines are
	// separated by spaces, and spaces within lines are underscores.
	cs = "nameserver_10.0.2.3 nameserver_192.0.2.53 search_example.com_example.org"
	conf := dnsReadConfig("/etc/resolv.conf")
	if want := []string{"10.0.2.3:53", "192.0.2.53:53"}; !slices.Equal(conf.servers, want) {
		t.Errorf("servers = %q, want %q", conf.servers, want)
	}
	if want := []string{"example.com.", "example.org."}; !slices.Equal(conf.search, want) {
		t.Errorf("search = %q, want %q", conf.search, want)
	}
	if conf.resolveConf != cs {
		t.Errorf("resolveConf = %q, want %q", conf.resolveConf, cs)
	}

	// Transitions of _CS_RESOLVE.
	if qnxResolvConfChanged("/etc/resolv.conf", conf) {
		t.Error("unchanged _CS_RESOLVE reported as changed")
	}
	cs = "nameserver_10.0.2.4"
	if !qnxResolvConfChanged("/etc/resolv.conf", conf) {
		t.Error("changed _CS_RESOLVE not reported")
	}
	cs = ""
	if !qnxResolvConfChanged("/etc/resolv.conf", conf) {
		t.Error("cleared _CS_RESOLVE not reported")
	}
	// Never set: falls back to the file's mtime, which differs from a
	// zero mtime only if the file exists.
	_, err := os.Stat("/etc/resolv.conf")
	if got, want := qnxResolvConfChanged("/etc/resolv.conf", &dnsConfig{}), err == nil; got != want {
		t.Errorf("unset _CS_RESOLVE, /etc/resolv.conf exists=%v: changed = %v, want %v", err == nil, got, want)
	}
	cs = "nameserver_10.0.2.3"
	if !qnxResolvConfChanged("/etc/resolv.conf", &dnsConfig{}) {
		t.Error("newly set _CS_RESOLVE not reported")
	}

	// Other files are never read from _CS_RESOLVE.
	dir := t.TempDir()
	name := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(name, []byte("nameserver 198.51.100.1\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	conf = dnsReadConfig(name)
	if want := []string{"198.51.100.1:53"}; !slices.Equal(conf.servers, want) || conf.resolveConf != "" {
		t.Errorf("%s: servers = %q, resolveConf = %q", name, conf.servers, conf.resolveConf)
	}
	if qnxResolvConfChanged(name, conf) {
		t.Error("unchanged file reported as changed")
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(name, future, future); err != nil {
		t.Fatal(err)
	}
	if !qnxResolvConfChanged(name, conf) {
		t.Error("file with new mtime not reported as changed")
	}
}
