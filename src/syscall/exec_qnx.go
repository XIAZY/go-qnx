// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

import (
	"internal/abi"
	"runtime"
	"sync"
	"unsafe"
)

// QNX 6.5 refuses fork() in a multithreaded process, and every Go
// program has several threads, so processes are created with vfork and
// execve, as with CLONE_VFORK on Linux. spawn() and posix_spawn() are not
// an option: neither can change the child's working directory.

type SysProcAttr struct {
	Chroot     string      // Chroot.
	Credential *Credential // Credential.
	Setsid     bool        // Create session.
	// Setpgid sets the process group ID of the child to Pgid,
	// or, if Pgid == 0, to the new child's process ID.
	Setpgid bool
	// Setctty sets the controlling terminal of the child to
	// file descriptor Ctty. Ctty must be a descriptor number
	// in the child process: an index into ProcAttr.Files.
	// This is only meaningful if Setsid is true.
	Setctty bool
	Noctty  bool // Detach fd 0 from controlling terminal
	Ctty    int  // Controlling TTY fd
	// Foreground places the child process group in the foreground.
	// This implies Setpgid. The Ctty field must be set to
	// the descriptor of the controlling TTY.
	// Unlike Setctty, in this case Ctty must be a descriptor
	// number in the parent process.
	Foreground bool
	Pgid       int // Child's process group ID if Setpgid.
}

// rawVforkExec and qnxThreadSigmask are implemented in the runtime
// (runtime/sys_qnx.go); childcall in asm_qnx_386.s.
func rawVforkExec(child, arg, stk uintptr) (pid int, err uintptr)
func runtime_BeforeFork()
func runtime_AfterFork()
func qnxThreadSigmask() (lo, hi uint32)
func childcall(fn, a1, a2, a3 uintptr) (r1 uintptr, err Errno)

func libc_sigaction_trampoline()
func libc_pthread_sigmask_trampoline()

//go:cgo_import_dynamic libc_sigaction sigaction "libc.so.3"
//go:cgo_import_dynamic libc_pthread_sigmask pthread_sigmask "libc.so.3"

// childStackSize is the part of the vfork child's stack for its own and
// libc's frames. The child only calls libc, up to and including execve.
// execve also builds QNX's spawn message on the stack, and that message
// holds every argument and environment string, so childStack adds room
// for those.
const childStackSize = 32 << 10

// qnxChild is everything the vfork child needs, prepared by the parent.
// The child shares the parent's memory; it may read all of this, and it
// writes only to fd, which the parent does not use again.
type qnxChild struct {
	argv0, chroot, dir *byte
	argv, envv         **byte
	fd                 *int
	nfd                int
	nextfd             int
	pipe               int
	sys                *SysProcAttr
	groups             *_Gid_t
	ngroups            int
	rlim               *Rlimit
	mask               [2]uint32 // signal mask to restore before execve
}

// closeLock orders Close against vfork, whose copy of the descriptor
// table fails if a descriptor is closed during it. Close holds it for
// reading; forkAndExecInChild holds it for writing around vfork. It is
// separate from ForkLock because forkExec closes descriptors while
// holding ForkLock for writing.
//
// internal/poll's socket lock (socklock_qnx.go) is taken before
// closeLock and ForkLock, both shared, so whoever holds closeLock or
// ForkLock for writing, as the vfork below does, must make no socket
// call.
var closeLock sync.RWMutex

// vforkFailHook, if set by a test, is called before each vfork; when it
// returns true, the vfork is not made and fails with EBADF instead.
var vforkFailHook func() bool

// Close closes the file descriptor fd.
func Close(fd int) (err error) {
	closeLock.RLock()
	err = closeFD(fd)
	closeLock.RUnlock()
	return
}

// qnxSigaction is struct sigaction on QNX: handler, flags, mask.
type qnxSigaction struct {
	handler uintptr
	flags   int32
	mask    [2]uint32
}

