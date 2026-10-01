// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
	"unsafe"
)

// The *_trampoline functions convert from the Go calling convention to
// the C calling convention and then call the libc function. They are
// defined in sys_qnx_$ARCH.s. A trampoline that reports an errno reads
// it immediately after the call, before anything else can overwrite it.

//go:nosplit
//go:cgo_unsafe_args
func pthread_attr_init(attr *pthreadattr) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(pthread_attr_init_trampoline)), unsafe.Pointer(&attr))
	KeepAlive(attr)
	return ret
}
func pthread_attr_init_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func pthread_attr_destroy(attr *pthreadattr) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(pthread_attr_destroy_trampoline)), unsafe.Pointer(&attr))
	KeepAlive(attr)
	return ret
}
func pthread_attr_destroy_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func pthread_attr_getstacksize(attr *pthreadattr, size *uintptr) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(pthread_attr_getstacksize_trampoline)), unsafe.Pointer(&attr))
	KeepAlive(attr)
	KeepAlive(size)
	return ret
}
func pthread_attr_getstacksize_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func pthread_attr_setstacksize(attr *pthreadattr, size uintptr) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(pthread_attr_setstacksize_trampoline)), unsafe.Pointer(&attr))
	KeepAlive(attr)
	return ret
}
func pthread_attr_setstacksize_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func pthread_attr_setdetachstate(attr *pthreadattr, state int) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(pthread_attr_setdetachstate_trampoline)), unsafe.Pointer(&attr))
	KeepAlive(attr)
	return ret
}
func pthread_attr_setdetachstate_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func pthread_create(attr *pthreadattr, start uintptr, arg unsafe.Pointer) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(pthread_create_trampoline)), unsafe.Pointer(&attr))
	KeepAlive(attr)
	KeepAlive(arg) // Just for consistency. Arg of course needs to be kept alive for the start function.
	return ret
}
func pthread_create_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func pthread_self() (t pthread) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(pthread_self_trampoline)), unsafe.Pointer(&t))
	return
}
func pthread_self_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func pthread_kill(t pthread, sig int32) int32 {
	return libcCall(unsafe.Pointer(abi.FuncPCABI0(pthread_kill_trampoline)), unsafe.Pointer(&t))
}
func pthread_kill_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func sem_init(sem *semt, pshared int32, value uint32) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(sem_init_trampoline)), unsafe.Pointer(&sem))
	KeepAlive(sem)
	return ret
}
func sem_init_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func sem_wait(sem *semt) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(sem_wait_trampoline)), unsafe.Pointer(&sem))
	KeepAlive(sem)
	return ret
}
func sem_wait_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func sem_timedwait_monotonic(sem *semt, abstime *timespec) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(sem_timedwait_monotonic_trampoline)), unsafe.Pointer(&sem))
	KeepAlive(sem)
	KeepAlive(abstime)
	return ret
}
func sem_timedwait_monotonic_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func sem_post(sem *semt) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(sem_post_trampoline)), unsafe.Pointer(&sem))
	KeepAlive(sem)
	return ret
}
func sem_post_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func sem_destroy(sem *semt) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(sem_destroy_trampoline)), unsafe.Pointer(&sem))
	KeepAlive(sem)
	return ret
}
func sem_destroy_trampoline()

//go:nosplit
func osyield() {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(sched_yield_trampoline)), unsafe.Pointer(nil))
}
func sched_yield_trampoline()

//go:nosplit
func osyield_no_g() {
	asmcgocall_no_g(unsafe.Pointer(abi.FuncPCABI0(sched_yield_trampoline)), unsafe.Pointer(nil))
}

