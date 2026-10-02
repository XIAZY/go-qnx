// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program can be used as go_qnx_arm_exec by the Go tool. It runs a
// qnx/arm test or program on a remote QNX device over ssh, as
// go_android_exec does over adb: the Go tool builds the binary on the
// host and invokes this wrapper with the binary and its arguments.
//
// The device is described entirely by the environment, so nothing about
// a particular machine is baked into the Go tree:
//
//	GOQNX_EXEC_SSH      ssh destination (required), e.g. a host defined
//	                    in the user's ssh config.
//	GOQNX_EXEC_RUN      directory the binary is copied to and executed
//	                    from; it must permit execution. Default /tmp.
//	GOQNX_EXEC_TMPDIR   a directory on a real filesystem that the run
//	                    user can write (optional). When set, each run
//	                    gets a subdirectory of it holding a copy of the
//	                    test's package directory, run as the working
//	                    directory, with the test's TMPDIR beside it.
//	                    Without it, the binary runs from GOQNX_EXEC_RUN
//	                    with no package files, and tests that need a real
//	                    directory or their testdata fail visibly.
//	GOQNX_EXEC_USER     run the binary as this user with "on -u"
//	                    (optional); some QNX configurations refuse to
//	                    execute an untrusted file as root.
//	GOQNX_EXEC_STAGE    a directory to copy the binary to before the run
//	                    directory (optional), for devices whose run
//	                    directory cannot be written to directly.
//
// A single ssh ControlMaster connection is reused across invocations, so
// a package's tests are one login rather than one per test binary.
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// exitMarker ends the remote output; it is stripped before the output is
// relayed, and carries the binary's exit status. The token is one no test
// is expected to print, and it is written on its own line so a test that
// ends without a newline does not run into it.
const exitMarker = "__GOQNX_EXIT__="

func main() {
	log.SetFlags(0)
	log.SetPrefix("go_qnx_exec: ")
	code, err := run()
	if err != nil {
		log.Fatal(err)
	}
	os.Exit(code)
}

// config is the device description from the environment.
type config struct {
	ssh    string // GOQNX_EXEC_SSH
	run    string // GOQNX_EXEC_RUN
	tmpdir string // GOQNX_EXEC_TMPDIR
	user   string // GOQNX_EXEC_USER
	stage  string // GOQNX_EXEC_STAGE
	ctl    string // ssh ControlPath socket
}

func configure() (*config, error) {
	c := &config{
		ssh:    os.Getenv("GOQNX_EXEC_SSH"),
		run:    os.Getenv("GOQNX_EXEC_RUN"),
		tmpdir: os.Getenv("GOQNX_EXEC_TMPDIR"),
		user:   os.Getenv("GOQNX_EXEC_USER"),
		stage:  os.Getenv("GOQNX_EXEC_STAGE"),
	}
	if c.ssh == "" {
		return nil, fmt.Errorf("GOQNX_EXEC_SSH is not set")
	}
	if c.run == "" {
		c.run = "/tmp"
	}
	// Reuse one ssh connection per destination for the life of the
	// ControlPersist window, so a package's test binaries share a login.
	c.ctl = filepath.Join(os.TempDir(), "go_qnx_exec-"+sanitize(c.ssh)+".sock")
	return c, nil
}

func sanitize(s string) string {
	return regexp.MustCompile(`[^A-Za-z0-9._-]`).ReplaceAllString(s, "_")
}

