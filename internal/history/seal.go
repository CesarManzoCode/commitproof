package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
)

// Run-directory file names (CONTRACT §5.4).
const (
	FileManifest     = "manifest.json"
	FilePlan         = "plan.json"
	FileHistory      = "history.jsonl"
	FileObservations = "observations.jsonl"
	FileCommands     = "commands.jsonl"
	FileSeal         = "seal.json"
	DirLogs          = "logs"
)

// RequiredFiles must exist in every sealed run directory.
var RequiredFiles = []string{FileManifest, FilePlan, FileHistory, FileObservations, FileCommands}

// MerkleAlgorithm documents the seal construction; it is written into
// seal.json so a reviewer can recompute the root without this code.
const MerkleAlgorithm = "files sorted by slash-separated relative path; " +
	"leaf = sha256(0x00 || path || 0x00 || lowercase hex sha256 of the file); " +
	"node = sha256(0x01 || left || right) over raw 32-byte digests; " +
	"an odd last node is carried up unchanged; root of zero files = sha256 of empty input"

// SealFile is one entry of seal.json.
type SealFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Seal is seal.json.
type Seal struct {
	Algorithm  string     `json:"algorithm"`
	Files      []SealFile `json:"files"`
	MerkleRoot string     `json:"merkle_root"`
	Version    int64      `json:"seal_version"`
}

// MerkleRoot computes the root over files (which must be sorted by Path).
func MerkleRoot(files []SealFile) (string, error) {
	if len(files) == 0 {
		sum := sha256.Sum256(nil)
		return hex.EncodeToString(sum[:]), nil
	}
	level := make([][]byte, 0, len(files))
	for i, f := range files {
		if i > 0 && files[i-1].Path >= f.Path {
			return "", fmt.Errorf("seal: files not sorted/unique at %q", f.Path)
		}
		h := sha256.New()
		h.Write([]byte{0})
		h.Write([]byte(f.Path))
		h.Write([]byte{0})
		h.Write([]byte(f.SHA256))
		level = append(level, h.Sum(nil))
	}
	for len(level) > 1 {
		var next [][]byte
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i])
				continue
			}
			h := sha256.New()
			h.Write([]byte{1})
			h.Write(level[i])
			h.Write(level[i+1])
			next = append(next, h.Sum(nil))
		}
		level = next
	}
	return hex.EncodeToString(level[0]), nil
}

// hashTree hashes every regular file under dir except seal.json at the top.
// Symlinks and other special files are an error.
func hashTree(dir string) ([]SealFile, error) {
	var files []SealFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("seal: %s is not a regular file", rel)
		}
		if rel == FileSeal {
			return nil
		}
		sum, size, err := HashFile(path)
		if err != nil {
			return err
		}
		files = append(files, SealFile{Path: rel, SHA256: sum, Size: size})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// HashFile returns the lowercase hex sha256 and size of a file.
func HashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func (s *Seal) canonical() ([]byte, error) {
	files := make([]any, len(s.Files))
	for i, f := range s.Files {
		files[i] = map[string]any{"path": f.Path, "sha256": f.SHA256, "size": f.Size}
	}
	return canonjson.Marshal(map[string]any{
		"algorithm": s.Algorithm, "files": files, "merkle_root": s.MerkleRoot, "seal_version": s.Version,
	})
}

// SealDir writes seal.json for dir, then makes every file 0444 and every
// directory 0555 (CONTRACT §5.4). It refuses a directory that is already
// sealed or lacks a required file.
func SealDir(dir string) (*Seal, error) {
	if _, err := os.Stat(filepath.Join(dir, FileSeal)); err == nil {
		return nil, fmt.Errorf("seal: %s is already sealed", dir)
	}
	for _, f := range RequiredFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return nil, fmt.Errorf("seal: required file: %w", err)
		}
	}
	files, err := hashTree(dir)
	if err != nil {
		return nil, err
	}
	root, err := MerkleRoot(files)
	if err != nil {
		return nil, err
	}
	s := &Seal{Algorithm: MerkleAlgorithm, Files: files, MerkleRoot: root, Version: 1}
	b, err := s.canonical()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, FileSeal), append(b, '\n'), 0o444); err != nil {
		return nil, fmt.Errorf("seal: write: %w", err)
	}
	// Files first, then directories bottom-up (a 0555 directory still lets the
	// owner chmod its entries, but not create or delete them).
	var dirs []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		return os.Chmod(path, 0o444)
	})
	if err != nil {
		return nil, fmt.Errorf("seal: chmod: %w", err)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i], 0o555); err != nil {
			return nil, fmt.Errorf("seal: chmod %s: %w", dirs[i], err)
		}
	}
	return s, nil
}