func forkAndExecInChild(argv0 *byte, argv, envv []*byte, chroot, dir *byte, attr *ProcAttr, sys *SysProcAttr, pipe int) (pid int, err Errno) {
	if spawnable(chroot, dir, attr, sys) {
		return spawnExec(argv0, argv, envv, attr, sys)
	}
	c := &qnxChild{
		argv0:  argv0,
		chroot: chroot,
		dir:    dir,
		argv:   &argv[0],
		envv:   &envv[0],
		pipe:   pipe,
		sys:    sys,
		rlim:   origRlimitNofile.Load(),
	}

	// Guard against side effects of shuffling fds below: make sure
	// nextfd is beyond any currently open files, so that we can't run
	// the risk of overwriting any of them.
	fd := make([]int, len(attr.Files))
	c.nextfd = len(attr.Files)
	for i, ufd := range attr.Files {
		if c.nextfd < int(ufd) {
			c.nextfd = int(ufd)
		}
		fd[i] = int(ufd)
	}
	c.nextfd++
	if len(fd) > 0 {
		c.fd = &fd[0]
		c.nfd = len(fd)
	}

	var groups []_Gid_t
	if cred := sys.Credential; cred != nil && !cred.NoSetGroups && len(cred.Groups) > 0 {
		groups = make([]_Gid_t, len(cred.Groups))
		for i, g := range cred.Groups {
			groups[i] = _Gid_t(g)
		}
		c.groups = &groups[0]
		c.ngroups = len(groups)
	}

	c.mask[0], c.mask[1] = qnxThreadSigmask()
	stk, err1 := childStack(argv, envv)
	if err1 != nil {
		return 0, err1.(Errno)
	}
	defer Munmap(stk)

	// QNX's vfork duplicates the parent's descriptors one by one, and fails
	// with EBADF, creating no child, if another thread creates or closes
	// one meanwhile. Close waits for closeLock, so the vfork holds it.
	// Descriptors that other threads create, or the runtime closes without
	// Close, can still make it fail. In a loop of vforks on QNX 6.5:
	//
	//	other threads                       EBADF          longest run
	//	none                                0 of 1605      -
	//	one blocked in open of a FIFO, 1 s  0 of 806       -
	//	1 opening disk files                270 of 2518    0.8 ms
	//	2 opening disk files                671 of 3401    2.4 ms
	//	4 opening disk files                1126 of 2949   3.5 ms
	//
	// A thread blocked in open does no harm; opens that complete do, and
	// the failures come in runs of a few milliseconds. So retry over time
	// rather than a number of times: four times at once, then after sleeps
	// of 1, 2, 4 and then 8 ms (QNX's clock ticks every 1 ms, so a shorter
	// sleep would last a tick anyway), until the sleeps add up to a second.
	// A sleep lasts at least as long as asked, so that is at least a second
	// of trying. closeLock is released while sleeping, but syscall.forkExec
	// holds ForkLock throughout: no other exec starts, and no descriptor is
	// created through the ForkLock paths, until this one gets its child.
	// That is rare and bounded.
	var e uintptr
	sleep := int64(0) // ns
	slept := int64(0)
	for try := 0; ; try++ {
		closeLock.Lock()
		if vforkFailHook != nil && vforkFailHook() {
			e = uintptr(EBADF)
		} else {
			// About to call vfork. No more allocation or calls of
			// non-assembly functions.
			runtime_BeforeFork()
			pid, e = rawVforkExec(abi.FuncPCABIInternal(qnxChildExec), uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(&stk[len(stk)-8])))
			runtime_AfterFork()
		}
		closeLock.Unlock()
		if e != uintptr(EBADF) || slept >= 1e9 {
			break
		}
		if try < 4 {
			runtime.Gosched()
			continue
		}
		if sleep == 0 {
			sleep = 1e6
		} else if sleep < 8e6 {
			sleep *= 2
		}
		ts := NsecToTimespec(sleep)
		for Nanosleep(&ts, &ts) == EINTR { // no SA_RESTART on qnx
		}
		slept += sleep
	}

	// The child has exec'd or exited: it no longer uses any of this.
	runtime.KeepAlive(c)
	runtime.KeepAlive(fd)
	runtime.KeepAlive(groups)
	runtime.KeepAlive(argv)
	runtime.KeepAlive(envv)
	if e != 0 {
		return 0, Errno(e)
	}
	return pid, 0
}

// spawnable reports whether a child can be started with QNX's spawn instead
// of vfork. spawn gives the child exactly the descriptors in its fd_map, so
// it does not copy the parent's descriptor table and does not race with
// other threads creating and closing descriptors. It has no way to set the
// child's working or root directory, credentials, or controlling terminal,
// or to restore RLIMIT_NOFILE; those use vfork. Nor does it honor
// SPAWN_FDCLOSED: on QNX 6.5 a child slot marked closed is still open in
// the child, so a closed descriptor in attr.Files uses vfork too. Setsid
// with Setpgid uses vfork as well: its child calls setsid and then setpgid,
// which fails because a session leader cannot change its process group, and
// spawn does not document the order in which it applies the two.
func spawnable(chroot, dir *byte, attr *ProcAttr, sys *SysProcAttr) bool {
	if dir != nil || chroot != nil || sys.Credential != nil ||
		sys.Setctty || sys.Noctty || sys.Foreground ||
		(sys.Setsid && sys.Setpgid) || origRlimitNofile.Load() != nil {
		return false
	}
	for _, fd := range attr.Files {
		if fd == ^uintptr(0) {
			return false
		}
	}
	return true
}