func run() (int, error) {
	if len(os.Args) < 2 {
		return 0, fmt.Errorf("usage: %s binary [args...]", os.Args[0])
	}
	c, err := configure()
	if err != nil {
		return 0, err
	}

	// Serialize invocations: the device is one machine and we hold to one
	// session at a time.
	lock, err := os.OpenFile(filepath.Join(os.TempDir(), "go_qnx_exec-lock"), os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return 0, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return 0, err
	}

	bin := os.Args[1]
	args := os.Args[2:]
	id := fmt.Sprintf("go_qnx_exec-%d", os.Getpid())

	runBin := path.Join(c.run, id)
	pidFile := path.Join(c.run, id+".pid")

	// Copy the binary onto the device, via the staging directory if one
	// is configured.
	dst := runBin
	if c.stage != "" {
		dst = path.Join(c.stage, id)
	}
	if err := c.scp(bin, dst); err != nil {
		return 0, err
	}
	if c.stage != "" {
		if err := c.ssh2("cp " + sh(dst) + " " + sh(runBin)); err != nil {
			return 0, err
		}
	}
	if err := c.ssh2("chmod 755 " + sh(runBin)); err != nil {
		return 0, err
	}

	// Decide the working directory and TMPDIR. With GOQNX_EXEC_TMPDIR we
	// copy the package directory (the Go tool runs us in it) so testdata
	// and a real TMPDIR are available; otherwise we run from the run
	// directory with neither.
	cwd := c.run
	tmp := c.run
	var perRun string
	if c.tmpdir != "" {
		perRun = path.Join(c.tmpdir, id)
		cwd = path.Join(perRun, "pkg")
		tmp = path.Join(perRun, "tmp")
		if err := c.ssh2("mkdir -p " + sh(cwd) + " " + sh(tmp)); err != nil {
			return 0, err
		}
		if err := c.copyTree(".", cwd); err != nil {
			return 0, err
		}
		// mkdir and scp ran as the login user; hand the tree to the run
		// user so the test can write its working directory and TMPDIR.
		// Best effort: on a filesystem without ownership the directory is
		// already writable, and where the run user still can't write, its
		// tests fail visibly, which is the honest result.
		if c.user != "" {
			c.ssh2("chown -R " + sh(c.user) + " " + sh(perRun))
		}
	}

	// Clean up at the end, but never remove a directory that holds a Unix
	// socket name: unlinking one can wedge the QNX network stack.
	defer func() {
		c.removeSafely(runBin)
		c.removeSafely(pidFile)
		if c.stage != "" {
			c.removeSafely(dst)
		}
		if perRun != "" {
			c.removeSafely(perRun)
		}
	}()

	// Build the remote command: set the environment, run the binary in
	// the background so its pid can be recorded (and killed on a signal),
	// wait for it, and print the exit marker on its own line.
	var b strings.Builder
	fmt.Fprintf(&b, "cd %s; ", sh(cwd))
	for _, kv := range passEnv() {
		fmt.Fprintf(&b, "export %s; ", kv)
	}
	fmt.Fprintf(&b, "export TMPDIR=%s; ", sh(tmp))
	fmt.Fprintf(&b, "%s", sh(runBin))
	for _, a := range args {
		fmt.Fprintf(&b, " %s", sh(a))
	}
	b.WriteString(" & pid=$!; echo $pid > " + sh(pidFile) + "; wait $pid; st=$?; ")
	fmt.Fprintf(&b, "echo; echo %s$st", exitMarker)

	remote := b.String()
	if c.user != "" {
		remote = "on -u " + sh(c.user) + " sh -c " + sh(remote)
	} else {
		remote = "sh -c " + sh(remote)
	}

	cmd := exec.Command("ssh", append(c.sshArgs(), c.ssh, remote)...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	filter, _ := newExitFilter(os.Stdout)
	cmd.Stdout = filter

	// On a timeout or interrupt, kill the remote process so it does not
	// keep running on the device, then let the deferred cleanup run.
	// go test sends SIGQUIT before killing, so trap that too. The kill
	// targets the process group first (to reach children a test spawned,
	// where the shell made one), then the binary itself.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(sigc)
	go func() {
		<-sigc
		c.ssh2("p=$(cat " + sh(pidFile) + " 2>/dev/null); kill -TERM -$p 2>/dev/null; kill -TERM $p 2>/dev/null")
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	err = cmd.Run()
	code, ferr := filter.finish()
	if ferr != nil {
		// No marker: the run itself failed (e.g. ssh could not connect).
		if err != nil {
			return 0, err
		}
		return 0, ferr
	}
	return code, nil
}

// passEnv returns the environment to set on the device. Only the
// runtime's own knobs and CGO_ENABLED are forwarded. The host's paths —
// GOROOT in particular — are not: they don't exist on the device, and a
// forwarded GOROOT would change runtime.GOROOT() and testenv decisions.
// A test that looks for GOROOT then reports it missing, which is the
// truth on the device.
func passEnv() []string {
	allow := []string{"GODEBUG", "GOTRACEBACK", "GOMAXPROCS", "GOGC", "GOMEMLIMIT", "CGO_ENABLED"}
	var out []string
	for _, k := range allow {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, sh(k+"="+v))
		}
	}
	return out
}

// sshArgs returns the common ssh options, including the reused connection.
func (c *config) sshArgs() []string {
	return []string{
		"-o", "BatchMode=yes",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + c.ctl,
		"-o", "ControlPersist=120",
	}
}

// ssh2 runs a command on the device, with output going to our stderr.
func (c *config) ssh2(remote string) error {
	cmd := exec.Command("ssh", append(c.sshArgs(), c.ssh, remote)...)
	cmd.Stderr = os.Stderr
	if out, err := cmd.Output(); err != nil {
		os.Stderr.Write(out)
		return fmt.Errorf("ssh %q: %v", remote, err)
	}
	return nil
}

