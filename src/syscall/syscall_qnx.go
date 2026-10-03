// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// QNX Neutrino 6.5 system calls.
// This file is compiled as ordinary Go code,
// but it is also input to mksyscall,
// which parses the //sys lines and generates system call stubs.
//
// QNX has no system call numbers in the Unix sense: open, read and
// the rest are messages to resource managers, sent by libc. So, as on
// OpenBSD, every call goes through a libc (or libsocket) function via
// the runtime's syscall_syscall* functions, and Syscall with a trap
// number always fails with ENOSYS.
//
// Where libc has 64-bit variants (fstat64, lseek64, ...), the //sys
// lines name them explicitly: our Stat_t, Dirent and Rlimit are the
// _FILE_OFFSET_BITS=64 layouts, and the unsuffixed functions take the
// 32-bit ones.

package syscall

import (
	"internal/abi"
	"unsafe"
)

// SYS_EXECVE only exists so that exec_unix.go compiles; QNX uses
// execveLibc there, like the other libc-based ports.
const SYS_EXECVE = 0

func init() {
	execveLibc = execve
}

// Constants QNX 6.5 does not have. As on AIX, F_DUPFD_CLOEXEC is 0 so
// that internal/poll uses the F_DUPFD+FD_CLOEXEC fallback. O_DIRECTORY
// is 0 too: package os checks that what it opened is a directory.
const (
	F_DUPFD_CLOEXEC = 0
	O_DIRECTORY     = 0
)

// Implemented in the runtime package (runtime/sys_qnx.go).
func syscall(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno)
func syscallX(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno)
func syscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno)
func syscall6X(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno)
func syscall10(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10 uintptr) (r1, r2 uintptr, err Errno)
func syscallPtr(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno)
func rawSyscall(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno)
func rawSyscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno)
func rawSyscall10(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10 uintptr) (r1, r2 uintptr, err Errno)

func syscall9(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9 uintptr) (r1, r2 uintptr, err Errno) {
	return syscall10(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, 0)
}

func rawSyscall9(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9 uintptr) (r1, r2 uintptr, err Errno) {
	return rawSyscall10(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, 0)
}

// There are no trap numbers on QNX.

func Syscall(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno) {
	return 0, 0, ENOSYS
}

func Syscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno) {
	return 0, 0, ENOSYS
}

func Syscall9(trap, a1, a2, a3, a4, a5, a6, a7, a8, a9 uintptr) (r1, r2 uintptr, err Errno) {
	return 0, 0, ENOSYS
}

func RawSyscall(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno) {
	return 0, 0, ENOSYS
}

func RawSyscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno) {
	return 0, 0, ENOSYS
}

// retryEINTR calls f until it does not fail with EINTR. QNX 6.5 has no
// SA_RESTART, so calls that other systems restart transparently after a
// signal fail here, and Go's signal handling sends signals routinely.
func retryEINTR(f func() error) error {
	for {
		if err := f(); err != EINTR {
			return err
		}
	}
}

/*
 * Wrapped
 */

