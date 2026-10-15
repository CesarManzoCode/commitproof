package history

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

// ExtractArchive extracts a run archive (CONTRACT §13: a zstd tar of a
// sealed run directory) into dir. Only regular files and
// directories with relative, non-escaping names are accepted; modes are
// kept (sealed runs are 0444/0555).
func ExtractArchive(archive, dir string) error {
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
