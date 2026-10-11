// Package slowfs is the slow-disk layer of CONTRACT §8 (F4a, ADR-0006): a
// FUSE pass-through file system (github.com/hanwen/go-fuse/v2 v2.12.0)
// mounted over a node's /data before pgctld starts, whose file `write` and
// `fsync` requests sleep for the delay held in a control file that lives
// outside the mount (P17).
//
// The package also holds the node-side tools the nemesis runs through
// `docker exec` inside pg-nX: the write+fsync latency probe (F4a), the
// PostgreSQL process listing (F2a, F3a) and the control-file writer. It runs
// only inside node containers; no verdict binary links it (CONTRACT §5.3).
package slowfs

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// ControlKey is the only key of the control file: "delay_ms=<n>".
const ControlKey = "delay_ms"

// ParseControl parses the control file content ("delay_ms=<n>", optional
// surrounding whitespace). A malformed file is an error; the daemon then
// keeps its previous delay and reports the error in its status file.
func ParseControl(b []byte) (time.Duration, error) {
	s := strings.TrimSpace(string(b))
	k, v, ok := strings.Cut(s, "=")
	if !ok || strings.TrimSpace(k) != ControlKey {
		return 0, fmt.Errorf("control file %q: want %s=<milliseconds>", s, ControlKey)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n < 0 || n > 600_000 {
		return 0, fmt.Errorf("control file %q: delay must be an integer in [0, 600000] ms", s)
	}
	return time.Duration(n) * time.Millisecond, nil
}

// FormatControl renders a control file.
func FormatControl(d time.Duration) []byte {
	return []byte(fmt.Sprintf("%s=%d\n", ControlKey, d.Milliseconds()))
}

// Delay is the delay applied to every file write and fsync request.
type Delay struct{ ns atomic.Int64 }

// Get returns the current delay.
func (d *Delay) Get() time.Duration { return time.Duration(d.ns.Load()) }

// Set changes the delay.
func (d *Delay) Set(v time.Duration) { d.ns.Store(int64(v)) }

func (d *Delay) sleep(ctx context.Context) {
	v := d.Get()
	if v <= 0 {
		return
	}
	t := time.NewTimer(v)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done(): // the request was interrupted by the caller
	}
}

// node is a loopback node whose files are opened through slowFile.
type node struct {
	*fs.LoopbackNode
	delay *Delay
}

var (
	_ fs.NodeOpener      = (*node)(nil)
	_ fs.NodeCreater     = (*node)(nil)
	_ fs.NodeWrapChilder = (*node)(nil)
)

// WrapChild makes every node of the tree a slowfs node.
func (n *node) WrapChild(ctx context.Context, ops fs.InodeEmbedder) fs.InodeEmbedder {
	if lb, ok := ops.(*fs.LoopbackNode); ok {
		return &node{LoopbackNode: lb, delay: n.delay}
	}
	return ops
}

func (n *node) wrap(fh fs.FileHandle) fs.FileHandle {
	if lf, ok := fh.(*fs.LoopbackFile); ok {
		return &slowFile{f: lf, delay: n.delay}
	}
	return fh
}

// Open wraps the loopback file handle.
func (n *node) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	fh, fl, errno := n.LoopbackNode.Open(ctx, flags)
	if errno != 0 {
		return nil, 0, errno
	}
	return n.wrap(fh), fl, 0
}

// Create wraps the loopback file handle.
func (n *node) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	in, fh, fl, errno := n.LoopbackNode.Create(ctx, name, flags, mode, out)
	if errno != 0 {
		return nil, nil, 0, errno
	}
	return in, n.wrap(fh), fl, 0
}

// slowFile delegates to a loopback file and delays Write and Fsync. It holds
// the loopback file in a named field instead of embedding it, so that it does
// not expose PassthroughFd (kernel FUSE passthrough would send reads and
// writes to the backing file without asking the daemon, bypassing the delay)
// nor Writev (the bridge then always calls Write).
type slowFile struct {
	f     *fs.LoopbackFile
	delay *Delay
}

var (
	_ fs.FileReader    = (*slowFile)(nil)
	_ fs.FileWriter    = (*slowFile)(nil)
	_ fs.FileFsyncer   = (*slowFile)(nil)
	_ fs.FileFlusher   = (*slowFile)(nil)
	_ fs.FileReleaser  = (*slowFile)(nil)
	_ fs.FileGetattrer = (*slowFile)(nil)
	_ fs.FileSetattrer = (*slowFile)(nil)
	_ fs.FileLseeker   = (*slowFile)(nil)
	_ fs.FileAllocater = (*slowFile)(nil)
	_ fs.FileGetlker   = (*slowFile)(nil)
	_ fs.FileSetlker   = (*slowFile)(nil)
	_ fs.FileSetlkwer  = (*slowFile)(nil)
)

