package slowfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// ControlPollInterval is how often the daemon re-reads the control file.
// The F4a confirmation predicate measures the latency itself (P17), so this
// only bounds how fast an injection takes effect.
const ControlPollInterval = 100 * time.Millisecond

// LaunchConfig configures the node launcher.
type LaunchConfig struct {
	Backing string   // the node's data volume
	Mount   string   // where the pass-through is mounted (/data)
	Control string   // control file, outside the mount
	Bypass  []string // subdirectories bind-mounted from the backing volume over the mount
	UID     int      // user the command runs as
	GID     int
	Argv    []string // the command (pgctld and its flags)
}

func logf(f string, a ...any) {
	fmt.Fprintf(os.Stderr, time.Now().UTC().Format(time.RFC3339Nano)+" "+f+"\n", a...)
}

// Launch is the entrypoint of pg-nX (PID 1). It resets the control file to
// delay 0, mounts the pass-through of Backing at Mount, bind-mounts each
// Bypass directory of the backing volume over its pass-through view, then
// runs Argv as UID:GID. It forwards termination signals to that command,
// reaps every child it inherits (pg_ctl daemonizes the postmaster, which is
// then re-parented to PID 1), and exits with the command's status after
// unmounting.
//
// Bypass exists for pg_sockets: PostgreSQL binds its Unix socket there and
// multipooler, in its own container and mount namespace, connects to it
// through the plain volume. A socket bound inside the FUSE view is attached
// to the FUSE inode and unreachable from the plain volume, so the socket
// directory is the backing directory itself in both containers.
func Launch(cfg LaunchConfig) int {
	if os.Geteuid() != 0 {
		logf("slowfs: launcher must run as root to mount (euid %d)", os.Geteuid())
		return 3
	}
	if len(cfg.Argv) == 0 {
		logf("slowfs: no command")
		return 3
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Control), 0o755); err != nil {
		logf("slowfs: %v", err)
		return 2
	}
	if err := SetControl(cfg.Control, 0); err != nil {
		logf("slowfs: reset control: %v", err)
		return 2
	}
	delay := &Delay{}
	srv, err := Mount(cfg.Backing, cfg.Mount, delay)
	if err != nil {
		logf("%v", err)
		return 2
	}
	logf("slowfs: mounted %s on %s (control %s)", cfg.Backing, cfg.Mount, cfg.Control)
	// go-fuse v2.12.0's direct mount returns the /dev/fuse descriptor
	// without close-on-exec (only its fusermount path sets it), so pgctld,
	// PostgreSQL and every backend would inherit it. A process holding the
	// device keeps the connection alive after the daemon dies, and backends
	// exiting with files open on the mount then wait forever for replies
	// nobody serves (observed in K3 development: postgres processes stuck
	// in D state after teardown). Mark it close-on-exec before any child
	// starts.
	n, err := cloexecFuseFDs()
	if err != nil || n == 0 {
		logf("slowfs: close-on-exec on /dev/fuse: %d descriptors, %v", n, err)
		srv.Unmount()
		return 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	go WatchControl(ctx, cfg.Control, delay, ControlPollInterval, logf)
	var binds []string
	cleanup := func() {
		cancel()
		for i := len(binds) - 1; i >= 0; i-- {
			if err := unix.Unmount(binds[i], unix.MNT_DETACH); err != nil {
				logf("slowfs: unmount %s: %v", binds[i], err)
			}
		}
		if err := srv.Unmount(); err != nil {
			logf("slowfs: unmount %s: %v (detaching)", cfg.Mount, err)
			unix.Unmount(cfg.Mount, unix.MNT_DETACH)
		}
	}
	for _, d := range cfg.Bypass {
		src, dst := filepath.Join(cfg.Backing, d), filepath.Join(cfg.Mount, d)
		if err := ensureDir(src, cfg.UID, cfg.GID); err != nil {
			logf("slowfs: bypass %s: %v", d, err)
			cleanup()
			return 2
		}
		if err := unix.Mount(src, dst, "", unix.MS_BIND, ""); err != nil {
			logf("slowfs: bind %s on %s: %v", src, dst, err)
			cleanup()
			return 2
		}
		binds = append(binds, dst)
		logf("slowfs: bypass %s (bind of %s)", dst, src)
	}
	code := runChild(cfg)
	// The command is gone; whatever it left (PostgreSQL after a failed
	// stop, SIGSTOPped backends) is killed and reaped while this process
	// still serves the mount, so nothing exits against a dead daemon.
	killRemaining(10 * time.Second)
	cleanup()
	return code
}