// This is exported via linkname to assembly in runtime/cgo.
//
//go:linkname exit
//go:nosplit
//go:cgo_unsafe_args
func exit(code int32) {
	// libc's exit runs atexit functions and C destructors, which may
	// call back into Go (see TestDestructorCallback). As cgocall does,
	// tell the scheduler that this goroutine is leaving Go, so that a
	// callback finds the syscall state that cgocallbackg expects.
	//
	// Only do so from an ordinary goroutine that holds a P. exit is
	// also called on crash paths (throw and fatalpanic, on g0 via
	// systemstack) and from signal handlers, where entersyscall would
	// itself be fatal and Go cannot take callbacks. There call _exit,
	// which runs no C code, as a Linux program's exit_group runs none.
	if iscgo {
		if gp := getg(); gp != nil && gp.m != nil && gp == gp.m.curg && gp.m.p != 0 && gp.m.locks == 0 && gp.syscallsp == 0 {
			entersyscall()
		} else {
			libcCall(unsafe.Pointer(abi.FuncPCABI0(_exit_trampoline)), unsafe.Pointer(&code))
		}
	}
	libcCall(unsafe.Pointer(abi.FuncPCABI0(exit_trampoline)), unsafe.Pointer(&code))
}
func exit_trampoline()
func _exit_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func getpid() (pid int32) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(getpid_trampoline)), unsafe.Pointer(&pid))
	return
}
func getpid_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func raiseproc(sig uint32) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(raiseproc_trampoline)), unsafe.Pointer(&sig))
}
func raiseproc_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func getuid() (id int32) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(getuid_trampoline)), unsafe.Pointer(&id))
	return
}
func getuid_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func geteuid() (id int32) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(geteuid_trampoline)), unsafe.Pointer(&id))
	return
}
func geteuid_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func getgid() (id int32) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(getgid_trampoline)), unsafe.Pointer(&id))
	return
}
func getgid_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func getegid() (id int32) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(getegid_trampoline)), unsafe.Pointer(&id))
	return
}
func getegid_trampoline()

// QNX has no SA_RESTART: a signal interrupts any call that is a message
// to a resource manager or to procnto's memory manager with EINTR, even
// with SA_RESTART set. mmap, munmap, posix_madvise, open, read, write,
// pipe and fcntl below retry on EINTR, as the kernel would restart them
// elsewhere; close is never retried, and poll, usleep, the semaphores and
// the signal calls are left as they are. syscall's wrappers do the same
// (see mksyscall.pl).

// mmap is used to do low-level memory allocation via mmap. Don't allow stack
// splits, since this function (used by sysAlloc) is called in a lot of low-level
// parts of the runtime and callers often assume it won't acquire any locks.
//
//go:nosplit
func mmap(addr unsafe.Pointer, n uintptr, prot, flags, fd int32, off uint32) (unsafe.Pointer, int) {
	args := struct {
		addr            unsafe.Pointer
		n               uintptr
		prot, flags, fd int32
		off             uint32
		ret1            unsafe.Pointer
		ret2            int
	}{addr, n, prot, flags, fd, off, nil, 0}
	libcCall(unsafe.Pointer(abi.FuncPCABI0(mmap_trampoline)), unsafe.Pointer(&args))
	for args.ret2 == _EINTR { // no SA_RESTART on qnx; see mmap
		libcCall(unsafe.Pointer(abi.FuncPCABI0(mmap_trampoline)), unsafe.Pointer(&args))
	}
	KeepAlive(addr) // Just for consistency. Hopefully addr is not a Go address.
	return args.ret1, args.ret2
}
func mmap_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func munmap(addr unsafe.Pointer, n uintptr) {
	errno := libcCall(unsafe.Pointer(abi.FuncPCABI0(munmap_trampoline)), unsafe.Pointer(&addr))
	for errno == _EINTR { // no SA_RESTART on qnx; see mmap
		errno = libcCall(unsafe.Pointer(abi.FuncPCABI0(munmap_trampoline)), unsafe.Pointer(&addr))
	}
	if errno != 0 {
		throw("runtime: munmap failed")
	}
	KeepAlive(addr) // Just for consistency. Hopefully addr is not a Go address.
}
func munmap_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func posix_madvise(addr unsafe.Pointer, n uintptr, advice int32) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(posix_madvise_trampoline)), unsafe.Pointer(&addr))
	for ret == _EINTR { // no SA_RESTART on qnx; see mmap
		ret = libcCall(unsafe.Pointer(abi.FuncPCABI0(posix_madvise_trampoline)), unsafe.Pointer(&addr))
	}
	KeepAlive(addr) // Just for consistency. Hopefully addr is not a Go address.
	return ret
}
func posix_madvise_trampoline()

