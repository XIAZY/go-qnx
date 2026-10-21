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

To use a prebuilt toolchain instead of building one, see
[Prebuilt toolchain and container image](#prebuilt-toolchain-and-container-image)
below.

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

`golang.org/x/sys/unix` for QNX lives on the `qnx` branch of
https://github.com/XIAZY/go-sys-qnx, a fork of golang.org/x/sys, and
needs no cgo. Use it with a replace line in your `go.mod`:

	replace golang.org/x/sys => github.com/XIAZY/go-sys-qnx v0.0.0-20261016215730-6348336c2429

It holds what QNX 6.5 has, so there are no `*at` calls, `pipe2`,
`accept4`, `dup3` or `sendfile`, and `ReadDirent` lists only part of a
union directory such as `/` or `/dev` (package `os` lists them whole).

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

On QNX 6.5, the system stops responding until a reboot if one thread of
a process reads a file with `read` into memory it has not touched yet
while another thread of the same process pages in the same file through
a memory mapping: the disk driver, replying into the untouched page, and
the kernel's pager end up waiting on each other. This is known issue
Ref# 62483 in the QNX SDP 6.5.0 release notes. Private and shared
mappings both trigger it, on one CPU and on two; memory that `mmap` has
already committed still counts as untouched; touching the buffer before
the read prevents it. A running program's executable and shared
libraries are such mappings, so a program that reads its own executable
while running from disk can trigger it. A read of a different file into
an untouched buffer can also freeze the system. In testing, reads into
untouched buffers froze it in 7 of 12 runs and reads into touched
buffers in none of 12. A write to a file from an untouched buffer can
freeze it as well: in testing, writes from untouched buffers froze it in
3 of 3 runs, and writes from buffers whose pages had first been read in
none of 3. The order of events inside the kernel has not been shown.

On QNX the Go runtime therefore writes one byte to each page of the
memory it hands out, the heap and goroutine stacks, before the program
can use it. This costs page faults taken earlier, no memory: about 0.4
ms per MB on QNX 6.5 and 1.7 ms per MB on BlackBerry 10, the first time
the memory is used. Thread stacks and C memory under cgo are not
covered, nor is memory a program maps itself with `syscall.Mmap` or
`golang.org/x/sys/unix`. The Go toolchain's commands (the go command,
the compiler and the linker) map files as well; on QNX they read them
instead, and leave telemetry off, since its counter files are shared
through mappings. For other programs there are two workarounds: run them
from `/dev/shmem`, which is memory, so a copy there is lost at reboot;
or start `procnto` with `-mL`, the release notes' workaround, which QNX
documents as making memory resident as soon as it is mapped. In testing,
a file mapped privately without `MAP_LAZY` was read in whole by `mmap`
before it returned and never paged in later. Program text and shared
libraries are mapped shared without `MAP_LAZY`; whether they are read in
whole the same way was not observed. A file mapped with `MAP_LAZY` still
paged in under `-mL`, and with such a file the different-file case did
not freeze in 3 runs, against 5 of 9 without it. Its cost in memory was
not measured for Go programs. BlackBerry 10 has not been tested for the
deadlock; the runtime touches memory there too.

To keep a Go program's own pages off the disk entirely, run it from
`/dev/shmem` with a copy of `/lib/libsocket.so.3` there and
`LD_LIBRARY_PATH=/dev/shmem`; libc came from the boot image, in memory,
on the system tested. Starting the program once with `DL_DEBUG=libs` set
shows where each library is loaded from.

On a BlackBerry Passport running BlackBerry 10 (Broadcom Wi-Fi over
SDIO, driver `devnp-bcm_dhd.so` 1.88.21, firmware 6.38.9.13), the Wi-Fi
transmit path changes one byte of an outgoing frame after the network
stack has computed its TCP checksum. The byte is always the second byte
of the IP packet's last 32-bit word, whatever the packet: data segments
of 64, 143, 524 and 1388 bytes and pure ACKs alike. Its new value varies
with no pattern found. The damage depends on how many connections send
at once, not on the rate: measured over about 200,000 packets each, one
frame in 190,000 with one connection, one in 4,200 to 5,200 with four,
and one in 2,700 with eight. The checksum no longer matches, so a peer
that checks TCP checksums drops the frame and the phone resends it, and
nothing shows. A peer that does not check them takes the damaged bytes:
with TLS the record fails to authenticate and the connection ends with
"bad record MAC" (one public CDN edge was measured to behave this way);
without TLS the data is silently wrong. Nothing in Go is involved: the
bytes Go hands to the system are intact in the phone's own capture,
taken above the driver, and the same TLS test from QNX 6.5 on x86 and
from Linux ran without a rejected record.