// cloexecFuseFDs sets FD_CLOEXEC on every descriptor of this process that
// refers to /dev/fuse and returns how many it found.
func cloexecFuseFDs() (int, error) {
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range ents {
		fd, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if t, err := os.Readlink("/proc/self/fd/" + e.Name()); err == nil && t == "/dev/fuse" {
			if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// killRemaining sends SIGKILL to every other process of the PID namespace
// (kill(-1) from PID 1) and reaps them, for at most d.
func killRemaining(d time.Duration) {
	if os.Getpid() != 1 {
		return
	}
	if err := unix.Kill(-1, unix.SIGKILL); err != nil && !errors.Is(err, unix.ESRCH) {
		logf("slowfs: kill remaining: %v", err)
	}
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, nil)
		switch {
		case errors.Is(err, unix.ECHILD):
			return
		case err != nil && !errors.Is(err, unix.EINTR):
			logf("slowfs: wait4: %v", err)
			return
		case pid > 0:
			logf("slowfs: reaped pid %d at shutdown (%s)", pid, describe(ws))
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	logf("slowfs: processes remain after %s", d)
}

func ensureDir(p string, uid, gid int) error {
	if st, err := os.Stat(p); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("%s is not a directory", p)
		}
		return nil
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		return err
	}
	return os.Chown(p, uid, gid)
}

// childEnv is the environment of the command: the container's, with the
// identity variables of the user it runs as (as if the container ran as
// that user, which is what K1's pg-nX did).
func childEnv(uid int) []string {
	env := os.Environ()
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return env
	}
	set := map[string]string{"HOME": u.HomeDir, "USER": u.Username, "LOGNAME": u.Username}
	out := make([]string, 0, len(env)+3)
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if _, replaced := set[k]; !replaced {
			out = append(out, kv)
		}
	}
	for _, k := range []string{"HOME", "USER", "LOGNAME"} {
		out = append(out, k+"="+set[k])
	}
	return out
}

// runChild starts the command and reaps children until the command exits.
// It never uses os/exec's Wait: the reaper owns every child (wait4(-1)).
func runChild(cfg LaunchConfig) int {
	path, err := exec.LookPath(cfg.Argv[0])
	if err != nil {
		logf("slowfs: %v", err)
		return 127
	}
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGUSR2)
	defer signal.Stop(sigs)
	proc, err := os.StartProcess(path, cfg.Argv, &os.ProcAttr{
		Env:   childEnv(cfg.UID),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys: &syscall.SysProcAttr{Credential: &syscall.Credential{
			Uid: uint32(cfg.UID), Gid: uint32(cfg.GID), Groups: []uint32{uint32(cfg.GID)},
		}},
	})
	if err != nil {
		logf("slowfs: start %s: %v", path, err)
		return 126
	}
	logf("slowfs: started %s as %d:%d (pid %d)", path, cfg.UID, cfg.GID, proc.Pid)
	go func() {
		for s := range sigs {
			logf("slowfs: forwarding %v to pid %d", s, proc.Pid)
			proc.Signal(s)
		}
	}()
	for {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(-1, &ws, 0, nil)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			logf("slowfs: wait4: %v", err)
			return 2
		}
		if pid != proc.Pid {
			logf("slowfs: reaped orphan pid %d (%s)", pid, describe(ws))
			continue
		}
		logf("slowfs: command pid %d ended (%s)", pid, describe(ws))
		switch {
		case ws.Exited():
			return ws.ExitStatus()
		case ws.Signaled():
			return 128 + int(ws.Signal())
		}
		return 2
	}
}

func describe(ws unix.WaitStatus) string {
	switch {
	case ws.Exited():
		return fmt.Sprintf("exit %d", ws.ExitStatus())
	case ws.Signaled():
		return fmt.Sprintf("signal %v", ws.Signal())
	}
	return fmt.Sprintf("status %#x", uint32(ws))
}