// open returns a descriptor, or -1, as on other systems (see TestBadOpen).
// Its trampoline returns -errno, so that EINTR can be retried.
//
//go:nosplit
//go:cgo_unsafe_args
func open(name *byte, mode, perm int32) (ret int32) {
	ret = libcCall(unsafe.Pointer(abi.FuncPCABI0(open_trampoline)), unsafe.Pointer(&name))
	for ret == -_EINTR { // no SA_RESTART on qnx; see mmap
		ret = libcCall(unsafe.Pointer(abi.FuncPCABI0(open_trampoline)), unsafe.Pointer(&name))
	}
	if ret < 0 {
		ret = -1
	}
	KeepAlive(name)
	return
}
func open_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func closefd(fd int32) int32 {
	return libcCall(unsafe.Pointer(abi.FuncPCABI0(close_trampoline)), unsafe.Pointer(&fd))
}
func close_trampoline()

// read returns the number of bytes read, or a negative errno.
//
//go:nosplit
//go:cgo_unsafe_args
func read(fd int32, p unsafe.Pointer, n int32) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(read_trampoline)), unsafe.Pointer(&fd))
	for ret == -_EINTR { // no SA_RESTART on qnx; see mmap
		ret = libcCall(unsafe.Pointer(abi.FuncPCABI0(read_trampoline)), unsafe.Pointer(&fd))
	}
	KeepAlive(p)
	return ret
}
func read_trampoline()

// write1 returns the number of bytes written, or a negative errno.
//
//go:nosplit
//go:cgo_unsafe_args
func write1(fd uintptr, p unsafe.Pointer, n int32) int32 {
	ret := libcCall(unsafe.Pointer(abi.FuncPCABI0(write_trampoline)), unsafe.Pointer(&fd))
	for ret == -_EINTR { // no SA_RESTART on qnx; see mmap
		ret = libcCall(unsafe.Pointer(abi.FuncPCABI0(write_trampoline)), unsafe.Pointer(&fd))
	}
	KeepAlive(p)
	return ret
}
func write_trampoline()

// pipe returns the two descriptors, or an errno.
func pipe() (r, w int32, errno int32) {
	var p [2]int32
	errno = libcCall(unsafe.Pointer(abi.FuncPCABI0(pipe_trampoline)), noescape(unsafe.Pointer(&p)))
	for errno == _EINTR { // no SA_RESTART on qnx; see mmap
		errno = libcCall(unsafe.Pointer(abi.FuncPCABI0(pipe_trampoline)), noescape(unsafe.Pointer(&p)))
	}
	return p[0], p[1], errno
}
func pipe_trampoline()

// poll returns the number of ready descriptors, or -1 and an errno.
//
//go:nosplit
func poll(pfds *pollfd, npfds uint32, timeout int32) (int32, int32) {
	args := struct {
		pfds    unsafe.Pointer
		npfds   uint32
		timeout int32
		ret     int32
		errno   int32
	}{noescape(unsafe.Pointer(pfds)), npfds, timeout, 0, 0}
	libcCall(unsafe.Pointer(abi.FuncPCABI0(poll_trampoline)), unsafe.Pointer(&args))
	KeepAlive(pfds)
	return args.ret, args.errno
}
func poll_trampoline()

// setitimerErrno is the errno of the last failed setitimer, or 0.
var setitimerErrno int32

//go:nosplit
func setitimer(mode int32, new, old *itimerval) {
	args := struct {
		mode     int32
		new, old *itimerval
		errno    int32
	}{mode, new, old, 0}
	libcCall(unsafe.Pointer(abi.FuncPCABI0(setitimer_trampoline)), unsafe.Pointer(&args))
	KeepAlive(new)
	KeepAlive(old)
	setitimerErrno = args.errno
}
func setitimer_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func usleep(usec uint32) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(usleep_trampoline)), unsafe.Pointer(&usec))
}
func usleep_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func usleep_no_g(usec uint32) {
	asmcgocall_no_g(unsafe.Pointer(abi.FuncPCABI0(usleep_trampoline)), unsafe.Pointer(&usec))
}

//go:nosplit
//go:cgo_unsafe_args
func sysconf(name int32) int32 {
	return libcCall(unsafe.Pointer(abi.FuncPCABI0(sysconf_trampoline)), unsafe.Pointer(&name))
}
func sysconf_trampoline()