// getcwd returns buf, or NULL on failure, so it needs syscallPtr.
func getcwd(buf []byte) (err error) {
	var p unsafe.Pointer
	if len(buf) > 0 {
		p = unsafe.Pointer(&buf[0])
	} else {
		p = unsafe.Pointer(&_zero)
	}
	_, _, e1 := syscallPtr(abi.FuncPCABI0(libc_getcwd_trampoline), uintptr(p), uintptr(len(buf)), 0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

func libc_getcwd_trampoline()

//go:cgo_import_dynamic libc_getcwd getcwd "libc.so.3"

const ImplementsGetwd = true

// Directory reading on QNX goes through libc's opendir/readdir_r/closedir
// rather than read(2) on a directory descriptor. QNX merges a union directory
// (one served by several resource managers, such as / or /dev) in the client:
// opendir connects to every server, while a plain read reaches only the first,
// so read(2) silently returns a partial listing. QNX's libc has no fdopendir,
// so os reopens the directory by name and verifies it with fstat; see
// os/dir_qnx.go. These are linked into os with go:linkname.

func opendir(name string) (dir uintptr, err error) {
	var p *byte
	p, err = BytePtrFromString(name)
	if err != nil {
		return 0, err
	}
	r1, _, e1 := syscallPtr(abi.FuncPCABI0(libc_opendir_trampoline), uintptr(unsafe.Pointer(p)), 0, 0)
	if r1 == 0 {
		if e1 != 0 {
			return 0, errnoErr(e1)
		}
		return 0, EINVAL
	}
	return r1, nil
}
func libc_opendir_trampoline()

//go:cgo_import_dynamic libc_opendir opendir "libc.so.3"

// readdir_r reports its error through the return value, not the errno global:
// 0 on success (with result pointing at entry, or nil at end of directory) or
// an errno. entry must have room for the name beyond Dirent.Name; os provides
// a buffer of that size.
func readdir_r(dir uintptr, entry *Dirent, result **Dirent) (errno Errno) {
	r1, _, _ := syscall(abi.FuncPCABI0(libc_readdir_r_trampoline), dir, uintptr(unsafe.Pointer(entry)), uintptr(unsafe.Pointer(result)))
	return Errno(r1)
}
func libc_readdir_r_trampoline()

//go:cgo_import_dynamic libc_readdir_r readdir_r "libc.so.3"

func closedir(dir uintptr) (err error) {
	_, _, e1 := syscall(abi.FuncPCABI0(libc_closedir_trampoline), dir, 0, 0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}
func libc_closedir_trampoline()

//go:cgo_import_dynamic libc_closedir closedir "libc.so.3"

func Getwd() (string, error) {
	var buf [PathMax]byte
	if err := getcwd(buf[:]); err != nil {
		return "", err
	}
	n := clen(buf[:])
	if n < 1 {
		return "", EINVAL
	}
	return string(buf[:n]), nil
}

//sysnb	getgroups(ngid int, gid *_Gid_t) (n int, err error)
//sysnb	setgroups(ngid int, gid *_Gid_t) (err error)

func Getgroups() (gids []int, err error) {
	n, err := getgroups(0, nil)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}

	// Sanity check group count. Max is 16 on BSD.
	if n < 0 || n > 1000 {
		return nil, EINVAL
	}

	a := make([]_Gid_t, n)
	n, err = getgroups(n, &a[0])
	if err != nil {
		return nil, err
	}
	gids = make([]int, n)
	for i, v := range a[0:n] {
		gids[i] = int(v)
	}
	return
}

func Setgroups(gids []int) (err error) {
	if len(gids) == 0 {
		return setgroups(0, nil)
	}

	a := make([]_Gid_t, len(gids))
	for i, v := range gids {
		a[i] = _Gid_t(v)
	}
	return setgroups(len(a), &a[0])
}

// ReadDirent reads directory entries from fd into buf. On QNX a read
// of a directory descriptor returns struct dirent records; a read of any
// other descriptor returns its bytes instead of failing, so check.
func ReadDirent(fd int, buf []byte) (n int, err error) {
	var st Stat_t
	if err := Fstat(fd, &st); err != nil {
		return 0, err
	}
	if st.Mode&S_IFMT != S_IFDIR {
		return 0, ENOTDIR
	}
	return read(fd, buf)
}

func direntIno(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(Dirent{}.Ino), unsafe.Sizeof(Dirent{}.Ino))
}

func direntReclen(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(Dirent{}.Reclen), unsafe.Sizeof(Dirent{}.Reclen))
}

func direntNamlen(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(Dirent{}.Namelen), unsafe.Sizeof(Dirent{}.Namelen))
}

// Wait status, as in QNX's <sys/wait.h>. It differs from the BSD
// encoding: a signaled status has a zero high byte.
type WaitStatus uint32

const (
	waitStopFlag = 0x7f
	waitCoreFlag = 0x80
)

func (w WaitStatus) Exited() bool { return w&0xff == 0 }

