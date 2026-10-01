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

On QNX 6.5, removing the name of a Unix socket that is already closed
can hang the network stack until a reboot; removing the name of an open
socket is safe. Package net removes the names of the sockets it binds
before closing them. If you keep a socket's name past Close (a listener
from FileListener or with SetUnlinkOnClose(false), a socket bound with
syscall.Bind, or one made by another program), remove the name before
closing the socket, never after.

### Contributing

Go is the work of thousands of contributors. We appreciate your help!

To contribute, please read the contribution guidelines at https://go.dev/doc/contribute.

Note that the Go project uses the issue tracker for bug reports and
proposals only. See https://go.dev/wiki/Questions for a list of
places to ask questions about the Go language.

[rf]: https://reneefrench.blogspot.com/
[cc4-by]: https://creativecommons.org/licenses/by/4.0/