// spawnExec starts argv0 with spawn. The child gets attr.Files as its
// descriptors 0, 1, 2, ... and no others, whether or not they are
// close-on-exec in the parent. spawn reports a failure to exec the
// program itself, so the status pipe is not used: the child never has
// its write end, and the parent reads end of file from it.
func spawnExec(argv0 *byte, argv, envv []*byte, attr *ProcAttr, sys *SysProcAttr) (pid int, err Errno) {
	fdMap := make([]int32, len(attr.Files))
	for i, fd := range attr.Files {
		fdMap[i] = int32(fd)
	}
	var mapp *int32
	if len(fdMap) > 0 {
		mapp = &fdMap[0]
	}

	var inh _Inheritance
	// Restore the signal mask the calling thread had before
	// syscall.forkExec blocked signals, as the vfork child does, and
	// give every signal Go handles its default action. Signals the
	// program ignores stay ignored, as across execve.
	lo, hi := qnxThreadSigmask()
	inh.Flags = _SPAWN_SETSIGMASK | _SPAWN_SETSIGDEF
	inh.Sigmask.X__bits = [2]int32{int32(lo), int32(hi)}
	for i := 1; i < 57; i++ {
		if Signal(i) == SIGKILL || Signal(i) == SIGSTOP {
			continue
		}
		var sa qnxSigaction
		_, _, e := syscall(abi.FuncPCABI0(libc_sigaction_trampoline), uintptr(i), 0, uintptr(unsafe.Pointer(&sa)))
		if e == 0 && sa.handler > 1 { // not SIG_DFL or SIG_IGN
			inh.Sigdefault.X__bits[(i-1)/32] |= 1 << ((i - 1) % 32)
		}
	}
	if sys.Setsid {
		inh.Flags |= _SPAWN_SETSID
	}
	if sys.Setpgid {
		inh.Flags |= _SPAWN_SETGROUP
		inh.Pgroup = int32(sys.Pgid) // _SPAWN_NEWPGROUP if 0
	}

	pid, e := spawn(argv0, len(fdMap), mapp, &inh, &argv[0], &envv[0])
	runtime.KeepAlive(fdMap)
	runtime.KeepAlive(argv)
	runtime.KeepAlive(envv)
	if e != nil {
		return 0, e.(Errno)
	}
	return pid, 0
}

// childStack maps the stack for a vfork child that will exec argv with
// environment envv. The child shares the parent's memory, so a child that
// ran off the end of a heap-allocated stack would silently overwrite the
// parent's objects, such as other goroutines' stacks. The stack is
// mapped instead, with an inaccessible guard page below it, so that an
// overflow faults in the child.
func childStack(argv, envv []*byte) ([]byte, error) {
	// What execve copies: each string with its NUL, and each pointer
	// array with its terminating nil (argv and envv already end in nil).
	// Allow twice that, as the message is not the only copy libc makes
	// on the stack.
	args := 0
	for _, list := range [][]*byte{argv, envv} {
		args += len(list) * int(unsafe.Sizeof(list[0]))
		for _, p := range list {
			if p != nil {
				args += cstrlen(p) + 1
			}
		}
	}
	n := childStackSize + 2*args
	page := Getpagesize()
	n = (n+page-1)/page*page + page
	stk, err := Mmap(-1, 0, n, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANON)
	if err != nil {
		return nil, err
	}
	if err := Mprotect(stk[:page], PROT_NONE); err != nil {
		Munmap(stk)
		return nil, err
	}
	return stk, nil
}

// cstrlen returns the length of the NUL-terminated string at p.
func cstrlen(p *byte) int {
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return n
}