func (w WaitStatus) ExitStatus() int {
	if !w.Exited() {
		return -1
	}
	return int(w>>8) & 0xff
}

func (w WaitStatus) Signaled() bool { return w&0xff != 0 && w&0xff00 == 0 }

func (w WaitStatus) Signal() Signal {
	if !w.Signaled() {
		return -1
	}
	return Signal(w & 0x7f)
}

func (w WaitStatus) CoreDump() bool { return w.Signaled() && w&waitCoreFlag != 0 }

func (w WaitStatus) Stopped() bool { return w&0xff == waitStopFlag && w&0xff00 != 0 }

func (w WaitStatus) Continued() bool { return w&0xffff == 0xffff }

func (w WaitStatus) StopSignal() Signal {
	if !w.Stopped() {
		return -1
	}
	return Signal(w>>8) & 0xff
}

func (w WaitStatus) TrapCause() int { return -1 }

//sys	wait4(pid int, wstatus *_C_int, options int, rusage *Rusage) (wpid int, err error)

func Wait4(pid int, wstatus *WaitStatus, options int, rusage *Rusage) (wpid int, err error) {
	var status _C_int
	err = retryEINTR(func() error {
		var e error
		wpid, e = wait4(pid, &status, options, rusage)
		return e
	})
	if wstatus != nil {
		*wstatus = WaitStatus(status)
	}
	return
}

func Pipe(p []int) error {
	if len(p) != 2 {
		return EINVAL
	}
	var pp [2]_C_int
	err := pipe(&pp)
	if err == nil {
		p[0] = int(pp[0])
		p[1] = int(pp[1])
	}
	return err
}

// QNX 6.5 has no pipe2, accept4 or dup3, so there is no Pipe2, Accept4
// or Dup3 either: an emulation could not set O_CLOEXEC atomically.
// Callers use Pipe, Accept and Dup2 and set close-on-exec while holding
// ForkLock, as on AIX and Darwin (see forkpipe.go).

// Open opens path with O_LARGEFILE, as on 32-bit Linux: without it, QNX
// fails any offset of 2 GB or more on a qnx6 filesystem with EOVERFLOW,
// lseek64 included.
func Open(path string, mode int, perm uint32) (fd int, err error) {
	return open(path, mode|O_LARGEFILE, perm)
}

func Getdirentries(fd int, buf []byte, basep *uintptr) (n int, err error) {
	return ReadDirent(fd, buf)
}

// QNX 6.5 has no sendfile.
func sendfile(outfd int, infd int, offset *int64, count int) (written int, err error) {
	return -1, ENOSYS
}

func Utimes(path string, tv []Timeval) (err error) {
	if len(tv) != 2 {
		return EINVAL
	}
	for _, t := range tv {
		if t.Usec < 0 || t.Usec >= 1e6 {
			return EINVAL
		}
	}
	return utimes(path, (*[2]Timeval)(unsafe.Pointer(&tv[0])))
}

// timeFits reports whether sec can be stored in QNX's time_t, an
// unsigned 32-bit count of seconds since 1970.
func timeFits(sec int64) bool {
	return sec >= 0 && sec <= 1<<32-1
}

// utimeOmit and utimeNow are the Linux UTIME_OMIT and UTIME_NOW values;
// internal/syscall/unix.UTIME_OMIT for QNX matches utimeOmit. A Timespec
// with Nsec utimeOmit leaves that time unchanged, and with utimeNow sets
// it to the current time.
const (
	utimeOmit = (1 << 30) - 2
	utimeNow  = (1 << 30) - 1
)

