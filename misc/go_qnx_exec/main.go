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
//	                    user can write (optional). When set, each run gets
//	                    a subdirectory of it holding the test's package
//	                    directory, laid out at its path relative to the
//	                    root (GOROOT or the module root) with the testdata
//	                    of every parent copied too, run as the working
//	                    directory, with the test's TMPDIR beside it. This
//	                    is the go_android_exec layout, so a test reading
//	                    ../testdata finds it. Without it, the binary runs
//	                    from GOQNX_EXEC_RUN with no package files, and
//	                    tests that need a real directory or their testdata
//	                    fail visibly.
//	GOQNX_EXEC_USER     run the binary as this user with "on -u"
//	                    (optional); some QNX configurations refuse to
//	                    execute an untrusted file as root. The test then
//	                    gets HOME set to a fresh directory beside its
//	                    TMPDIR when GOQNX_EXEC_TMPDIR is set, and no HOME
//	                    otherwise, never the login user's.
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
	rmsock bool   // GOQNX_EXEC_RMSOCK=1
	ctl    string // ssh ControlPath socket
}

func configure() (*config, error) {
	c := &config{
		ssh:    os.Getenv("GOQNX_EXEC_SSH"),
		run:    os.Getenv("GOQNX_EXEC_RUN"),
		tmpdir: os.Getenv("GOQNX_EXEC_TMPDIR"),
		user:   os.Getenv("GOQNX_EXEC_USER"),
		stage:  os.Getenv("GOQNX_EXEC_STAGE"),
		rmsock: os.Getenv("GOQNX_EXEC_RMSOCK") == "1",
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
	// Keep id short: it becomes a path component of every test's TMPDIR, and
	// a test that binds a Unix socket there has only sun_path's ~104 bytes to
	// work with. It need only be unique among concurrent runs on the device.
	id := fmt.Sprintf("gq%d", os.Getpid())

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
	home := "" // the test's HOME when it runs as GOQNX_EXEC_USER; see below
	var perRun string
	if c.tmpdir != "" {
		perRun = path.Join(c.tmpdir, id)
		tmp = path.Join(perRun, "tmp")
		if c.user != "" {
			home = path.Join(perRun, "home")
		}
		// Lay the package out at its path relative to the root (GOROOT for
		// a standard-library package, the module root otherwise) under a
		// synthetic "pkg" root, as go_android_exec does, so a test that
		// reads ../testdata finds it at the same relative path. If the
		// layout can't be determined, fall back to a flat copy.
		root := path.Join(perRun, "pkg")
		rel, ok := pkgRelPath()
		if ok {
			cwd = path.Join(root, rel)
		} else {
			cwd = root
		}
		dirs := sh(cwd) + " " + sh(tmp)
		if home != "" {
			dirs += " " + sh(home)
		}
		if err := c.ssh2("mkdir -p " + dirs); err != nil {
			return 0, err
		}
		// The package's own directory, with its testdata.
		if err := c.copyTree(".", cwd); err != nil {
			return 0, err
		}
		// The testdata (and go.mod/go.sum) of every parent up to the root,
		// so a test reaching into a parent's testdata finds it. Only those
		// names are copied, never a whole parent and never the GOROOT.
		if ok {
			if err := c.copyParentTestdata(rel, cwd); err != nil {
				return 0, err
			}
		}
		// The toolchain's bundled time zone database, at its GOROOT-relative
		// path under the synthetic root, so time's hermetic test (which reads
		// ../../lib/time/zoneinfo.zip, not the system zoneinfo) finds it.
		// go_ios_exec copies it into the app bundle for the same reason.
		if err := c.copyZoneinfo(root); err != nil {
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
	// A test run as another user must not see the login user's HOME,
	// which it may not even be able to read. The device may have no
	// password database to look the run user's home up in, so give it a
	// home of its own in the per-run directory, or none at all.
	if c.user != "" {
		if home != "" {
			fmt.Fprintf(&b, "export HOME=%s; ", sh(home))
		} else {
			b.WriteString("unset HOME; ")
		}
	}
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

// pkgRelPath reports where the test's package sits relative to its root:
// the slash path from GOROOT for a standard-library package, or from the
// module root otherwise, e.g. "src/net". It runs the go tool found on
// PATH, the one the go command used to invoke this wrapper, in the current
// directory, which the go command set to the package directory. ok is
// false when the layout can't be determined, and the caller falls back to
// a flat copy.
func pkgRelPath() (rel string, ok bool) {
	goBin := "go"
	if p, err := exec.LookPath("go"); err == nil {
		goBin = p
	}
	// Fields are newline-separated: none of ImportPath, Standard, the
	// module directory or the package directory contains a newline, and
	// unlike NUL a newline is a legal exec argument.
	out, err := exec.Command(goBin, "list", "-e", "-f",
		"{{.ImportPath}}\n{{.Standard}}\n{{with .Module}}{{.Dir}}{{end}}\n{{.Dir}}").Output()
	if err != nil {
		return "", false
	}
	f := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(f) < 4 {
		return "", false
	}
	importPath, std, modDir, dir := f[0], f[1] == "true", f[2], f[3]
	if importPath == "" || importPath == "." {
		return "", false
	}
	if std {
		return path.Join("src", importPath), true
	}
	if modDir != "" {
		r, err := filepath.Rel(modDir, dir)
		if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return "", false
		}
		return filepath.ToSlash(r), true
	}
	return "", false
}

// copyParentTestdata copies the testdata directory and any go.mod/go.sum
// of each parent of the package, up to the root, into the matching parent
// of deviceCwd on the device. rel is the package's slash path from the
// root, as returned by pkgRelPath, and deviceCwd is where the package
// itself was copied; because deviceCwd is nested rel-deep under the
// synthetic root, the ".." paths stay within it. Only testdata, go.mod and
// go.sum are copied, never a whole parent directory.
func (c *config) copyParentTestdata(rel, deviceCwd string) error {
	if rel == "." {
		return nil // the package is the root; it has no parents to copy
	}
	// One level per path element of rel, up to and including the root (the
	// last iteration), so a module root's go.mod, go.sum and testdata are
	// copied too. deviceCwd is nested rel-deep under the synthetic root, so
	// the ".." paths stay within it.
	dir := ""
	for n := strings.Count(rel, "/") + 1; n > 0; n-- {
		dir = path.Join(dir, "..")
		for _, name := range []string{"testdata", "go.mod", "go.sum"} {
			hostPath := filepath.Join(dir, name)
			fi, err := os.Stat(hostPath)
			if err != nil {
				continue
			}
			deviceDir := path.Join(deviceCwd, dir)
			if fi.IsDir() {
				target := path.Join(deviceDir, name)
				if err := c.ssh2("mkdir -p " + sh(target)); err != nil {
					return err
				}
				if err := c.copyTree(hostPath, target); err != nil {
					return err
				}
			} else {
				if err := c.ssh2("mkdir -p " + sh(deviceDir)); err != nil {
					return err
				}
				if err := c.scp(hostPath, path.Join(deviceDir, name)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// copyZoneinfo places the toolchain's lib/time/zoneinfo.zip at the matching
// path under the synthetic root, so a standard-library test that reads it by a
// GOROOT-relative path (time's initTestingZone) finds it. The file is small,
// so it is copied unconditionally, as go_ios_exec does. A missing GOROOT or
// file is not an error; the test then skips or fails on its own.
func (c *config) copyZoneinfo(root string) error {
	goBin := "go"
	if p, err := exec.LookPath("go"); err == nil {
		goBin = p
	}
	out, err := exec.Command(goBin, "env", "GOROOT").Output()
	if err != nil {
		return nil
	}
	goroot := strings.TrimSpace(string(out))
	if goroot == "" {
		return nil
	}
	local := filepath.Join(goroot, "lib", "time", "zoneinfo.zip")
	if _, err := os.Stat(local); err != nil {
		return nil
	}
	deviceDir := path.Join(root, "lib", "time")
	if err := c.ssh2("mkdir -p " + sh(deviceDir)); err != nil {
		return err
	}
	return c.scp(local, path.Join(deviceDir, "zoneinfo.zip"))
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
// reported, rather than removed on faith; when a socket is found,
// everything around it is removed, leaving only the socket name and its
// parent directories. A removal that fails, or that leaves the path
// behind, is reported too, with rm's own messages, so that nothing is
// left on the device without saying why. With GOQNX_EXEC_RMSOCK=1, on a
// device where removing socket names is known to be safe, it removes the
// path whatever it holds.
func (c *config) removeSafely(p string) {
	if c.rmsock {
		// GOQNX_EXEC_RMSOCK=1: removing socket names is known to be
		// safe on this device, so remove the path whatever it holds.
		// The socket names go first, listed in full before any is
		// removed: on BlackBerry 10, once a socket name is removed, the
		// next read of an open listing of its directory can fail with
		// EBADF, which stops rm -rf (and find) part way.
		c.ssh2(fmt.Sprintf("for s in $(find %s -type s); do rm -f \"$s\"; done; true", sh(p)))
		c.remove(p)
		return
	}
	out, err := c.output("find " + sh(p) + " -type s")
	if err != nil {
		fmt.Fprintf(os.Stderr, "go_qnx_exec: not removing %s: could not check it for socket names: %v\n", p, err)
		return
	}
	if len(bytes.TrimSpace(out)) != 0 {
		// Keep each socket name and the directories leading to it;
		// remove everything else, so that what stays behind is the
		// socket's path and not the whole tree: first every file that
		// isn't a socket, then, bottom-up, every directory with no
		// socket under it. (Not rmdir: BlackBerry 10 has none.)
		c.ssh2(fmt.Sprintf("find %[1]s ! -type s ! -type d -exec rm -f {} \\; ; "+
			"find %[1]s -depth -type d | while read d; do "+
			"[ -d \"$d\" ] && [ -z \"$(find \"$d\" -type s)\" ] && rm -rf \"$d\"; done; true", sh(p)))
		fmt.Fprintf(os.Stderr, "go_qnx_exec: leaving these socket names in %s, which are unsafe to remove on QNX, with only their parent directories:\n%s", p, out)
		return
	}
	c.remove(p)
}

// remove removes a device path with rm -rf and reports it if the path
// survives or the command cannot be run.
func (c *config) remove(p string) {
	// rm's messages go to its standard output here, so that they are
	// reported only if the path survives. One retry: rm can fail on a
	// directory it emptied while reading it.
	const rm = "rm -rf %[1]s 2>&1; test -e %[1]s && echo REMAINS; true"
	var out []byte
	var err error
	for try := 0; try < 2; try++ {
		out, err = c.output(fmt.Sprintf(rm, sh(p)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "go_qnx_exec: could not remove %s: %v\n", p, err)
			return
		}
		if !bytes.Contains(out, []byte("REMAINS")) {
			return
		}
	}
	fmt.Fprintf(os.Stderr, "go_qnx_exec: %s is still on the device after rm -rf:\n%s", p, out)
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
