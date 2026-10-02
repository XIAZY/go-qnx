// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

// time_t is an unsigned 32-bit integer on QNX 6.5 (see <sys/target_nto.h>),
// so Sec is a uint32: times before 1970 cannot be represented, and times
// up to 2106 can. Converting back with int64(ts.Sec) never sign-extends.

// setTimespec and setTimeval cannot return an error. When sec does not
// fit time_t they return an invalid nanosecond (microsecond) field
// instead of a wrapped date, so that UtimesNano, and the C library for
// everything else, reject the value with EINVAL.
func setTimespec(sec, nsec int64) Timespec {
	if !timeFits(sec) {
		return Timespec{Nsec: -1}
	}
	return Timespec{Sec: uint32(sec), Nsec: int32(nsec)}
}

func setTimeval(sec, usec int64) Timeval {
	if !timeFits(sec) {
		return Timeval{Usec: -1}
	}
	return Timeval{Sec: uint32(sec), Usec: int32(usec)}
}

func (iov *Iovec) SetLen(length int) {
	iov.Len = uint32(length)
}

func (msghdr *Msghdr) SetControllen(length int) {
	msghdr.Controllen = uint32(length)
}

func (cmsg *Cmsghdr) SetLen(length int) {
	cmsg.Len = uint32(length)
}
