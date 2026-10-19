# Release tarballs and container image

This directory builds the release tarballs and the container image
`ghcr.io/xiazy/go-qnx` for this repository's QNX ports. The layout follows
[docker-library/golang](https://github.com/docker-library/golang), which builds
the official `golang` images; the Dockerfile template is derived from theirs
under the BSD-3-Clause licence in [LICENSE](LICENSE), Copyright (c) 2014
Docker, Inc.

## Using the image

It is the official `golang` image's layout with this toolchain: Go in
`/usr/local/go`, `GOPATH=/go`, `GOTOOLCHAIN=local`, on Debian bookworm, for
`linux/amd64` and `linux/arm64`. Tags: `1.27.2` for the toolchain `go1.27.2`,
`1.27` for the latest build of the 1.27 line, and `latest`.

```sh
docker run --rm -v "$PWD:/src" -w /src ghcr.io/xiazy/go-qnx:1.27 \
	sh -c 'CGO_ENABLED=0 GOOS=qnx GOARCH=arm GOARM=7 go build -o app-qnx-arm .'
```

The release tarballs, `go1.27.2.linux-amd64.tar.gz`,
`go1.27.2.linux-arm64.tar.gz` and `go1.27.2.src.tar.gz`, each with a
`.sha256`, are attached to the `go1.27.2` release of this repository and
install like an official Go release. This repository's `go1.27.2` tag is
upstream's go1.27.2 plus the QNX port, so these files share Google's names but
not their bytes; that is why the checksums are published beside them.

Neither contains a QNX or BlackBerry 10 SDK. With `CGO_ENABLED=0` none is
needed; for cgo, mount your own SDK and set `CC` as described in the
top-level README.

## How a release is made

1. `versions.json` pins a tag of this repository and the commit it must point
   at.
2. The tarballs are built from that commit with `make.bash -distpack`,
   independently of CI, and `./update.sh 1.27 <dir>` records their checksums
   and regenerates `1.27/bookworm/Dockerfile`.
3. A pull request that changes this directory or
   `.github/workflows/release-image.yml` builds the tarballs in CI, which must
   reproduce those checksums byte for byte, then builds the image from them
   on amd64 and arm64 hosts. Inside each image it cross-compiles `test/hello`
   for `qnx/386` and `qnx/arm` (`CGO_ENABLED=0 go build -trimpath`); the
   binaries from the two hosts must be identical, and they can be compared
   with builds of the same commit made elsewhere. Nothing is published.
4. Pushing the pinned tag publishes. GitHub reads a tag push's workflow from
   the tagged commit, so the release branch carries a small
   `.github/workflows/release.yml` that calls `release-image.yml` on master.
   The run must be for the pinned tag at the pinned commit. It creates the
   GitHub release with the tarballs, then builds and pushes the image,
   installed from the release assets by checksum. Publishing a release at
   that tag by hand re-runs the checks, and assets a release already has are
   never replaced.
