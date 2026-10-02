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

### QNX Neutrino 6.5 and BlackBerry 10

This is a fork of Go, at https://github.com/XIAZY/go-qnx, with a port to
QNX: QNX Neutrino 6.5.0 on x86 (`GOOS=qnx GOARCH=386`; 6.5.0 SP1 behaves
the same) and BlackBerry 10 — BlackBerry's system derived from QNX of
that era — on ARM (`GOOS=qnx GOARCH=arm`). The port rides on upstream
release branches: build from `release-branch.go1.27`, and `master`
carries the same work on the development tip. The port is not part of
upstream Go, and cmd/dist lists it among the broken ports, so a toolchain
that runs on QNX itself needs make.bash `--force`.

To cross-compile from another system, build the toolchain for that
system as usual and set `GOOS` and `GOARCH`:

	cd src && ./make.bash
	GOOS=qnx GOARCH=386 ../bin/go build hello.go              # QNX 6.5, x86
	GOOS=qnx GOARCH=arm GOARM=7 ../bin/go build hello.go      # BlackBerry 10, ARM

Copy the resulting binary to the target and run it there.

For QNX 6.5 on x86, a toolchain that runs on QNX itself is built the same
way, with `GOOS=qnx GOARCH=386 ./make.bash --force`, and lands in
`bin/qnx_386`; copy the tree to the QNX machine and use that `go` there.
BlackBerry 10 cannot host the toolchain, so build for it only by
cross-compiling, and run the binaries on the device as an ordinary,
non-root user.

cgo on QNX 6.5/x86 needs a `CC` that supports `-std=gnu90` and the
`__atomic` builtins (GCC 4.7 or later, or clang with the QNX headers);
the QNX 6.5 SDP compilers do not, so build with `CGO_ENABLED=0` there.
cgo on BlackBerry 10/ARM works with the BlackBerry 10 SDK's clang as
`CC`, targeting armv7 QNX with soft-float VFP and supplying the QNX
startup files, libraries and compiler-rt builtins explicitly, since the
SDK's clang has no complete QNX sysroot of its own.

To run Go's own tests on a QNX device, set the ssh host of the device; the
go command runs the test binaries there through the exec wrapper it builds
as `bin/go_qnx_arm_exec` and uses by default for qnx/arm:

	GOQNX_EXEC_SSH=<ssh-host> GOOS=qnx GOARCH=arm go test runtime

See `misc/go_qnx_exec/README` for the other `GOQNX_EXEC_*` variables.

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

### Contributing

Go is the work of thousands of contributors. We appreciate your help!

To contribute, please read the contribution guidelines at https://go.dev/doc/contribute.

Note that the Go project uses the issue tracker for bug reports and
proposals only. See https://go.dev/wiki/Questions for a list of
places to ask questions about the Go language.

[rf]: https://reneefrench.blogspot.com/
[cc4-by]: https://creativecommons.org/licenses/by/4.0/