// SealReport is the result of VerifySeal.
type SealReport struct {
	Valid            bool     `json:"valid"`
	MerkleRoot       string   `json:"seal_root"`
	Files            int      `json:"files"`
	HistoryLines     int      `json:"history_lines"`
	ObservationLines int      `json:"observation_lines"`
	Problems         []string `json:"problems"`
	History          []Event  `json:"-"`
	Observations     []Event  `json:"-"`
}

// VerifySeal recomputes every file hash and the Merkle root, checks that the
// set of files equals the sealed set, verifies both hash chains, and checks
// that history.jsonl is a well-formed history. Any problem makes the run
// unusable for every consumer.
func VerifySeal(dir string) *SealReport {
	r := &SealReport{}
	bad := func(f string, a ...any) { r.Problems = append(r.Problems, fmt.Sprintf(f, a...)) }
	raw, err := os.ReadFile(filepath.Join(dir, FileSeal))
	if err != nil {
		bad("read seal: %v", err)
		return r
	}
	raw = []byte(strings.TrimSuffix(string(raw), "\n"))
	if !canonjson.IsCanonical(raw) {
		bad("seal.json is not canonical JSON")
	}
	var s Seal
	if err := json.Unmarshal(raw, &s); err != nil {
		bad("parse seal: %v", err)
		return r
	}
	r.MerkleRoot = s.MerkleRoot
	if s.Version != 1 || s.Algorithm != MerkleAlgorithm {
		bad("unsupported seal version/algorithm")
	}
	root, err := MerkleRoot(s.Files)
	if err != nil {
		bad("%v", err)
	} else if root != s.MerkleRoot {
		bad("merkle root in seal.json (%s) does not match its file list (%s)", s.MerkleRoot, root)
	}
	actual, err := hashTree(dir)
	if err != nil {
		bad("hash tree: %v", err)
		return r
	}
	r.Files = len(actual)
	sealed := map[string]SealFile{}
	for _, f := range s.Files {
		sealed[f.Path] = f
	}
	for _, f := range actual {
		sf, ok := sealed[f.Path]
		switch {
		case !ok:
			bad("file %s is not in the seal", f.Path)
		case sf.SHA256 != f.SHA256 || sf.Size != f.Size:
			bad("file %s: sha256 %s size %d, sealed %s size %d", f.Path, f.SHA256, f.Size, sf.SHA256, sf.Size)
		}
		delete(sealed, f.Path)
	}
	for p := range sealed {
		bad("sealed file %s is missing", p)
	}
	for _, f := range RequiredFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			bad("required file %s missing", f)
		}
	}
	evs, err := ReadChain(filepath.Join(dir, FileHistory))
	r.History, r.HistoryLines = evs, len(evs)
	if err != nil {
		bad("history.jsonl: %v", err)
	}
	for _, ev := range evs {
		if ev.Type == TypeObs {
			bad("history.jsonl: i=%d has type obs", ev.I)
			break
		}
	}
	for _, p := range WellFormed(evs) {
		bad("history.jsonl: %s", p)
	}
	obs, err := ReadChain(filepath.Join(dir, FileObservations))
	r.Observations, r.ObservationLines = obs, len(obs)
	if err != nil {
		bad("observations.jsonl: %v", err)
	}
	for _, ev := range obs {
		if ev.Type != TypeObs {
			bad("observations.jsonl: i=%d has type %s", ev.I, ev.Type)
			break
		}
	}
	r.Valid = len(r.Problems) == 0
	if r.Problems == nil {
		r.Problems = []string{}
	}
	return r
}
