// Package report renders CommitProof's static incident report from sealed
// runs (CONTRACT §15 K6, ADR-0036): one self-contained HTML file per run —
// operations per client lane coloured by outcome, fault phases, observed
// rules and leader changes, the metrics the observations hold, checker
// anomalies linked to the operations involved, and per-fault failover
// metrics — and a per-campaign summary page with the §12.3 statistics.
//
// Every input is a sealed run whose seal verifies; anything else is an
// error and nothing is written (fail closed). Output is a pure function of
// the sealed bytes and the checks file: no time, host or path of the
// rendering, every map iterated in sorted order, numbers from integers.
// The package must not import runner, nemesis, cluster, workload,
// observation or checker logic (CONTRACT §5.3; checks.json is read through
// package checker/output).
package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/checker/output"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

// Format names the report format; it is printed on every page instead of
// the renderer's build, so that an unchanged run renders to the same bytes
// until the format itself changes (ADR-0036).
const Format = "cpreport-format 1"

// StubSuffix marks a run stub archive (manifest, plan, seal; CONTRACT §13).
const StubSuffix = ".stub" + history.ArchiveSuffix

// ChecksSuffix is the suffix of cpcheck's output next to a run.
const ChecksSuffix = ".checks.json"

// Run is a sealed run, verified and bound to its checks file.
type Run struct {
	ID           string
	SealRoot     string
	SealedFiles  int
	Manifest     map[string]any
	History      []history.Event
	Observations []history.Event
	Checks       *output.Checks
	ChecksSHA256 string
}

// runPaths returns the run id, whether path is an archive, and the checks
// file of a run given as a directory or as <id>.tar.zst.
func runPaths(path string) (id string, archive bool, checks string, err error) {
	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	switch {
	case strings.HasSuffix(base, StubSuffix):
		return "", false, "", fmt.Errorf("%s is a run stub: it holds no history to render (CONTRACT §13)", base)
	case strings.HasSuffix(base, history.ArchiveSuffix):
		id = strings.TrimSuffix(base, history.ArchiveSuffix)
		return id, true, filepath.Join(filepath.Dir(clean), id+ChecksSuffix), nil
	}
	return base, false, clean + ChecksSuffix, nil
}

// IsRun reports whether path names a run (a directory holding seal.json, or
// a run archive) rather than a campaign directory.
func IsRun(path string) bool {
	if strings.HasSuffix(path, history.ArchiveSuffix) {
		return true
	}
	_, err := os.Stat(filepath.Join(path, history.FileSeal))
	return err == nil
}

// LoadRun opens a run directory or run archive, verifies its seal
// (history.VerifySeal: every file hash, the Merkle root, both hash chains,
// well-formedness), reads its manifest and binds its checks file, whose
// run_id and seal_root must be the run's. Any failure is an error.
func LoadRun(path string) (*Run, error) {
	id, archive, checksPath, err := runPaths(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Clean(path)
	if archive {
		tmp, err := os.MkdirTemp("", "cpreport-run-")
		if err != nil {
			return nil, err
		}
		defer removeTree(tmp)
		if err := history.ExtractArchive(path, tmp); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		ents, err := os.ReadDir(tmp)
		if err != nil {
			return nil, err
		}
		if len(ents) != 1 || ents[0].Name() != id || !ents[0].IsDir() {
			return nil, fmt.Errorf("%s does not hold exactly the directory %s/", filepath.Base(path), id)
		}
		dir = filepath.Join(tmp, id)
	} else if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is neither a run directory nor a run archive", filepath.Base(path))
	}
	return loadDir(dir, id, checksPath)
}

func loadDir(dir, id, checksPath string) (*Run, error) {
	rep := history.VerifySeal(dir)
	if !rep.Valid {
		n := min(len(rep.Problems), 5)
		return nil, fmt.Errorf("run %s: seal does not verify: %s", id, strings.Join(rep.Problems[:n], "; "))
	}
	man, err := readJSONObject(filepath.Join(dir, history.FileManifest))
	if err != nil {
		return nil, fmt.Errorf("run %s: manifest: %w", id, err)
	}
	if got, _ := man["run_id"].(string); got != id {
		return nil, fmt.Errorf("run %s: manifest run_id %q does not name its directory", id, got)
	}
	cb, err := os.ReadFile(checksPath)
	if err != nil {
		return nil, fmt.Errorf("run %s: checks file: %w", id, err)
	}
	checks, err := output.Decode(cb)
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", id, err)
	}
	if checks.RunID != id || checks.SealRoot != rep.MerkleRoot {
		return nil, fmt.Errorf("run %s: checks file names run %q with seal root %s, the run's seal root is %s", id, checks.RunID, checks.SealRoot, rep.MerkleRoot)
	}
	sum := sha256.Sum256(cb)
	return &Run{ID: id, SealRoot: rep.MerkleRoot, SealedFiles: rep.Files, Manifest: man, History: rep.History,
		Observations: rep.Observations, Checks: checks, ChecksSHA256: hex.EncodeToString(sum[:])}, nil
}

func readJSONObject(p string) (map[string]any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// removeTree removes a directory that holds sealed (read-only) files.
func removeTree(dir string) {
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			os.Chmod(p, 0o755)
		}
		return nil
	})
	os.RemoveAll(dir)
}

// WriteFile creates dir and writes name into it; called only once a page
// has rendered completely, so a failure before leaves nothing behind.
func WriteFile(dir, name string, b []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return "", err
	}
	return p, nil
}
