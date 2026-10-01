// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"internal/poll"
	"internal/syscall/unix"
	"os"
	"syscall"
	"unsafe"
)

// The interfaces and their addresses come from getifaddrs in libsocket,
// which lists an AF_LINK entry for each interface (its index, hardware
// address and struct if_data) followed by its AF_INET and AF_INET6
// entries.

// ifaddrsList returns a copy of what getifaddrs reports.
func ifaddrsList() ([]ifaddr, error) {
	// getifaddrs makes requests to io-pkt of its own, so it takes the
	// socket lock (see internal/poll's socklock_qnx.go).
	poll.SockRLock()
	head, err := unix.Getifaddrs()
	poll.SockRUnlock()
	if err != nil {
		return nil, os.NewSyscallError("getifaddrs", err)
	}
	defer unix.Freeifaddrs(head)
	var list []ifaddr
	for p := head; p != nil; p = p.Next {
		a := ifaddr{name: gostring(p.Name), flags: p.Flags}
		if p.Addr != nil {
			a.family = p.Addr.Family
			a.addr = sockaddrBytes(p.Addr)
		}
		if p.Netmask != nil {
			a.netmask = sockaddrBytes(p.Netmask)
		}
		if a.family == syscall.AF_LINK && p.Data != nil {
			a.mtu = int(*(*uint64)(unsafe.Add(p.Data, unix.IfDataMTUOffset)))
		}
		list = append(list, a)
	}
	return list, nil
}

type ifaddr struct {
	name    string
	flags   uint32
	family  uint8
	addr    []byte // the sockaddr, sa_len bytes
	netmask []byte
	mtu     int // AF_LINK only
}

func gostring(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// sockaddrBytes returns the sa_len bytes of sa. Netmasks may be shorter
// than their family's sockaddr; the missing bytes are zero.
func sockaddrBytes(sa *syscall.RawSockaddr) []byte {
	n := int(sa.Len)
	if n < 2 {
		return nil
	}
	b := make([]byte, n)
	copy(b, unsafe.Slice((*byte)(unsafe.Pointer(sa)), n))
	return b
}

// sockaddrIP returns the address in the AF_INET or AF_INET6 sockaddr b,
// zero-filled past its end.
func sockaddrIP(family uint8, b []byte) []byte {
	off, n := 4, IPv4len // sin_addr
	if family == syscall.AF_INET6 {
		off, n = 8, IPv6len // sin6_addr
	}
	ip := make([]byte, n)
	if len(b) > off {
		copy(ip, b[off:])
	}
	return ip
}

// If the ifindex is zero, interfaceTable returns mappings of all
// network interfaces. Otherwise it returns a mapping of a specific
// interface.
func interfaceTable(ifindex int) ([]Interface, error) {
	list, err := ifaddrsList()
	if err != nil {
		return nil, err
	}
	var ift []Interface
	seen := make(map[int]int) // index to position in ift
	for _, a := range list {
		if a.family != syscall.AF_LINK || len(a.addr) < 8 {
			continue
		}
		// struct sockaddr_dl: sdl_len, sdl_family, sdl_index (2 bytes),
		// sdl_type, sdl_nlen, sdl_alen, sdl_slen, sdl_data.
		index := int(*(*uint16)(unsafe.Pointer(&a.addr[2])))
		if ifindex != 0 && ifindex != index {
			continue
		}
		// getifaddrs may list an interface's AF_LINK address twice, and
		// only one of the entries carries its struct if_data.
		if i, ok := seen[index]; ok {
			if ift[i].MTU == 0 {
				ift[i].MTU = a.mtu
			}
			continue
		}
		seen[index] = len(ift)
		ifi := Interface{Index: index, MTU: a.mtu, Name: a.name, Flags: linkFlags(a.flags)}
		nlen, alen := int(a.addr[5]), int(a.addr[6])
		if alen > 0 && 8+nlen+alen <= len(a.addr) {
			ifi.HardwareAddr = make(HardwareAddr, alen)
			copy(ifi.HardwareAddr, a.addr[8+nlen:])
		}
		ift = append(ift, ifi)
	}
	return ift, nil
}

func linkFlags(rawFlags uint32) Flags {
	var f Flags
	if rawFlags&syscall.IFF_UP != 0 {
		f |= FlagUp
	}
	if rawFlags&syscall.IFF_RUNNING != 0 {
		f |= FlagRunning
	}
	if rawFlags&syscall.IFF_BROADCAST != 0 {
		f |= FlagBroadcast
	}
	if rawFlags&syscall.IFF_LOOPBACK != 0 {
		f |= FlagLoopback
	}
	if rawFlags&syscall.IFF_POINTOPOINT != 0 {
		f |= FlagPointToPoint
	}
	if rawFlags&syscall.IFF_MULTICAST != 0 {
		f |= FlagMulticast
	}
	return f
}

// If the ifi is nil, interfaceAddrTable returns addresses for all
// network interfaces. Otherwise it returns addresses for a specific
// interface.
func interfaceAddrTable(ifi *Interface) ([]Addr, error) {
	list, err := ifaddrsList()
	if err != nil {
		return nil, err
	}
	var ifat []Addr
	for _, a := range list {
		if a.family != syscall.AF_INET && a.family != syscall.AF_INET6 {
			continue
		}
		if ifi != nil && ifi.Name != a.name {
			continue
		}
		var ip IP
		b := sockaddrIP(a.family, a.addr)
		mask := IPMask(sockaddrIP(a.family, a.netmask))
		if a.family == syscall.AF_INET {
			ip = IPv4(b[0], b[1], b[2], b[3])
		} else {
			ip = IP(b)
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
				// The KAME-derived stack embeds the interface index
				// in the second 16 bits of link-local addresses.
				ip[2], ip[3] = 0, 0
			}
		}
		ifat = append(ifat, &IPNet{IP: ip, Mask: mask})
	}
	return ifat, nil
}

// interfaceMulticastAddrTable returns addresses for a specific
// interface. getifaddrs does not list multicast group memberships, so,
// as on NetBSD, from which QNX's network stack derives, none are
// reported.
func interfaceMulticastAddrTable(ifi *Interface) ([]Addr, error) {
	return nil, nil
}