// UtimesNano sets the times with microsecond precision: QNX 6.5 has no
// utimensat. A time whose Nsec is UTIME_OMIT keeps its current value,
// which is read with stat first (so this is not atomic), and UTIME_NOW
// means the current time. Times that do not fit QNX's unsigned 32-bit
// time_t (before 1970 or after 2106) fail with EINVAL rather than wrap.
func UtimesNano(path string, ts []Timespec) error {
	if len(ts) != 2 {
		return EINVAL
	}
	var tv [2]Timeval
	if ts[0].Nsec == utimeOmit || ts[1].Nsec == utimeOmit {
		var st Stat_t
		if err := Stat(path, &st); err != nil {
			return err
		}
		tv[0] = Timeval{Sec: st.Atime}
		tv[1] = Timeval{Sec: st.Mtime}
	}
	for i := range ts {
		switch ts[i].Nsec {
		case utimeOmit:
		case utimeNow:
			if err := Gettimeofday(&tv[i]); err != nil {
				return err
			}
		default:
			// setTimespec marks times outside time_t's range
			// with an invalid Nsec.
			if ts[i].Nsec < 0 || ts[i].Nsec >= 1e9 {
				return EINVAL
			}
			tv[i] = Timeval{Sec: ts[i].Sec, Usec: ts[i].Nsec / 1e3}
		}
	}
	return utimes(path, (*[2]Timeval)(unsafe.Pointer(&tv[0])))
}

// Futimes is not supported: QNX 6.5 has futime, with one-second
// precision, but no futimes.
func Futimes(fd int, tv []Timeval) (err error) {
	return ENOSYS
}

/*
 * Sockets (libsocket.so.3). QNX's io-pkt stack is derived from
 * NetBSD, so the socket addresses have BSD length bytes.
 */

// The socket functions below are imported from libsocket.so.3, which the
// linker only lists as DT_NEEDED when asked to, as with the runtime's
// libc.so.3.
//
//go:cgo_import_dynamic _ _ "libsocket.so.3"

type SockaddrDatalink struct {
	Len    uint8
	Family uint8
	Index  uint16
	Type   uint8
	Nlen   uint8
	Alen   uint8
	Slen   uint8
	Data   [12]int8
	raw    RawSockaddrDatalink
}

//sys	accept(s int, rsa *RawSockaddrAny, addrlen *_Socklen) (fd int, err error)
//sys	bindLibc(s int, addr unsafe.Pointer, addrlen _Socklen) (err error) = SYS_bind
//sys	connect(s int, addr unsafe.Pointer, addrlen _Socklen) (err error)
//sysnb	socket(domain int, typ int, proto int) (fd int, err error)
//sys	getsockopt(s int, level int, name int, val unsafe.Pointer, vallen *_Socklen) (err error)
//sys	setsockopt(s int, level int, name int, val unsafe.Pointer, vallen uintptr) (err error)
//sysnb	getpeername(fd int, rsa *RawSockaddrAny, addrlen *_Socklen) (err error)
//sysnb	getsockname(fd int, rsa *RawSockaddrAny, addrlen *_Socklen) (err error)
//sys	Shutdown(s int, how int) (err error)
//sys	Listen(s int, backlog int) (err error)
//sysnb	socketpair(domain int, typ int, proto int, fd *[2]int32) (err error)
//sys	recvfrom(fd int, p []byte, flags int, from *RawSockaddrAny, fromlen *_Socklen) (n int, err error)
//sys	sendto(s int, buf []byte, flags int, to unsafe.Pointer, addrlen _Socklen) (err error)
//sys	recvmsg(s int, msg *Msghdr, flags int) (n int, err error)
//sys	sendmsg(s int, msg *Msghdr, flags int) (n int, err error)

func (sa *SockaddrInet4) sockaddr() (unsafe.Pointer, _Socklen, error) {
	if sa.Port < 0 || sa.Port > 0xFFFF {
		return nil, 0, EINVAL
	}
	sa.raw.Len = SizeofSockaddrInet4
	sa.raw.Family = AF_INET
	p := (*[2]byte)(unsafe.Pointer(&sa.raw.Port))
	p[0] = byte(sa.Port >> 8)
	p[1] = byte(sa.Port)
	sa.raw.Addr = sa.Addr
	return unsafe.Pointer(&sa.raw), _Socklen(sa.raw.Len), nil
}

