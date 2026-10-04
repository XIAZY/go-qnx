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
upstream Go, and cmd/dist lists it among the broken ports, so make.bash
for a qnx target needs `--force`.

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

To run Go's own tests on a BlackBerry 10 device, build the toolchain for
qnx/arm, which also builds the exec wrapper `bin/go_qnx_arm_exec`; with
`bin` on the `PATH`, the go command runs each test binary on the device
through it, over ssh:

	cd src && GOOS=qnx GOARCH=arm ./make.bash --force
	export PATH=$(pwd)/../bin:$PATH
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

#### Changes since go1.27.1

A build from `release-branch.go1.27` reports `go version
go1.27-devel_<commit>`: the VERSION file is removed on purpose, so the build is
identified by its commit and never shares a build cache with Google's go1.27.1.
If you build the toolchain and copy the tree to another machine without `.git`,
copy `VERSION.cache` too (make.bash writes it), or `go run cmd/dist` and the
tests that invoke it fail.

New:

- The ARM port, `GOOS=qnx GOARCH=arm` (GOARM=7), for QNX Neutrino 6.5-era
  systems including BlackBerry 10.
- cgo for qnx/arm, and the `misc/go_qnx_exec` on-device test runner (both above).

Fixed, on both architectures unless noted:

- Directory listings no longer truncate. os.ReadDir, filepath.WalkDir and the
  rest returned only the first resource manager's entries of a union directory
  (`/`, `/dev`) — `/` gave 14 of 16 on QNX 6.5 and 11 of 31 on BlackBerry 10,
  silently, since release 1. Directories are now read through libc.
- The network poller is rebuilt on QNX pulses and ionotify instead of poll(2),
  which on 6.5 could lose a socket's readiness.
- CPU profiling works; QNX 6.5 has no CPU-time timers, so a profiling thread
  reads each thread's CPU clock and sends SIGPROF.
- nanotime reads the TSC where it is trusted, instead of 6.5's 1 ms
  CLOCK_MONOTONIC.
- A program that stops the world in a loop (such as ReadMemStats) no longer
  starves other goroutines on one CPU: the runtime yields once before each stop.
  Also affected release 1.
- The Unix-socket crash and hang above are guarded; the socket-name lock is
  taken only below release 8 (QNX 6.5), the hang being measured absent on
  BlackBerry 10 (release 8.0.0).
- An HTTP client timeout is reported as the Client.Timeout error, not a bare
  context error, when the clock reads exactly the deadline. General fix.
- Child processes with closed descriptor slots start via spawn, not vfork.

Known limitations, besides the Unix-socket hazards above:

- Timers and sleeps have 1 ms granularity where the clock does (qnx/arm, and
  qnx/386 without a trusted TSC).
- File.Readdir and Readdir(-1) on `/dev` return "resource busy" for an active
  resource manager (such as `/dev/nws`); os.ReadDir lists such entries with an
  unknown type instead.
- There is no `go` tool on BlackBerry 10, so tests that build or run Go programs
  on the device skip there.
- A relative Unix-socket path is resolved from `/`, not the working directory,
  so on BlackBerry 10 as an ordinary user `net.Listen("unix", "name")` fails
  with EACCES; use an absolute path.

Tested:

- BlackBerry 10 (qnx/arm), on device: full `go test -short std` — 377 packages,
  0 failures, 0 hangs; plus cgo's runtime signal/thread cases and
  cmd/cgo/internal/test.
- QNX 6.5.0 (qnx/386), one CPU: full `go test -short std` from prebuilt
  binaries — 377 packages, 0 failures, 0 hangs.
- QNX 6.5.0 (qnx/386), two CPUs under QEMU software emulation: full `go test
  -short std` from prebuilt binaries: 377 packages; 3 timing tests fail under
  emulation's clock, the same with the process pinned to one CPU, and pass on
  real-time hardware.

### Contributing

Go is the work of thousands of contributors. We appreciate your help!

To contribute, please read the contribution guidelines at https://go.dev/doc/contribute.

Note that the Go project uses the issue tracker for bug reports and
proposals only. See https://go.dev/wiki/Questions for a list of
places to ask questions about the Go language.

[rf]: https://reneefrench.blogspot.com/
[cc4-by]: https://creativecommons.org/licenses/by/4.0/