// serverInfo is QNX's struct _server_info (struct _msg_info).
type serverInfo struct {
	nd    uint32
	srcnd uint32
	pid   int32
	tid   int32
	chid  int32
	_     [7]int32
}

// connectServerInfo returns information about the server at the other
// end of connection coid, which for a file descriptor is the resource
// manager serving it. It returns the coid described, which is not coid
// if coid is not a connection (see serverOf), or -1 on failure.
//
//go:nosplit
//go:cgo_unsafe_args
func connectServerInfo(coid int32, info *serverInfo) int32 {
	args := struct {
		pid, coid int32
		info      *serverInfo
	}{0, coid, info}
	return libcCall(unsafe.Pointer(abi.FuncPCABI0(connectServerInfo_trampoline)), unsafe.Pointer(&args))
}
func connectServerInfo_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func fcntl(fd, cmd, arg int32) (ret int32, errno int32) {
	args := struct {
		fd, cmd, arg int32
		ret, errno   int32
	}{fd, cmd, arg, 0, 0}
	libcCall(unsafe.Pointer(abi.FuncPCABI0(fcntl_trampoline)), unsafe.Pointer(&args))
	for args.errno == _EINTR { // no SA_RESTART on qnx; see mmap
		libcCall(unsafe.Pointer(abi.FuncPCABI0(fcntl_trampoline)), unsafe.Pointer(&args))
	}
	return args.ret, args.errno
}
func fcntl_trampoline()

// clockMonotonic returns clock_gettime(CLOCK_MONOTONIC). nanotime uses it
// when it cannot use the TSC (see tsc_qnx.go).
//
//go:nosplit
func clockMonotonic() int64 {
	var ts timespec
	args := struct {
		clock_id int32
		tp       unsafe.Pointer
	}{_CLOCK_MONOTONIC, unsafe.Pointer(&ts)}
	if errno := libcCall(unsafe.Pointer(abi.FuncPCABI0(clock_gettime_trampoline)), unsafe.Pointer(&args)); errno < 0 {
		// Avoid growing the nosplit stack.
		systemstack(func() {
			println("runtime: errno", -errno)
			throw("clock_gettime failed")
		})
	}
	return int64(ts.tv_sec)*1e9 + int64(ts.tv_nsec)
}
func clock_gettime_trampoline()

//go:nosplit
func walltime() (int64, int32) {
	var ts timespec
	args := struct {
		clock_id int32
		tp       unsafe.Pointer
	}{_CLOCK_REALTIME, unsafe.Pointer(&ts)}
	if errno := libcCall(unsafe.Pointer(abi.FuncPCABI0(clock_gettime_trampoline)), unsafe.Pointer(&args)); errno < 0 {
		// Avoid growing the nosplit stack.
		systemstack(func() {
			println("runtime: errno", -errno)
			throw("clock_gettime failed")
		})
	}
	// time_t is unsigned: no sign extension.
	return int64(ts.tv_sec), int32(ts.tv_nsec)
}

//go:nosplit
//go:cgo_unsafe_args
func sigaction(sig uint32, new *sigactiont, old *sigactiont) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(sigaction_trampoline)), unsafe.Pointer(&sig))
	KeepAlive(new)
	KeepAlive(old)
}
func sigaction_trampoline()

//go:nosplit
//go:cgo_unsafe_args
func sigprocmask(how uint32, new *sigset, old *sigset) {
	// sigprocmask is called from sigsave, which is called from needm.
	// As such, we have to be able to run with no g here.
	asmcgocall_no_g(unsafe.Pointer(abi.FuncPCABI0(sigprocmask_trampoline)), unsafe.Pointer(&how))
	KeepAlive(new)
	KeepAlive(old)
}
func sigprocmask_trampoline()

// The syscall package calls libc through these. fn is the address of
// the libc function; err is errno when the call returned -1.
//
// The X versions expect a 64-bit result in DX:AX and test all of it
// for -1; the others test only AX.

//go:linkname syscall_syscall syscall.syscall
//go:nosplit
//go:cgo_unsafe_args
func syscall_syscall(fn, a1, a2, a3 uintptr) (r1, r2, err uintptr) {
	entersyscall()
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscall)), unsafe.Pointer(&fn))
	exitsyscall()
	return
}
func syscall()