func (s *slowFile) Read(ctx context.Context, buf []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	return s.f.Read(ctx, buf, off)
}

// Write sleeps for the current delay, then writes (P17: per write request).
func (s *slowFile) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	s.delay.sleep(ctx)
	return s.f.Write(ctx, data, off)
}

// Fsync sleeps for the current delay, then syncs (fsync and fdatasync).
func (s *slowFile) Fsync(ctx context.Context, flags uint32) syscall.Errno {
	s.delay.sleep(ctx)
	return s.f.Fsync(ctx, flags)
}

func (s *slowFile) Flush(ctx context.Context) syscall.Errno   { return s.f.Flush(ctx) }
func (s *slowFile) Release(ctx context.Context) syscall.Errno { return s.f.Release(ctx) }
func (s *slowFile) Getattr(ctx context.Context, out *fuse.AttrOut) syscall.Errno {
	return s.f.Getattr(ctx, out)
}
func (s *slowFile) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	return s.f.Setattr(ctx, in, out)
}
func (s *slowFile) Lseek(ctx context.Context, off uint64, whence uint32) (uint64, syscall.Errno) {
	return s.f.Lseek(ctx, off, whence)
}
func (s *slowFile) Allocate(ctx context.Context, off uint64, size uint64, mode uint32) syscall.Errno {
	return s.f.Allocate(ctx, off, size, mode)
}
func (s *slowFile) Getlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) syscall.Errno {
	return s.f.Getlk(ctx, owner, lk, flags, out)
}
func (s *slowFile) Setlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	return s.f.Setlk(ctx, owner, lk, flags)
}
func (s *slowFile) Setlkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	return s.f.Setlkw(ctx, owner, lk, flags)
}

// Mount mounts the pass-through of backing at mountpoint and returns the
// server. Options: allow_other (pgctld and PostgreSQL run as the node user,
// the daemon as root), default_permissions (the kernel checks permissions
// from the backing attributes, as on the plain volume), no writeback cache,
// a direct mount(2) without a fusermount fallback (the launcher reaps every
// child of PID 1, so it must not fork helpers), and no entry, attribute or
// negative caching: multipooler writes files of the same volume through
// the plain mount in its own container (e.g. postgresql.auto.conf before a
// restart), and the FUSE view must see them at once. With a one-second
// attribute cache PostgreSQL read a stale size of postgresql.auto.conf and
// missed archive_mode (K3 development, NOTES).
func Mount(backing, mountpoint string, delay *Delay) (*fuse.Server, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(backing, &st); err != nil {
		return nil, fmt.Errorf("slowfs: backing %s: %w", backing, err)
	}
	root := &fs.LoopbackRoot{Path: backing, Dev: uint64(st.Dev)}
	rn := &node{LoopbackNode: &fs.LoopbackNode{RootData: root}, delay: delay}
	root.RootNode = rn
	var zero time.Duration
	opts := &fs.Options{
		EntryTimeout:    &zero,
		AttrTimeout:     &zero,
		NegativeTimeout: &zero,
		MountOptions: fuse.MountOptions{
			AllowOther:        true,
			Options:           []string{"default_permissions"},
			FsName:            "slowfs:" + backing,
			Name:              "slowfs",
			DirectMount:       true,
			DirectMountStrict: true,
		},
	}
	srv, err := fs.Mount(mountpoint, rn, opts)
	if err != nil {
		return nil, fmt.Errorf("slowfs: mount %s on %s: %w", backing, mountpoint, err)
	}
	return srv, nil
}

// WatchControl re-reads the control file every interval until ctx ends and
// applies its delay; it writes "<control>.status" after every change so the
// applied value is observable. A missing or malformed file keeps the delay.
func WatchControl(ctx context.Context, control string, delay *Delay, interval time.Duration, logf func(string, ...any)) {
	var last []byte
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		b, err := os.ReadFile(control)
		if err == nil && string(b) != string(last) {
			last = b
			d, perr := ParseControl(b)
			status := ""
			if perr != nil {
				status = fmt.Sprintf("error=%q applied_ms=%d\n", perr.Error(), delay.Get().Milliseconds())
			} else {
				delay.Set(d)
				status = fmt.Sprintf("applied_ms=%d\n", d.Milliseconds())
			}
			logf("slowfs: control %s: %s", control, strings.TrimSpace(status))
			writeAtomic(control+".status", []byte(status))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SetControl writes the control file atomically (the nemesis's F4a
// injection and heal).
func SetControl(control string, d time.Duration) error {
	return writeAtomic(control, FormatControl(d))
}