// scp copies a local file to the device.
func (c *config) scp(local, remote string) error {
	cmd := exec.Command("scp", append(c.scpArgs(), local, c.ssh+":"+remote)...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp %s: %v", remote, err)
	}
	return nil
}

// copyTree copies the contents of a local directory into a device
// directory, by piping a tar stream to tar on the device. scp -r over
// the SFTP protocol mishandles directory trees on current OpenSSH.
func (c *config) copyTree(localDir, remoteDir string) error {
	tr := exec.Command("tar", "-C", localDir, "-cf", "-", ".")
	tr.Stderr = os.Stderr
	pipe, err := tr.StdoutPipe()
	if err != nil {
		return err
	}
	rx := exec.Command("ssh", append(c.sshArgs(), c.ssh, "cd "+sh(remoteDir)+" && tar -xf -")...)
	rx.Stdin = pipe
	rx.Stderr = os.Stderr
	if err := rx.Start(); err != nil {
		return err
	}
	if err := tr.Run(); err != nil {
		rx.Wait()
		return fmt.Errorf("tar %s: %v", localDir, err)
	}
	if err := rx.Wait(); err != nil {
		return fmt.Errorf("extract into %s: %v", remoteDir, err)
	}
	return nil
}

func (c *config) scpArgs() []string {
	return []string{
		"-q",
		"-o", "BatchMode=yes",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + c.ctl,
		"-o", "ControlPersist=120",
	}
}

// removeSafely removes a device path, unless it is or contains a Unix
// socket name: unlinking a socket name can wedge QNX's network stack. It
// fails closed — if the check can't be made (find missing, path gone),
// or a socket is found, the path is left in place and the reason
// reported, rather than removed on faith.
func (c *config) removeSafely(p string) {
	out, err := c.output("find " + sh(p) + " -type s")
	if err != nil {
		fmt.Fprintf(os.Stderr, "go_qnx_exec: not removing %s: could not check it for socket names: %v\n", p, err)
		return
	}
	if len(bytes.TrimSpace(out)) != 0 {
		fmt.Fprintf(os.Stderr, "go_qnx_exec: leaving %s in place: it holds a socket name, which is unsafe to remove on QNX:\n%s", p, out)
		return
	}
	c.ssh2("rm -rf " + sh(p))
}

// output runs a command on the device and returns its standard output.
func (c *config) output(remote string) ([]byte, error) {
	cmd := exec.Command("ssh", append(c.sshArgs(), c.ssh, remote)...)
	return cmd.Output()
}

// sh quotes s for a POSIX shell.
func sh(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// exitFilter relays output while watching for the exit marker at the end.
type exitFilter struct {
	w   io.Writer
	re  *regexp.Regexp
	buf bytes.Buffer
}

func newExitFilter(w io.Writer) (*exitFilter, string) {
	// The remote prints the marker as "echo; echo MARKER$st", i.e. a
	// newline of its own followed by the marker line. Match and strip the
	// whole sequence, the leading newline included, so the output the Go
	// tool sees is byte-for-byte what the binary wrote.
	seq := "\n" + exitMarker
	var re strings.Builder
	for i := 1; i <= len(seq); i++ {
		fmt.Fprintf(&re, "%s$|", regexp.QuoteMeta(seq[:i]))
	}
	fmt.Fprintf(&re, "%s([0-9]+)\n?$", regexp.QuoteMeta(seq))
	return &exitFilter{w: w, re: regexp.MustCompile(re.String())}, exitMarker
}

func (f *exitFilter) Write(data []byte) (int, error) {
	n := len(data)
	f.buf.Write(data)
	b := f.buf.Bytes()
	match := f.re.FindIndex(b)
	if match == nil {
		_, err := f.w.Write(b)
		f.buf.Reset()
		return n, err
	}
	_, err := f.w.Write(b[:match[0]])
	f.buf.Next(match[0])
	return n, err
}

func (f *exitFilter) finish() (int, error) {
	b := f.buf.Bytes()
	defer f.buf.Reset()
	match := f.re.FindSubmatch(b)
	if len(match) < 2 || match[1] == nil {
		f.w.Write(b)
		return 0, fmt.Errorf("no exit marker in remote output (%q)", string(b))
	}
	code, err := strconv.Atoi(string(match[1]))
	if err != nil {
		f.w.Write(b)
		return 0, fmt.Errorf("bad exit code %q: %v", string(match[1]), err)
	}
	return code, nil
}
