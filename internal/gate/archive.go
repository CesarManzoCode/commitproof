package gate

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// ArchiveSuffix marks a run directory committed as a zstd-compressed tar
// (CONTRACT §13: evidence runs are compressed with zstd and still sealed by
// the hashes of the uncompressed files).
const ArchiveSuffix = ".tar.zst"

// RunDir resolves a recorded run directory under the evidence: the
// directory itself or, if only <dir>.tar.zst exists, the archive extracted
// into a temporary directory (removed by Close). Extraction is cached.
func (e *Evidence) RunDir(rel string) (string, error) {
	p := e.Path(rel)
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p, nil
	}
	arch := e.Path(rel + ArchiveSuffix)
	if _, err := os.Stat(arch); err != nil {
		return "", fmt.Errorf("run %s: neither a directory nor %s", rel, ArchiveSuffix)
	}
	if d, ok := e.extracted[arch]; ok {
		return d, nil
	}
	tmp, err := os.MkdirTemp("", "cpgate-run-")
	if err != nil {
		return "", err
	}
	if e.extracted == nil {
		e.extracted = map[string]string{}
	}
	e.temps = append(e.temps, tmp)
	if err := ExtractRun(arch, tmp); err != nil {
		return "", err
	}
	d := filepath.Join(tmp, strings.TrimSuffix(filepath.Base(arch), ArchiveSuffix))
	if st, err := os.Stat(d); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s does not contain %s/", arch, filepath.Base(d))
	}
	e.extracted[arch] = d
	return d, nil
}

// Close removes extracted runs.
func (e *Evidence) Close() {
	for _, t := range e.temps {
		removeAll(t)
	}
	e.temps, e.extracted = nil, nil
}

// ExtractRun extracts a run archive into dir. Only regular files and
// directories with relative, non-escaping names are accepted; modes are
// kept (sealed runs are 0444/0555).
func ExtractRun(archive, dir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		return fmt.Errorf("%s: %w", archive, err)
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	var dirs []struct {
		p    string
		mode fs.FileMode
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %w", archive, err)
		}
		name := filepath.Clean(h.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("%s: unsafe entry %q", archive, h.Name)
		}
		t := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(t, 0o755); err != nil {
				return err
			}
			dirs = append(dirs, struct {
				p    string
				mode fs.FileMode
			}{t, fs.FileMode(h.Mode).Perm()})
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(t), 0o755); err != nil {
				return err
			}
			o, err := os.OpenFile(t, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(o, tr); err != nil {
				o.Close()
				return err
			}
			if err := o.Close(); err != nil {
				return err
			}
			if err := os.Chmod(t, fs.FileMode(h.Mode).Perm()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: entry %q is not a file or directory", archive, h.Name)
		}
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Chmod(dirs[i].p, dirs[i].mode)
	}
	return nil
}

// ArchiveRun writes dir (a run directory) as a zstd tar named
// <dir>.tar.zst next to it: entries sorted, modes kept, no owners or times.
// Used by the K3.4 negative control to re-archive a forged run.
func ArchiveRun(dir, archive string) error {
	base := filepath.Base(dir)
	var paths []string
	if err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, p)
		return nil
	}); err != nil {
		return err
	}
	out, err := os.OpenFile(archive, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	zw, err := zstd.NewWriter(out)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(zw)
	for _, p := range paths {
		st, err := os.Lstat(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		name := filepath.ToSlash(filepath.Join(base, rel))
		h := &tar.Header{Name: name, Mode: int64(st.Mode().Perm()), Format: tar.FormatPAX}
		switch {
		case st.IsDir():
			h.Typeflag, h.Name = tar.TypeDir, name+"/"
			if err := tw.WriteHeader(h); err != nil {
				return err
			}
		case st.Mode().IsRegular():
			h.Typeflag, h.Size = tar.TypeReg, st.Size()
			if err := tw.WriteHeader(h); err != nil {
				return err
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, f)
			f.Close()
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s is not a file or directory", p)
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return zw.Close()
}
