// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

/*
Input to cgo.

GOARCH=386 go tool cgo -godefs -- -D_FILE_OFFSET_BITS=64 -D_QNX_SOURCE defs_qnx.go

on QNX 6.5, or with CC set to a C compiler for i386 QNX 6.5 elsewhere.

-D_FILE_OFFSET_BITS=64 must be given on the command line: cgo's prolog
includes <stddef.h> before the preamble below, and the QNX headers fix
the width of off_t and ino_t on first inclusion.
*/

package runtime

/*
#include <sys/types.h>
#include <sys/mman.h>
#include <sys/time.h>
#include <sys/neutrino.h>
#include <sys/siginfo.h>
#include <ucontext.h>
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <pthread.h>
#include <semaphore.h>
#include <signal.h>
#include <time.h>
#include <unistd.h>
*/
import "C"

const (
	EINTR     = C.EINTR
	EFAULT    = C.EFAULT
	EAGAIN    = C.EAGAIN
	ENOMEM    = C.ENOMEM
	ENOSYS    = C.ENOSYS
	ETIMEDOUT = C.ETIMEDOUT

	O_RDONLY   = C.O_RDONLY
	O_WRONLY   = C.O_WRONLY
	O_NONBLOCK = C.O_NONBLOCK
	O_CREAT    = C.O_CREAT
	O_TRUNC    = C.O_TRUNC
	O_CLOEXEC  = C.O_CLOEXEC

	F_GETFL    = C.F_GETFL
	F_SETFL    = C.F_SETFL
	F_GETFD    = C.F_GETFD
	F_SETFD    = C.F_SETFD
	FD_CLOEXEC = C.FD_CLOEXEC

	PROT_NONE  = C.PROT_NONE
	PROT_READ  = C.PROT_READ
	PROT_WRITE = C.PROT_WRITE
	PROT_EXEC  = C.PROT_EXEC

	MAP_ANON    = C.MAP_ANON
	MAP_PRIVATE = C.MAP_PRIVATE
	MAP_FIXED   = C.MAP_FIXED
	MAP_NOINIT  = C.MAP_NOINIT

	POSIX_MADV_DONTNEED = C.POSIX_MADV_DONTNEED

	SA_SIGINFO   = C.SA_SIGINFO
	SA_RESETHAND = C.SA_RESETHAND

	SIG_BLOCK   = C.SIG_BLOCK
	SIG_UNBLOCK = C.SIG_UNBLOCK
	SIG_SETMASK = C.SIG_SETMASK

	PTHREAD_CREATE_DETACHED = C.PTHREAD_CREATE_DETACHED

	SIGHUP    = C.SIGHUP
	SIGINT    = C.SIGINT
	SIGQUIT   = C.SIGQUIT
	SIGILL    = C.SIGILL
	SIGTRAP   = C.SIGTRAP
	SIGABRT   = C.SIGABRT
	SIGEMT    = C.SIGEMT
	SIGFPE    = C.SIGFPE
	SIGKILL   = C.SIGKILL
	SIGBUS    = C.SIGBUS
	SIGSEGV   = C.SIGSEGV
	SIGSYS    = C.SIGSYS
	SIGPIPE   = C.SIGPIPE
	SIGALRM   = C.SIGALRM
	SIGTERM   = C.SIGTERM
	SIGUSR1   = C.SIGUSR1
	SIGUSR2   = C.SIGUSR2
	SIGCHLD   = C.SIGCHLD
	SIGPWR    = C.SIGPWR
	SIGWINCH  = C.SIGWINCH
	SIGURG    = C.SIGURG
	SIGPOLL   = C.SIGPOLL
	SIGSTOP   = C.SIGSTOP
	SIGTSTP   = C.SIGTSTP
	SIGCONT   = C.SIGCONT
	SIGTTIN   = C.SIGTTIN
	SIGTTOU   = C.SIGTTOU
	SIGVTALRM = C.SIGVTALRM
	SIGPROF   = C.SIGPROF
	SIGXCPU   = C.SIGXCPU
	SIGXFSZ   = C.SIGXFSZ
	SIGRTMIN  = C.SIGRTMIN
	SIGRTMAX  = C.SIGRTMAX
	NSIG      = C._NSIG

	FPE_INTDIV = C.FPE_INTDIV
	FPE_INTOVF = C.FPE_INTOVF
	FPE_FLTDIV = C.FPE_FLTDIV
	FPE_FLTOVF = C.FPE_FLTOVF
	FPE_FLTUND = C.FPE_FLTUND
	FPE_FLTRES = C.FPE_FLTRES
	FPE_FLTINV = C.FPE_FLTINV
	FPE_FLTSUB = C.FPE_FLTSUB

	BUS_ADRALN = C.BUS_ADRALN
	BUS_ADRERR = C.BUS_ADRERR
	BUS_OBJERR = C.BUS_OBJERR

	SEGV_MAPERR = C.SEGV_MAPERR
	SEGV_ACCERR = C.SEGV_ACCERR

	SI_USER = C.SI_USER

	ITIMER_REAL    = C.ITIMER_REAL
	ITIMER_VIRTUAL = C.ITIMER_VIRTUAL
	ITIMER_PROF    = C.ITIMER_PROF

	CLOCK_REALTIME  = C.CLOCK_REALTIME
	CLOCK_MONOTONIC = C.CLOCK_MONOTONIC

	SC_NPROCESSORS_ONLN = C._SC_NPROCESSORS_ONLN
	SC_PAGESIZE         = C._SC_PAGESIZE
)

type Sigset C.sigset_t
type Siginfo C.siginfo_t
type Sigaction C.struct_sigaction
type StackT C.stack_t

type Timespec C.struct_timespec
type Timeval C.struct_timeval
type Itimerval C.struct_itimerval

type McontextT C.mcontext_t
type UcontextT C.ucontext_t
type Regs C.X86_CPU_REGISTERS

type PthreadAttr C.pthread_attr_t
type SemT C.sem_t