// qnxChildExec runs in the vfork child, on its own stack, and never
// returns: it execs, or writes the errno to the pipe and exits. It shares
// the parent's memory and runs while the parent's thread is suspended.
// It must not allocate, take locks, grow its stack or touch g, and it
// calls libc only through childcall, which stays on the current stack.
//
// The child also shares the vforking thread's control block: in a C
// vfork child on QNX 6.5, __tls() and &errno are the parent thread's.
// So the child sees that thread's g in __reserved1 (another reason it
// must never go through asmcgocall), and its failing libc calls set that
// thread's errno. That is harmless: the parent thread stays suspended
// in vfork until the child execs or exits, and afterwards reads errno
// only if vfork itself failed, in which case there was no child.
//
// The child's stack use was measured by painting the stack: 5.4 KB in
// the worst case tried (Setsid, eight groups, twenty extra descriptors,
// Dir, and a successful execve of /bin/sh), against childStackSize.
// If a dup or exec fails, it writes the errno error to pipe. (Pipe is
// close-on-exec so if exec succeeds, it will be closed.)
//
//go:nosplit
//go:norace
func qnxChildExec(c *qnxChild) {
	var (
		err1   Errno
		r1     uintptr
		i      int
		pgrp   _C_int
		sa     qnxSigaction
		sys    = c.sys
		pipe   = c.pipe
		nextfd = c.nextfd
		fd     = unsafe.Slice(c.fd, c.nfd)
	)

	// Give every signal Go handles its default action again, before
	// the mask is restored below: a Go handler must not run in the
	// child, which has no thread of Go's own. exec would reset caught
	// signals anyway; this closes the window before it.
	for i = 1; i < 57; i++ {
		if Signal(i) == SIGKILL || Signal(i) == SIGSTOP {
			continue
		}
		sa = qnxSigaction{}
		_, err1 = childcall(abi.FuncPCABI0(libc_sigaction_trampoline), uintptr(i), 0, uintptr(unsafe.Pointer(&sa)))
		if err1 != 0 || sa.handler <= 1 { // SIG_DFL, SIG_IGN
			continue
		}
		sa = qnxSigaction{}
		childcall(abi.FuncPCABI0(libc_sigaction_trampoline), uintptr(i), uintptr(unsafe.Pointer(&sa)), 0)
	}

	// Session ID
	if sys.Setsid {
		_, err1 = childcall(abi.FuncPCABI0(libc_setsid_trampoline), 0, 0, 0)
		if err1 != 0 {
			goto childerror
		}
	}

	// Set process group
	if sys.Setpgid || sys.Foreground {
		_, err1 = childcall(abi.FuncPCABI0(libc_setpgid_trampoline), 0, uintptr(sys.Pgid), 0)
		if err1 != 0 {
			goto childerror
		}
	}

	if sys.Foreground {
		pgrp = _C_int(sys.Pgid)
		if pgrp == 0 {
			r1, err1 = childcall(abi.FuncPCABI0(libc_getpid_trampoline), 0, 0, 0)
			if err1 != 0 {
				goto childerror
			}
			pgrp = _C_int(r1)
		}

		// Place process group in foreground.
		_, err1 = childcall(abi.FuncPCABI0(libc_ioctl_trampoline), uintptr(sys.Ctty), uintptr(TIOCSPGRP), uintptr(unsafe.Pointer(&pgrp)))
		if err1 != 0 {
			goto childerror
		}
	}

	// Restore the signal mask. We do this after TIOCSPGRP to avoid
	// having the kernel send a SIGTTOU signal to the process group.
	childcall(abi.FuncPCABI0(libc_pthread_sigmask_trampoline), 2 /* SIG_SETMASK */, uintptr(unsafe.Pointer(&c.mask)), 0)

	// Chroot
	if c.chroot != nil {
		_, err1 = childcall(abi.FuncPCABI0(libc_chroot_trampoline), uintptr(unsafe.Pointer(c.chroot)), 0, 0)
		if err1 != 0 {
			goto childerror
		}
	}

	// User and groups
	if cred := sys.Credential; cred != nil {
		if !cred.NoSetGroups {
			_, err1 = childcall(abi.FuncPCABI0(libc_setgroups_trampoline), uintptr(c.ngroups), uintptr(unsafe.Pointer(c.groups)), 0)
			if err1 != 0 {
				goto childerror
			}
		}
		_, err1 = childcall(abi.FuncPCABI0(libc_setgid_trampoline), uintptr(cred.Gid), 0, 0)
		if err1 != 0 {
			goto childerror
		}
		_, err1 = childcall(abi.FuncPCABI0(libc_setuid_trampoline), uintptr(cred.Uid), 0, 0)
		if err1 != 0 {
			goto childerror
		}
	}

	// Chdir
	if c.dir != nil {
		_, err1 = childcall(abi.FuncPCABI0(libc_chdir_trampoline), uintptr(unsafe.Pointer(c.dir)), 0, 0)
		if err1 != 0 {
			goto childerror
		}
	}

	// Pass 1: look for fd[i] < i and move those up above len(fd)
	// so that pass 2 won't stomp on an fd it needs later.
	if pipe < nextfd {
		_, err1 = childcall(abi.FuncPCABI0(libc_dup2_trampoline), uintptr(pipe), uintptr(nextfd), 0)
		if err1 != 0 {
			goto childerror
		}
		_, err1 = childcall(abi.FuncPCABI0(libc_fcntl_trampoline), uintptr(nextfd), F_SETFD, FD_CLOEXEC)
		if err1 != 0 {
			goto childerror
		}
		pipe = nextfd
		nextfd++
	}
	for i = 0; i < len(fd); i++ {
		if fd[i] >= 0 && fd[i] < i {
			if nextfd == pipe { // don't stomp on pipe
				nextfd++
			}
			_, err1 = childcall(abi.FuncPCABI0(libc_dup2_trampoline), uintptr(fd[i]), uintptr(nextfd), 0)
			if err1 != 0 {
				goto childerror
			}
			_, err1 = childcall(abi.FuncPCABI0(libc_fcntl_trampoline), uintptr(nextfd), F_SETFD, FD_CLOEXEC)
			if err1 != 0 {
				goto childerror
			}
			fd[i] = nextfd
			nextfd++
		}
	}

	// Pass 2: dup fd[i] down onto i.
	for i = 0; i < len(fd); i++ {
		if fd[i] == -1 {
			childcall(abi.FuncPCABI0(libc_close_trampoline), uintptr(i), 0, 0)
			continue
		}
		if fd[i] == i {
			// dup2(i, i) won't clear close-on-exec flag.
			_, err1 = childcall(abi.FuncPCABI0(libc_fcntl_trampoline), uintptr(fd[i]), F_SETFD, 0)
			if err1 != 0 {
				goto childerror
			}
			continue
		}
		// The new fd is created NOT close-on-exec,
		// which is exactly what we want.
		_, err1 = childcall(abi.FuncPCABI0(libc_dup2_trampoline), uintptr(fd[i]), uintptr(i), 0)
		if err1 != 0 {
			goto childerror
		}
	}

	// By convention, we don't close-on-exec the fds we are
	// started with, so if len(fd) < 3, close 0, 1, 2 as needed.
	// Programs that know they inherit fds >= 3 will need
	// to set them close-on-exec.
	for i = len(fd); i < 3; i++ {
		childcall(abi.FuncPCABI0(libc_close_trampoline), uintptr(i), 0, 0)
	}

	// Detach fd 0 from tty
	if sys.Noctty {
		_, err1 = childcall(abi.FuncPCABI0(libc_ioctl_trampoline), 0, uintptr(TIOCNOTTY), 0)
		if err1 != 0 {
			goto childerror
		}
	}

	// Set the controlling TTY to Ctty
	if sys.Setctty {
		_, err1 = childcall(abi.FuncPCABI0(libc_ioctl_trampoline), uintptr(sys.Ctty), uintptr(TIOCSCTTY), 0)
		if err1 != 0 {
			goto childerror
		}
	}

	// Restore original rlimit.
	if c.rlim != nil {
		childcall(abi.FuncPCABI0(libc_setrlimit64_trampoline), uintptr(RLIMIT_NOFILE), uintptr(unsafe.Pointer(c.rlim)), 0)
	}

	// Time to exec.
	_, err1 = childcall(abi.FuncPCABI0(libc_execve_trampoline),
		uintptr(unsafe.Pointer(c.argv0)),
		uintptr(unsafe.Pointer(c.argv)),
		uintptr(unsafe.Pointer(c.envv)))

childerror:
	// send error code on pipe
	childcall(abi.FuncPCABI0(libc_write_trampoline), uintptr(pipe), uintptr(unsafe.Pointer(&err1)), unsafe.Sizeof(err1))
	for {
		childcall(abi.FuncPCABI0(libc__exit_trampoline), 253, 0, 0)
	}
}

// forkAndExecFailureCleanup cleans up after an exec failure.
func forkAndExecFailureCleanup(attr *ProcAttr, sys *SysProcAttr) {
	// Nothing to do.
}
