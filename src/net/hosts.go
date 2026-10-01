// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"errors"
	"internal/bytealg"
	"io/fs"
	"net/netip"
	"runtime"
	"sync"
	"time"
)

const cacheMaxAge = 5 * time.Second

func parseLiteralIP(addr string) string {
	ip, err := netip.ParseAddr(addr)
	if err != nil {
		return ""
	}
	return ip.String()
}

// parseHostsFileIP is parseLiteralIP for the address column of the
// hosts file. QNX's stock /etc/hosts says "127.1 localhost", in the
// shortened form inet_aton accepts; without it localhost would resolve
// only to ::1, which QNX's default IPv4-only network stack cannot use.
// Only the file is read this way: addresses given to the API are not.
func parseHostsFileIP(addr string) string {
	if s := parseLiteralIP(addr); s != "" || runtime.GOOS != "qnx" {
		return s
	}
	if ip, ok := parseShortIPv4(addr); ok {
		return ip.String()
	}
	return ""
}

// parseShortIPv4 parses the decimal forms "a.b" and "a.b.c" that
// inet_aton accepts, in which the last part fills the remaining bytes.
func parseShortIPv4(s string) (netip.Addr, bool) {
	var parts []string // split at dots, keeping empty parts
	for {
		i := 0
		for i < len(s) && s[i] != '.' {
			i++
		}
		parts = append(parts, s[:i])
		if i == len(s) {
			break
		}
		s = s[i+1:]
	}
	if len(parts) < 2 || len(parts) > 3 {
		return netip.Addr{}, false
	}
	var v uint32
	for i, p := range parts {
		if p == "" || len(p) > 10 || (len(p) > 1 && p[0] == '0') {
			return netip.Addr{}, false
		}
		var n uint64
		for _, c := range p {
			if c < '0' || c > '9' {
				return netip.Addr{}, false
			}
			n = n*10 + uint64(c-'0')
		}
		bits := uint(8)
		if i == len(parts)-1 {
			bits = uint(8 * (5 - len(parts)))
		}
		if n >= 1<<bits {
			return netip.Addr{}, false
		}
		v = v<<bits | uint32(n)
	}
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), true
}

type byName struct {
	addrs         []string
	canonicalName string
}

// hosts contains known host entries.
var hosts struct {
	sync.Mutex

	// Key for the list of literal IP addresses must be a host
	// name. It would be part of DNS labels, a FQDN or an absolute
	// FQDN.
	// For now the key is converted to lower case for convenience.
	byName map[string]byName

	// Key for the list of host names must be a literal IP address
	// including IPv6 address with zone identifier.
	// We don't support old-classful IP address notation.
	byAddr map[string][]string

	expire time.Time
	path   string
	mtime  time.Time
	size   int64
}

func readHosts() {
	now := time.Now()
	hp := hostsFilePath

	if now.Before(hosts.expire) && hosts.path == hp && len(hosts.byName) > 0 {
		return
	}
	mtime, size, err := stat(hp)
	if err == nil && hosts.path == hp && hosts.mtime.Equal(mtime) && hosts.size == size {
		hosts.expire = now.Add(cacheMaxAge)
		return
	}

	hs := make(map[string]byName)
	is := make(map[string][]string)

	file, err := open(hp)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrPermission) {
			return
		}
	}

	if file != nil {
		defer file.close()
		for line, ok := file.readLine(); ok; line, ok = file.readLine() {
			if i := bytealg.IndexByteString(line, '#'); i >= 0 {
				// Discard comments.
				line = line[0:i]
			}
			f := getFields(line)
			if len(f) < 2 {
				continue
			}
			addr := parseHostsFileIP(f[0])
			if addr == "" {
				continue
			}

			var canonical string
			for i := 1; i < len(f); i++ {
				name := absDomainName(f[i])
				h := []byte(f[i])
				lowerASCIIBytes(h)
				key := absDomainName(string(h))

				if i == 1 {
					canonical = key
				}

				is[addr] = append(is[addr], name)

				if v, ok := hs[key]; ok {
					hs[key] = byName{
						addrs:         append(v.addrs, addr),
						canonicalName: v.canonicalName,
					}
					continue
				}

				hs[key] = byName{
					addrs:         []string{addr},
					canonicalName: canonical,
				}
			}
		}
	}
	// Update the data cache.
	hosts.expire = now.Add(cacheMaxAge)
	hosts.path = hp
	hosts.byName = hs
	hosts.byAddr = is
	hosts.mtime = mtime
	hosts.size = size
}

// lookupStaticHost looks up the addresses and the canonical name for the given host from /etc/hosts.
func lookupStaticHost(host string) ([]string, string) {
	hosts.Lock()
	defer hosts.Unlock()
	readHosts()
	if len(hosts.byName) != 0 {
		if hasUpperCase(host) {
			lowerHost := []byte(host)
			lowerASCIIBytes(lowerHost)
			host = string(lowerHost)
		}
		if byName, ok := hosts.byName[absDomainName(host)]; ok {
			ipsCp := make([]string, len(byName.addrs))
			copy(ipsCp, byName.addrs)
			return ipsCp, byName.canonicalName
		}
	}
	return nil, ""
}

// lookupStaticAddr looks up the hosts for the given address from /etc/hosts.
func lookupStaticAddr(addr string) []string {
	hosts.Lock()
	defer hosts.Unlock()
	readHosts()
	addr = parseLiteralIP(addr)
	if addr == "" {
		return nil
	}
	if len(hosts.byAddr) != 0 {
		if hosts, ok := hosts.byAddr[addr]; ok {
			hostsCp := make([]string, len(hosts))
			copy(hostsCp, hosts)
			return hostsCp
		}
	}
	return nil
}