func (sa *SockaddrInet6) sockaddr() (unsafe.Pointer, _Socklen, error) {
	if sa.Port < 0 || sa.Port > 0xFFFF {
		return nil, 0, EINVAL
	}
	sa.raw.Len = SizeofSockaddrInet6
	sa.raw.Family = AF_INET6
	p := (*[2]byte)(unsafe.Pointer(&sa.raw.Port))
	p[0] = byte(sa.Port >> 8)
	p[1] = byte(sa.Port)
	sa.raw.Scope_id = sa.ZoneId
	sa.raw.Addr = sa.Addr
	return unsafe.Pointer(&sa.raw), _Socklen(sa.raw.Len), nil
}

func (sa *SockaddrUnix) sockaddr() (unsafe.Pointer, _Socklen, error) {
	name := sa.Name
	n := len(name)
	if n >= len(sa.raw.Path) || n == 0 {
		return nil, 0, EINVAL
	}
	sa.raw.Len = byte(3 + n) // 2 for Family, Len; 1 for NUL
	sa.raw.Family = AF_UNIX
	for i := 0; i < n; i++ {
		sa.raw.Path[i] = int8(name[i])
	}
	return unsafe.Pointer(&sa.raw), _Socklen(sa.raw.Len), nil
}

func (sa *SockaddrDatalink) sockaddr() (unsafe.Pointer, _Socklen, error) {
	if sa.Index == 0 {
		return nil, 0, EINVAL
	}
	sa.raw.Len = sa.Len
	sa.raw.Family = AF_LINK
	sa.raw.Index = sa.Index
	sa.raw.Type = sa.Type
	sa.raw.Nlen = sa.Nlen
	sa.raw.Alen = sa.Alen
	sa.raw.Slen = sa.Slen
	sa.raw.Data = sa.Data
	return unsafe.Pointer(&sa.raw), SizeofSockaddrDatalink, nil
}

func anyToSockaddr(rsa *RawSockaddrAny) (Sockaddr, error) {
	switch rsa.Addr.Family {
	case AF_LINK:
		pp := (*RawSockaddrDatalink)(unsafe.Pointer(rsa))
		sa := new(SockaddrDatalink)
		sa.Len = pp.Len
		sa.Family = pp.Family
		sa.Index = pp.Index
		sa.Type = pp.Type
		sa.Nlen = pp.Nlen
		sa.Alen = pp.Alen
		sa.Slen = pp.Slen
		sa.Data = pp.Data
		return sa, nil

	case AF_UNIX:
		pp := (*RawSockaddrUnix)(unsafe.Pointer(rsa))
		if pp.Len < 2 || pp.Len > SizeofSockaddrUnix {
			return nil, EINVAL
		}
		sa := new(SockaddrUnix)

		// Some BSDs include the trailing NUL in the length, whereas
		// others do not. Work around this by subtracting the leading
		// family and len. The path is then scanned to see if a NUL
		// terminator still exists within the length.
		n := int(pp.Len) - 2 // subtract leading Family, Len
		for i := 0; i < n; i++ {
			if pp.Path[i] == 0 {
				// found early NUL; assume Len included the NUL
				// or was overestimating.
				n = i
				break
			}
		}
		sa.Name = string(unsafe.Slice((*byte)(unsafe.Pointer(&pp.Path[0])), n))
		// QNX resolves a socket's path from the root, whatever the
		// working directory, and reports it without the leading
		// slash. Measured on QNX 6.5.0: a socket bound to /tmp/s was
		// named "tmp/s"; one bound to "s" with /tmp as the working
		// directory was created as /s, was named "s", and connecting
		// to /tmp/s failed. So every name is from the root: restore
		// the slash. An unbound socket's name is empty and stays so.
		if n > 0 && sa.Name[0] != '/' {
			sa.Name = "/" + sa.Name
		}
		return sa, nil

	case AF_INET:
		pp := (*RawSockaddrInet4)(unsafe.Pointer(rsa))
		sa := new(SockaddrInet4)
		p := (*[2]byte)(unsafe.Pointer(&pp.Port))
		sa.Port = int(p[0])<<8 + int(p[1])
		sa.Addr = pp.Addr
		return sa, nil

	case AF_INET6:
		pp := (*RawSockaddrInet6)(unsafe.Pointer(rsa))
		sa := new(SockaddrInet6)
		p := (*[2]byte)(unsafe.Pointer(&pp.Port))
		sa.Port = int(p[0])<<8 + int(p[1])
		sa.ZoneId = pp.Scope_id
		sa.Addr = pp.Addr
		return sa, nil
	}
	return nil, EAFNOSUPPORT
}