//go:linkname syscall_syscallX syscall.syscallX
//go:nosplit
//go:cgo_unsafe_args
func syscall_syscallX(fn, a1, a2, a3 uintptr) (r1, r2, err uintptr) {
	entersyscall()
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscallX)), unsafe.Pointer(&fn))
	exitsyscall()
	return
}
func syscallX()

// syscallPtr is for functions that return a pointer and report
// failure with NULL, such as getcwd.
//
//go:linkname syscall_syscallPtr syscall.syscallPtr
//go:nosplit
//go:cgo_unsafe_args
func syscall_syscallPtr(fn, a1, a2, a3 uintptr) (r1, r2, err uintptr) {
	entersyscall()
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscallPtr)), unsafe.Pointer(&fn))
	exitsyscall()
	return
}
func syscallPtr()

//go:linkname syscall_syscall6 syscall.syscall6
//go:nosplit
//go:cgo_unsafe_args
func syscall_syscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, err uintptr) {
	entersyscall()
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscall6)), unsafe.Pointer(&fn))
	exitsyscall()
	return
}
func syscall6()

//go:linkname syscall_syscall6X syscall.syscall6X
//go:nosplit
//go:cgo_unsafe_args
func syscall_syscall6X(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, err uintptr) {
	entersyscall()
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscall6X)), unsafe.Pointer(&fn))
	exitsyscall()
	return
}
func syscall6X()

//go:linkname syscall_syscall10 syscall.syscall10
//go:nosplit
//go:cgo_unsafe_args
func syscall_syscall10(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10 uintptr) (r1, r2, err uintptr) {
	entersyscall()
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscall10)), unsafe.Pointer(&fn))
	exitsyscall()
	return
}
func syscall10()

//go:linkname syscall_syscall10X syscall.syscall10X
//go:nosplit
//go:cgo_unsafe_args
func syscall_syscall10X(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10 uintptr) (r1, r2, err uintptr) {
	entersyscall()
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscall10X)), unsafe.Pointer(&fn))
	exitsyscall()
	return
}
func syscall10X()

//go:linkname syscall_rawSyscall syscall.rawSyscall
//go:nosplit
//go:cgo_unsafe_args
func syscall_rawSyscall(fn, a1, a2, a3 uintptr) (r1, r2, err uintptr) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscall)), unsafe.Pointer(&fn))
	return
}

//go:linkname syscall_rawSyscall6 syscall.rawSyscall6
//go:nosplit
//go:cgo_unsafe_args
func syscall_rawSyscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, err uintptr) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscall6)), unsafe.Pointer(&fn))
	return
}

//go:linkname syscall_rawSyscall6X syscall.rawSyscall6X
//go:nosplit
//go:cgo_unsafe_args
func syscall_rawSyscall6X(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, err uintptr) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscall6X)), unsafe.Pointer(&fn))
	return
}

//go:linkname syscall_rawSyscall10X syscall.rawSyscall10X
//go:nosplit
//go:cgo_unsafe_args
func syscall_rawSyscall10X(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10 uintptr) (r1, r2, err uintptr) {
	libcCall(unsafe.Pointer(abi.FuncPCABI0(syscall10X)), unsafe.Pointer(&fn))
	return
}

// syscall_rawVforkExec is the heart of process creation on QNX, which
// refuses fork in a process with more than one thread. It calls vfork on
// the g0 stack. In the child, which shares the parent's memory, it moves
// to the stack stk before touching memory and calls child(arg) there;
// child must be a nosplit function that never returns (it execs or
// exits) and never uses g. The parent's stack, which the child would
// otherwise share, is left untouched. In the parent it returns the
// child's pid, or an errno.
//
//go:linkname syscall_rawVforkExec syscall.rawVforkExec
//go:nosplit
func syscall_rawVforkExec(child, arg, stk uintptr) (pid int, err uintptr) {
	args := struct {
		child, arg, stk uintptr
		pid             int
		err             uintptr
	}{child, arg, stk, 0, 0}
	libcCall(unsafe.Pointer(abi.FuncPCABI0(vforkexec_trampoline)), unsafe.Pointer(&args))
	return args.pid, args.err
}
func vforkexec_trampoline()