Programs that talk to the network from BlackBerry 10 should therefore:
use TLS for anything whose integrity matters, never plain TCP to a peer
that may not check checksums; retry at the application level when a
connection ends with a rejected record or drops in the middle of an
upload, since TLS by design cannot repair a damaged record; and keep the
number of connections sending at the same time small where it matters.

Turning off the driver's packing of several queued frames into one
transfer to the chip stops the rejected records. With the driver setting
`txglomsize` at 1 instead of the shipped 10, the same test ran twice,
200 iterations each, without a rejected record and at the same speed,
where the phone as shipped gave 34 and 25 in runs of 200. As root on the
phone:

	/usr/sbin/dhd_bcm_dhd -i bcm0 txglomsize 1

Running `/usr/sbin/dhd_bcm_dhd -i bcm0 txglomsize` reads it back and
prints `1` (`10 (0xa)` as shipped). It is a runtime driver value, lost
at reboot, so whoever owns the phone has to apply it at boot. This needs
root on the phone; on a phone without it the setting cannot be changed
and only the rules above apply.

#### Prebuilt toolchain and container image

The container image `ghcr.io/xiazy/go-qnx` holds this toolchain. It is
public, for linux/amd64 and linux/arm64, and has the same layout as the
official golang image: Go in `/usr/local/go`, `GOPATH=/go`,
`GOTOOLCHAIN=local`. The tag `1.27.2` never changes, `1.27` follows the
1.27 point releases, and `latest` is the newest release. To build in the
current directory:

	docker run --rm -v "$PWD:/src" -w /src ghcr.io/xiazy/go-qnx:1.27.2 \
		sh -c 'CGO_ENABLED=0 GOOS=qnx GOARCH=arm GOARM=7 go build -o app .'   # BlackBerry 10, ARM
	docker run --rm -v "$PWD:/src" -w /src ghcr.io/xiazy/go-qnx:1.27.2 \
		sh -c 'CGO_ENABLED=0 GOOS=qnx GOARCH=386 go build -o app .'           # QNX 6.5, x86

The same toolchain is on the
[go1.27.2 release](https://github.com/XIAZY/go-qnx/releases/tag/go1.27.2)
as `go1.27.2.linux-amd64.tar.gz` and `go1.27.2.linux-arm64.tar.gz`, with
`go1.27.2.src.tar.gz`, each with a `.sha256` file beside it. Install one
like an official Go release:

	curl -fLO https://github.com/XIAZY/go-qnx/releases/download/go1.27.2/go1.27.2.linux-amd64.tar.gz
	echo "$(curl -fsL https://github.com/XIAZY/go-qnx/releases/download/go1.27.2/go1.27.2.linux-amd64.tar.gz.sha256)  go1.27.2.linux-amd64.tar.gz" | sha256sum -c
	tar -C /usr/local -xzf go1.27.2.linux-amd64.tar.gz

These files have the names of Google's go1.27.2 release but not its
bytes: this repository's go1.27.2 is upstream go1.27.2 plus the QNX port,
and `go version` prints go1.27.2 for both. Pin them by checksum, not by
version, and keep a `GOCACHE` apart from any stock Go. Neither the image
nor the tarballs contain a QNX or BlackBerry 10 SDK, so for cgo mount
your own SDK and set `CC` as described above.

How the release files and the image are built and checked is described
in [misc/docker/README.md](misc/docker/README.md).

#### Changes since go1.27.1

A build from `release-branch.go1.27` reports `go version go1.27.2`, the
same string as Google's release, so keep its `GOCACHE` apart from any stock
Go, as described above. A build from `master` has no VERSION file and reports
a devel version named by its commit, `go1.27-devel_<commit>`. If you build
the toolchain from `master` and copy the tree to another machine without
`.git`, copy `VERSION.cache` too (make.bash writes it), or `go run cmd/dist`
and the tests that invoke it fail.

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
- The go command, the compiler and the linker read files instead of mapping
  them, and telemetry stays off, because of the QNX 6.5 mapping deadlock
  above.
- The runtime writes one byte to each page of heap and stack memory before
  handing it out, because of the same deadlock.

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