func Accept(fd int) (nfd int, sa Sockaddr, err error) {
	var rsa RawSockaddrAny
	var len _Socklen = SizeofSockaddrAny
	err = retryEINTR(func() error {
		var e error
		nfd, e = accept(fd, &rsa, &len)
		return e
	})
	if err != nil {
		return
	}
	sa, err = anyToSockaddr(&rsa)
	if err != nil {
		Close(nfd)
		nfd = 0
	}
	return
}

func Getsockname(fd int) (sa Sockaddr, err error) {
	var rsa RawSockaddrAny
	var len _Socklen = SizeofSockaddrAny
	if err = getsockname(fd, &rsa, &len); err != nil {
		return
	}
	return anyToSockaddr(&rsa)
}

func GetsockoptByte(fd, level, opt int) (value byte, err error) {
	var n byte
	vallen := _Socklen(1)
	err = getsockopt(fd, level, opt, unsafe.Pointer(&n), &vallen)
	return n, err
}

func GetsockoptInet4Addr(fd, level, opt int) (value [4]byte, err error) {
	vallen := _Socklen(4)
	err = getsockopt(fd, level, opt, unsafe.Pointer(&value[0]), &vallen)
	return value, err
}

func GetsockoptIPMreq(fd, level, opt int) (*IPMreq, error) {
	var value IPMreq
	vallen := _Socklen(SizeofIPMreq)
	err := getsockopt(fd, level, opt, unsafe.Pointer(&value), &vallen)
	return &value, err
}

func GetsockoptIPv6Mreq(fd, level, opt int) (*IPv6Mreq, error) {
	var value IPv6Mreq
	vallen := _Socklen(SizeofIPv6Mreq)
	err := getsockopt(fd, level, opt, unsafe.Pointer(&value), &vallen)
	return &value, err
}

func GetsockoptIPv6MTUInfo(fd, level, opt int) (*IPv6MTUInfo, error) {
	var value IPv6MTUInfo
	vallen := _Socklen(SizeofIPv6MTUInfo)
	err := getsockopt(fd, level, opt, unsafe.Pointer(&value), &vallen)
	return &value, err
}

func GetsockoptICMPv6Filter(fd, level, opt int) (*ICMPv6Filter, error) {
	var value ICMPv6Filter
	vallen := _Socklen(SizeofICMPv6Filter)
	err := getsockopt(fd, level, opt, unsafe.Pointer(&value), &vallen)
	return &value, err
}

func recvmsgRaw(fd int, p, oob []byte, flags int, rsa *RawSockaddrAny) (n, oobn int, recvflags int, err error) {
	var msg Msghdr
	msg.Name = (*byte)(unsafe.Pointer(rsa))
	msg.Namelen = uint32(SizeofSockaddrAny)
	var iov Iovec
	if len(p) > 0 {
		iov.Base = &p[0]
		iov.SetLen(len(p))
	}
	var dummy byte
	if len(oob) > 0 {
		// receive at least one normal byte
		if len(p) == 0 {
			iov.Base = &dummy
			iov.SetLen(1)
		}
		msg.Control = &oob[0]
		msg.SetControllen(len(oob))
	}
	msg.Iov = &iov
	msg.Iovlen = 1
	if n, err = recvmsg(fd, &msg, flags); err != nil {
		return
	}
	oobn = int(msg.Controllen)
	recvflags = int(msg.Flags)
	return
}