// syscall_qnxThreadSigmask returns the M's normal signal mask, which a
// vfork child must restore before it execs; see AfterForkInChild.
//
//go:linkname syscall_qnxThreadSigmask syscall.qnxThreadSigmask
func syscall_qnxThreadSigmask() (lo, hi uint32) {
	m := getg().m.sigmask
	return m.__bits[0], m.__bits[1]
}

// Tell the linker that the libc_* functions are to be found
// in a system library, with the libc_ prefix missing.
// On QNX 6.5 threads, semaphores and the rest all live in libc.so.3.

//go:cgo_import_dynamic libc__init_libc _init_libc "libc.so.3"
//go:cgo_import_dynamic libc___get_errno_ptr __get_errno_ptr "libc.so.3"
//go:cgo_import_dynamic libc_pthread_attr_init pthread_attr_init "libc.so.3"
//go:cgo_import_dynamic libc_pthread_attr_destroy pthread_attr_destroy "libc.so.3"
//go:cgo_import_dynamic libc_pthread_attr_getstacksize pthread_attr_getstacksize "libc.so.3"
//go:cgo_import_dynamic libc_pthread_attr_setstacksize pthread_attr_setstacksize "libc.so.3"
//go:cgo_import_dynamic libc_pthread_attr_setdetachstate pthread_attr_setdetachstate "libc.so.3"
//go:cgo_import_dynamic libc_pthread_create pthread_create "libc.so.3"
//go:cgo_import_dynamic libc_pthread_self pthread_self "libc.so.3"
//go:cgo_import_dynamic libc_pthread_kill pthread_kill "libc.so.3"
//go:cgo_import_dynamic libc_pthread_sigmask pthread_sigmask "libc.so.3"
//go:cgo_import_dynamic libc_sem_init sem_init "libc.so.3"
//go:cgo_import_dynamic libc_sem_wait sem_wait "libc.so.3"
//go:cgo_import_dynamic libc_sem_timedwait_monotonic sem_timedwait_monotonic "libc.so.3"
//go:cgo_import_dynamic libc_sem_post sem_post "libc.so.3"
//go:cgo_import_dynamic libc_sem_destroy sem_destroy "libc.so.3"
//go:cgo_import_dynamic libc_sched_yield sched_yield "libc.so.3"
//go:cgo_import_dynamic libc_exit exit "libc.so.3"
//go:cgo_import_dynamic libc_vfork vfork "libc.so.3"
//go:cgo_import_dynamic libc__exit _exit "libc.so.3"
//go:cgo_import_dynamic libc_getpid getpid "libc.so.3"
//go:cgo_import_dynamic libc_kill kill "libc.so.3"
//go:cgo_import_dynamic libc_getuid getuid "libc.so.3"
//go:cgo_import_dynamic libc_geteuid geteuid "libc.so.3"
//go:cgo_import_dynamic libc_getgid getgid "libc.so.3"
//go:cgo_import_dynamic libc_getegid getegid "libc.so.3"
//go:cgo_import_dynamic libc_mmap mmap "libc.so.3"
//go:cgo_import_dynamic libc_munmap munmap "libc.so.3"
//go:cgo_import_dynamic libc_posix_madvise posix_madvise "libc.so.3"
//go:cgo_import_dynamic libc_open open "libc.so.3"
//go:cgo_import_dynamic libc_close close "libc.so.3"
//go:cgo_import_dynamic libc_read read "libc.so.3"
//go:cgo_import_dynamic libc_write write "libc.so.3"
//go:cgo_import_dynamic libc_clock_gettime clock_gettime "libc.so.3"
//go:cgo_import_dynamic libc_pipe pipe "libc.so.3"
//go:cgo_import_dynamic libc_poll poll "libc.so.3"
//go:cgo_import_dynamic libc_setitimer setitimer "libc.so.3"
//go:cgo_import_dynamic libc_usleep usleep "libc.so.3"
//go:cgo_import_dynamic libc_sysconf sysconf "libc.so.3"
//go:cgo_import_dynamic libc_ConnectServerInfo ConnectServerInfo "libc.so.3"
//go:cgo_import_dynamic libc_fcntl fcntl "libc.so.3"
//go:cgo_import_dynamic libc_sigaction sigaction "libc.so.3"
//go:cgo_import_dynamic _ _ "libc.so.3"
