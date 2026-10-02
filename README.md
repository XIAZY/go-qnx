# The Go Programming Language

Go is an open source programming language that makes it easy to build simple,
reliable, and efficient software.

![Gopher image](https://golang.org/doc/gopher/fiveyears.jpg)
*Gopher image by [Renee French][rf], licensed under [Creative Commons 4.0 Attribution license][cc4-by].*

Our canonical Git repository is located at https://go.googlesource.com/go.
There is a mirror of the repository at https://github.com/golang/go.

Unless otherwise noted, the Go source files are distributed under the
BSD-style license found in the LICENSE file.

### Download and Install

#### Binary Distributions

Official binary distributions are available at https://go.dev/dl/.

After downloading a binary release, visit https://go.dev/doc/install
for installation instructions.

#### Install From Source

If a binary distribution is not available for your combination of
operating system and architecture, visit
https://go.dev/doc/install/source
for source installation instructions.

### QNX Neutrino 6.5.0

This tree is Go 1.27.1 (upstream tag go1.27.1, commit 862c888e61) with a
port to QNX Neutrino 6.5.0 on x86, `GOOS=qnx GOARCH=386`. QNX 6.5.0 SP1
behaves the same. The port is not part of upstream Go, and cmd/dist
lists it among the broken ports, so make.bash needs `--force` to build
for it.

To cross-compile from another system, build the toolchain for that
system as usual and set `GOOS` and `GOARCH`:

	cd src && ./make.bash
	GOOS=qnx GOARCH=386 ../bin/go build hello.go

A toolchain that runs on QNX itself is built the same way, with
`GOOS=qnx GOARCH=386 ./make.bash --force`, and lands in `bin/qnx_386`.
Copy the tree to the QNX machine and use that `go` there.

cgo works when `CC` is a C compiler for i386 QNX 6.5 that supports
`-std=gnu90` and the `__atomic` builtins (GCC 4.7 or later, or clang
with the QNX headers); the compilers of the QNX 6.5 SDP do not, so set
`CGO_ENABLED=0` when building on QNX.

On QNX 6.5, removing the name of a Unix socket, whether the socket is
open or closed and whichever program removes it (`rm` included), can
hang io-pkt, the network stack, until a reboot, with a small chance each
time. Sockets without names (socketpair, unbound sockets) did not
trigger it. Programs that create and remove many named Unix sockets
should prefer socketpair or loopback TCP, and where they must remove a
name, remove it while the socket is still open.

On QNX 6.5, io-pkt crashes, taking every socket on the machine with it
until a reboot, if a process stops waiting for one of a few Unix socket
calls before it returns: binding a socket to a name, removing a socket
name, and sending descriptors (SCM_RIGHTS). A process that exits with
such a call in progress on another thread is enough, and so is a signal
that interrupts the call. Go programs block signals during these calls
and wait up to a second for them at exit. Not covered: C code (including
through cgo); a process killed from outside, which has not been tested
but presumably behaves like an exit; and a crash with
`GOTRACEBACK=crash`, which ends the process with SIGABRT instead of an
exit. io-pkt also keeps every socket name until it is removed, about
a thousand at most; after that, binding to a name fails with EMFILE.

On BlackBerry 10, BlackBerry's system derived from QNX of that era, on
ARM, the exit-during-bind crash above reproduces as well. The socket-name
hang did not reproduce there in 35,000 removals (QNX 6.5 hung within
33,000 in every run), but the workaround is kept for it too.

On QNX 6.5 under KVM with 2 virtual CPUs, the SMP kernel can hang, with
every CPU busy and no I/O completing, until a reboot, when processes are
created while the disk is being flushed. A native Go build is such a
load. Booting the uniprocessor kernel (`procnto-instr`) avoided it.
The same load did not hang a four-CPU BlackBerry 10 device, nor QNX
6.5's SMP kernel under QEMU's software emulation, so the hang appears
tied to KVM.

### Contributing

Go is the work of thousands of contributors. We appreciate your help!

To contribute, please read the contribution guidelines at https://go.dev/doc/contribute.

Note that the Go project uses the issue tracker for bug reports and
proposals only. See https://go.dev/wiki/Questions for a list of
places to ask questions about the Go language.

[rf]: https://reneefrench.blogspot.com/
[cc4-by]: https://creativecommons.org/licenses/by/4.0/