func sendmsgN(fd int, p, oob []byte, ptr unsafe.Pointer, salen _Socklen, flags int) (n int, err error) {
	var msg Msghdr
	msg.Name = (*byte)(ptr)
	msg.Namelen = uint32(salen)
	var iov Iovec
	if len(p) > 0 {
		iov.Base = &p[0]
		iov.SetLen(len(p))
	}
	var dummy byte
	if len(oob) > 0 {
		// send at least one normal byte
		if len(p) == 0 {
			iov.Base = &dummy
			iov.SetLen(1)
		}
		msg.Control = &oob[0]
		msg.SetControllen(len(oob))
	}
	msg.Iov = &iov
	msg.Iovlen = 1
	if len(oob) > 0 {
		// Control data may carry descriptors (SCM_RIGHTS), which io-pkt
		// passes on through the bracketed path; see runtime/blockop_qnx.go.
		qnxBlockopBegin()
		defer qnxBlockopEnd()
	}
	if n, err = sendmsg(fd, &msg, flags); err != nil {
		return 0, err
	}
	if len(oob) > 0 && len(p) == 0 {
		n = 0
	}
	return n, nil
}

/*
 * Memory
 */

//sys	mmap(addr uintptr, length uintptr, prot int, flag int, fd int, pos int64) (ret uintptr, err error) = SYS_mmap64
//sys	munmap(addr uintptr, length uintptr) (err error)

var mapper = &mmapper{
	active: make(map[*byte][]byte),
	mmap:   mmap,
	munmap: munmap,
}

func Mmap(fd int, offset int64, length int, prot int, flags int) (data []byte, err error) {
	return mapper.Mmap(fd, offset, length, prot, flags)
}

func Munmap(b []byte) (err error) {
	return mapper.Munmap(b)
}

//sys	Mprotect(b []byte, prot int) (err error)

/*
 * Exposed directly
 */
