// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build qnx

package runtime

import (
	"internal/abi"
	"unsafe"
)

// Kernel calls for the pulse netpoller. Except for ionotify, which is a
// message to the descriptor's resource manager, they return -errno on
// failure.

// QNX 6.5 constants (<sys/neutrino.h>, <sys/iomgr.h>, <sys/siginfo.h>).
const (
	_NTO_CHF_FIXED_PRIORITY = 0x0001
	_NTO_SIDE_CHANNEL       = 0x40000000
	_NTO_TIMEOUT_RECEIVE    = 1 << 5 // 1 << STATE_RECEIVE

	_NOTIFY_ACTION_POLL    = 0x2
	_NOTIFY_ACTION_POLLARM = 0x3
	_NOTIFY_COND_INPUT     = 0x10000000
	_NOTIFY_COND_OUTPUT    = 0x20000000
	_NOTIFY_COND_MASK      = 0xf0000000

	_SIGEV_PULSE              = 4
	_SIGEV_PULSE_PRIO_INHERIT = -1
)

// sigeventPulse is struct sigevent as SIGEV_PULSE_INIT fills it.
type sigeventPulse struct {
	notify   int32
	coid     int32
	value    int32
	code     int16
	priority int16
}

// qnxPulse is struct _pulse.
type qnxPulse struct {
	typ, subtype uint16
	code         int8
	_            [3]uint8
	value        int32
	scoid        int32
}

//go:nosplit
//go:cgo_unsafe_args
func channelCreate(flags uint32) int32 {
	return libcCall(unsafe.Pointer(abi.FuncPCABI0(channelCreate_trampoline)), unsafe.Pointer(&flags))
}
func channelCreate_trampoline()

//go:nosplit
func connectAttach(nd uint32, pid, chid int32, index uint32, flags int32) int32 {
	args := struct {
		nd        uint32
		pid, chid int32
		index     uint32
		flags     int32
	}{nd, pid, chid, index, flags}
	return libcCall(unsafe.Pointer(abi.FuncPCABI0(connectAttach_trampoline)), unsafe.Pointer(&args))
}
func connectAttach_trampoline()

// msgReceivePulse receives one pulse on chid. It returns 0 or -errno.
//
//go:nosplit
func msgReceivePulse(chid int32, p *qnxPulse) int32 {
	args := struct {
		chid  int32
		p     *qnxPulse
		bytes int32
		info  uintptr
	}{chid, p, int32(unsafe.Sizeof(*p)), 0}
	r := libcCall(unsafe.Pointer(abi.FuncPCABI0(msgReceivePulse_trampoline)), unsafe.Pointer(&args))
	KeepAlive(p)
	return r
}
func msgReceivePulse_trampoline()

//go:nosplit
func msgSendPulse(coid, priority, code, value int32) int32 {
	args := struct {
		coid, priority, code, value int32
	}{coid, priority, code, value}
	return libcCall(unsafe.Pointer(abi.FuncPCABI0(msgSendPulse_trampoline)), unsafe.Pointer(&args))
}
func msgSendPulse_trampoline()

// timerTimeoutReceive bounds the calling thread's next receive by ns
// nanoseconds (0: return at once if nothing is queued). It is consumed by
// that kernel call, even if the call is interrupted.
//
//go:nosplit
func timerTimeoutReceive(ns uint64) int32 {
	args := struct {
		id, flags    int32
		notify       uintptr
		ntime, otime *uint64
	}{_CLOCK_MONOTONIC, _NTO_TIMEOUT_RECEIVE, 0, &ns, nil}
	r := libcCall(unsafe.Pointer(abi.FuncPCABI0(timerTimeout_trampoline)), unsafe.Pointer(&args))
	KeepAlive(&ns)
	return r
}
func timerTimeout_trampoline()

// ionotify returns the conditions already met, or -1 and errno. It
// retries on EINTR: there is no SA_RESTART on qnx.
//
//go:nosplit
func ionotify(fd, action, flags int32, ev *sigeventPulse) (int32, int32) {
	args := struct {
		fd, action, flags int32
		ev                *sigeventPulse
		ret, errno        int32
	}{fd, action, flags, ev, 0, 0}
	for {
		libcCall(unsafe.Pointer(abi.FuncPCABI0(ionotify_trampoline)), unsafe.Pointer(&args))
		if args.errno != _EINTR {
			break
		}
	}
	KeepAlive(ev)
	return args.ret, args.errno
}
func ionotify_trampoline()

//go:cgo_import_dynamic libc_ChannelCreate_r ChannelCreate_r "libc.so.3"
//go:cgo_import_dynamic libc_ConnectAttach_r ConnectAttach_r "libc.so.3"
//go:cgo_import_dynamic libc_MsgReceivePulse_r MsgReceivePulse_r "libc.so.3"
//go:cgo_import_dynamic libc_MsgSendPulse_r MsgSendPulse_r "libc.so.3"
//go:cgo_import_dynamic libc_TimerTimeout_r TimerTimeout_r "libc.so.3"
//go:cgo_import_dynamic libc_ionotify ionotify "libc.so.3"
