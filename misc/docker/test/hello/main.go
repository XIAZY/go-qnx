// Command hello is the image's cross-compile smoke test: it is built inside
// the image for qnx/386 and qnx/arm, and the binaries are compared byte for
// byte with builds of the same go-qnx commit made outside the image.
package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Printf("hello from %s/%s, %s\n", runtime.GOOS, runtime.GOARCH, runtime.Version())
}