//sys	Access(path string, mode uint32) (err error)
//sys	Chdir(path string) (err error)
//sys	Chmod(path string, mode uint32) (err error)
//sys	Chown(path string, uid int, gid int) (err error)
//sys	Chroot(path string) (err error)
//sys	closeFD(fd int) (err error) = SYS_close
//sys	spawn(path *byte, fdCount int, fdMap *int32, inherit *_Inheritance, argv **byte, envp **byte) (pid int, err error)
//sys	Dup(fd int) (nfd int, err error)
//sys	Dup2(from int, to int) (err error)
//sys	Fchdir(fd int) (err error)
//sys	Fchmod(fd int, mode uint32) (err error)
//sys	Fchown(fd int, uid int, gid int) (err error)
//sys	Flock(fd int, how int) (err error)
// TODO(qnx): Fpathconf, Pathconf and Getpriority can succeed with -1
// (Pathconf without setting errno, for "no limit"), and a stale errno
// then turns success into an error. They need errno cleared before the
// call, which needs a runtime variant of syscall.
//sys	Fpathconf(fd int, name int) (val int, err error)
//sys	Fstat(fd int, stat *Stat_t) (err error) = SYS_fstat64
//sys	Fsync(fd int) (err error)
//sys	Ftruncate(fd int, length int64) (err error) = SYS_ftruncate64
//sysnb	Getegid() (egid int)
//sysnb	Geteuid() (uid int)
//sysnb	Getgid() (gid int)
//sysnb	Getpgid(pid int) (pgid int, err error)
//sysnb	Getpgrp() (pgrp int)
//sysnb	Getpid() (pid int)
//sysnb	Getppid() (ppid int)
//sys	Getpriority(which int, who int) (prio int, err error)
//sysnb	Getrlimit(which int, lim *Rlimit) (err error) = SYS_getrlimit64
//sysnb	Getrusage(who int, rusage *Rusage) (err error)
//sysnb	Getsid(pid int) (sid int, err error)
//sysnb	Gettimeofday(tv *Timeval) (err error)
//sysnb	Getuid() (uid int)
//sys	Kill(pid int, signum Signal) (err error)
//sys	Lchown(path string, uid int, gid int) (err error)
//sys	Link(path string, link string) (err error)
//sys	Lstat(path string, stat *Stat_t) (err error) = SYS_lstat64
//sys	Mkdir(path string, mode uint32) (err error)
//sys	Mkfifo(path string, mode uint32) (err error)
//sys	Mknod(path string, mode uint32, dev int) (err error)
//sys	Nanosleep(time *Timespec, leftover *Timespec) (err error)
//sys	open(path string, mode int, perm uint32) (fd int, err error)
//sys	Pathconf(path string, name int) (val int, err error)
//sys	pread(fd int, p []byte, offset int64) (n int, err error) = SYS_pread64
//sys	pwrite(fd int, p []byte, offset int64) (n int, err error) = SYS_pwrite64
//sys	read(fd int, p []byte) (n int, err error)
//sys	Readlink(path string, buf []byte) (n int, err error)
//sys	Rename(from string, to string) (err error)
//sys	Rmdir(path string) (err error)
//sys	Seek(fd int, offset int64, whence int) (newoffset int64, err error) = SYS_lseek64
//sys	Select(n int, r *FdSet, w *FdSet, e *FdSet, timeout *Timeval) (err error)
//sysnb	Setegid(egid int) (err error)
//sysnb	Seteuid(euid int) (err error)
//sysnb	Setgid(gid int) (err error)
//sysnb	Setpgid(pid int, pgid int) (err error)
//sys	Setpriority(which int, who int, prio int) (err error)
//sysnb	Setregid(rgid int, egid int) (err error)
//sysnb	Setreuid(ruid int, euid int) (err error)
//sysnb	setrlimit(which int, lim *Rlimit) (err error) = SYS_setrlimit64
//sysnb	Setsid() (pid int, err error)
//sysnb	Settimeofday(tp *Timeval) (err error)
//sysnb	Setuid(uid int) (err error)
//sysnb	Uname(buf *Utsname) (err error) = SYS_uname
//sys	Stat(path string, stat *Stat_t) (err error) = SYS_stat64
//sys	Symlink(path string, link string) (err error)
//sys	Sync() (err error)
//sys	Truncate(path string, length int64) (err error) = SYS_truncate64
//sys	Umask(newmask int) (oldmask int)
//sys	Unlink(path string) (err error)
//sys	write(fd int, p []byte) (n int, err error)
//sys	writev(fd int, iovecs []Iovec) (n uintptr, err error)
//sys	readlen(fd int, buf *byte, nbuf int) (n int, err error) = SYS_read
//sys	utimes(path string, timeval *[2]Timeval) (err error)
//sys	fcntl(fd int, cmd int, arg int) (val int, err error)
//sys	fcntlPtr(fd int, cmd int, arg unsafe.Pointer) (val int, err error) = SYS_fcntl
//sysnb	ioctl(fd int, req int, arg int) (err error)
//sysnb	ioctlPtr(fd int, req uint, arg unsafe.Pointer) (err error) = SYS_ioctl
//sysnb	pipe(p *[2]_C_int) (err error)
//sysnb	execve(path *byte, argv **byte, envp **byte) (err error)
//sysnb	exit(res int) (err error) = SYS__exit

// Implemented in the runtime package (runtime/blockop_qnx.go): they
// bracket the socket calls that io-pkt can crash on if the calling
// thread stops waiting, by a signal or by an exit.
func qnxBlockopBegin()
func qnxBlockopEnd()

// bind brackets the binding of a Unix socket to a name; see
// runtime/blockop_qnx.go. Other families are not affected.
func bind(s int, addr unsafe.Pointer, addrlen _Socklen) error {
	if addrlen < 2 || (*RawSockaddr)(addr).Family != AF_UNIX {
		return bindLibc(s, addr, addrlen)
	}
	qnxBlockopBegin()
	defer qnxBlockopEnd()
	return bindLibc(s, addr, addrlen)
}
