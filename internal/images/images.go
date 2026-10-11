// Package images builds the CommitProof images from the pinned Multigres
// commit, writes images.lock.json (CONTRACT §5.2) and verifies local images
// against it.
package images

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/gate"
	"github.com/CesarManzoCode/commitproof/internal/host"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Paths relative to the repository root.
const (
	LockFile  = "images.lock.json"
	SourceDir = ".cache/multigres"
)

// Label put on every temporary container so it falls under AGENTS.md §5.
const projectLabel = "com.docker.compose.project=commitproof"

// Spec is one image CommitProof builds.
type Spec struct {
	Tag        string
	Dockerfile string // repository-relative
	Upstream   string // upstream Dockerfile name at the pinned commit ("" = CommitProof-owned)
	Context    string // repository-relative build context
	Binaries   []string
	Pgbackrest bool
	BuildArgs  [][2]string
}

// Specs lists the images of the real build (tag suffix "real"; mutants use
// M<n> from K4).
func Specs(src host.Source) []Spec {
	mg := [][2]string{{"GIT_COMMIT", params.MultigresCommit}, {"COMMIT_DATE", params.MultigresCommitDate}}
	return []Spec{
		{Tag: "commitproof/multigres:real", Dockerfile: "build/Dockerfile", Upstream: "Dockerfile", Context: SourceDir,
			Binaries: []string{"/multigres/bin/multigres", "/multigres/bin/multiadmin", "/multigres/bin/multigateway",
				"/multigres/bin/multiorch", "/multigres/bin/multipooler", "/usr/bin/pgbackrest"},
			Pgbackrest: true, BuildArgs: mg},
		{Tag: "commitproof/pgctld:real", Dockerfile: "build/Dockerfile.pgctld", Upstream: "Dockerfile.pgctld", Context: SourceDir,
			Binaries: []string{"/usr/local/bin/pgctld", "/usr/bin/pgbackrest"}, Pgbackrest: true, BuildArgs: mg},
		{Tag: "commitproof/runner:real", Dockerfile: "build/Dockerfile.runner", Context: ".",
			Binaries:  []string{"/usr/local/bin/commitproof"},
			BuildArgs: [][2]string{{"CP_COMMIT", src.Commit}, {"CP_DIRTY", fmt.Sprint(src.Dirty)}}},
	}
}

// MultigresBinaries are the six binaries of K1.2.
var MultigresBinaries = []string{
	"commitproof/multigres:real:/multigres/bin/multigres",
	"commitproof/multigres:real:/multigres/bin/multiadmin",
	"commitproof/multigres:real:/multigres/bin/multigateway",
	"commitproof/multigres:real:/multigres/bin/multiorch",
	"commitproof/multigres:real:/multigres/bin/multipooler",
	"commitproof/pgctld:real:/usr/local/bin/pgctld",
}

// Builder runs builds through a command recorder.
type Builder struct {
	Root string
	Rec  *cmdlog.Recorder
	Echo io.Writer
}

func (b *Builder) cmd(ctx context.Context, name string, argv []string, o cmdlog.Opts, timeoutKind string) (cmdlog.Result, error) {
	t := host.QuickTimeout
	switch timeoutKind {
	case "build":
		t = host.BuildTimeout
	case "git":
		t = host.GitTimeout
	}
	if o.Dir == "" {
		o.Dir = b.Root
	}
	r := b.Rec.Run(ctx, t, name, argv, o)
	if r.Err != nil {
		return r, fmt.Errorf("%s: %w", strings.Join(argv, " "), r.Err)
	}
	if r.ExitCode != 0 {
		return r, fmt.Errorf("%s: exit %d: %s", strings.Join(argv, " "), r.ExitCode, tail(r.Stderr))
	}
	return r, nil
}

func tail(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 600 {
		s = "…" + s[len(s)-600:]
	}
	return s
}

// EnsureSource clones the pinned commit into the build cache if absent and
// verifies that the checkout is exactly the pinned tree (fail closed).
func (b *Builder) EnsureSource(ctx context.Context) error {
	dir := filepath.Join(b.Root, SourceDir)
	if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		steps := [][]string{
			{"git", "-C", dir, "init", "-q"},
			{"git", "-C", dir, "remote", "add", "origin", params.MultigresRepository},
			{"git", "-C", dir, "fetch", "-q", "--depth", "1", "origin", params.MultigresCommit},
			{"git", "-C", dir, "checkout", "-q", "FETCH_HEAD"},
		}
		for _, s := range steps {
			if _, err := b.cmd(ctx, "git", s, cmdlog.Opts{}, "git"); err != nil {
				return err
			}
		}
	}
	r, err := b.cmd(ctx, "git-head", []string{"git", "-C", dir, "rev-parse", "HEAD"}, cmdlog.Opts{}, "")
	if err != nil {
		return err
	}
	if got := strings.TrimSpace(string(r.Stdout)); got != params.MultigresCommit {
		return fmt.Errorf("%s is at %s, not the pinned %s", SourceDir, got, params.MultigresCommit)
	}
	r, err = b.cmd(ctx, "git-status", []string{"git", "-C", dir, "status", "--porcelain", "--ignored"}, cmdlog.Opts{}, "")
	if err != nil {
		return err
	}
	if s := strings.TrimSpace(string(r.Stdout)); s != "" {
		return fmt.Errorf("%s has local changes or untracked files; the build context must be the pinned tree:\n%s", SourceDir, s)
	}
	return nil
}

// upstreamFile returns a file of the pinned tree.
func (b *Builder) upstreamFile(ctx context.Context, name string) ([]byte, error) {
	r, err := b.cmd(ctx, "git-show-"+name, []string{"git", "-C", filepath.Join(b.Root, SourceDir), "show", params.MultigresCommit + ":" + name}, cmdlog.Opts{}, "")
	return r.Stdout, err
}

// Build builds every image, hashes their binaries and writes the lock.
// It returns the lock and the six Multigres binary hashes.
func (b *Builder) Build(ctx context.Context, noCache bool) (map[string]any, error) {
	src, err := host.SourceIdentity(b.Root)
	if err != nil {
		return nil, err
	}
	if err := b.EnsureSource(ctx); err != nil {
		return nil, err
	}
	specs := Specs(src)
	dockerfiles := map[string]any{}
	bases := map[string]string{}
	for _, s := range specs {
		content, err := os.ReadFile(filepath.Join(b.Root, s.Dockerfile))
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(content)
		entry := map[string]any{"sha256": hex.EncodeToString(sum[:])}
		if s.Upstream != "" {
			up, err := b.upstreamFile(ctx, s.Upstream)
			if err != nil {
				return nil, err
			}
			rep := gate.CheckDerivation(up, content)
			if !rep.OK {
				return nil, fmt.Errorf("%s is not a FROM-only derivation of upstream %s: %v", s.Dockerfile, s.Upstream, rep.Violations)
			}
			entry["upstream"] = s.Upstream + "@" + params.MultigresCommit
			entry["upstream_blob"] = gate.GitBlobID(up)
		}
		dockerfiles[s.Dockerfile] = entry
		for _, ref := range gate.FromRefs(content) {
			name, digest, ok := strings.Cut(ref, "@")
			if !ok {
				return nil, fmt.Errorf("%s: FROM %s is not digest-pinned", s.Dockerfile, ref)
			}
			if prev, dup := bases[name]; dup && prev != digest {
				return nil, fmt.Errorf("base %s pinned to two digests", name)
			}
			bases[name] = digest
		}
	}
	compose, err := os.ReadFile(filepath.Join(b.Root, "deploy", "compose.yaml"))
	if err != nil {
		return nil, err
	}
	for _, l := range strings.Split(string(compose), "\n") {
		l = strings.TrimSpace(l)
		if ref, ok := strings.CutPrefix(l, "image: "); ok && !strings.HasPrefix(ref, "commitproof/") {
			name, digest, ok := strings.Cut(ref, "@")
			if !ok {
				return nil, fmt.Errorf("deploy/compose.yaml: image %s is not digest-pinned", ref)
			}
			bases[name] = digest
		}
	}
	imgs := map[string]any{}
	for _, s := range specs {
		argv := []string{"docker", "buildx", "build", "--progress=plain", "--provenance=false", "--sbom=false",
			"--file", filepath.Join(b.Root, s.Dockerfile), "--tag", s.Tag}
		for _, a := range s.BuildArgs {
			argv = append(argv, "--build-arg", a[0]+"="+a[1])
		}
		if noCache {
			argv = append(argv, "--no-cache")
		}
		argv = append(argv, filepath.Join(b.Root, s.Context))
		if _, err := b.cmd(ctx, "build-"+path.Base(strings.Split(s.Tag, ":")[0]), argv, cmdlog.Opts{}, "build"); err != nil {
			return nil, err
		}
		id, err := b.ImageID(ctx, s.Tag)
		if err != nil {
			return nil, err
		}
		bins, err := b.hashBinaries(ctx, s)
		if err != nil {
			return nil, err
		}
		entry := map[string]any{"id": id, "dockerfile": s.Dockerfile, "binaries": bins}
		args := map[string]any{}
		for _, a := range s.BuildArgs {
			args[a[0]] = a[1]
		}
		entry["build_args"] = args
		if s.Pgbackrest {
			r, err := b.cmd(ctx, "pgbackrest-version", []string{"docker", "run", "--rm", "--label", projectLabel, "--network", "none",
				"--entrypoint", "pgbackrest", s.Tag, "version"}, cmdlog.Opts{}, "")
			if err != nil {
				return nil, err
			}
			entry["pgbackrest_version"] = strings.TrimSpace(string(r.Stdout))
		}
		imgs[s.Tag] = entry
	}
	baseMap := map[string]any{}
	for k, v := range bases {
		baseMap[k] = v
	}
	lock := map[string]any{
		"lock_version":     1,
		"contract_version": params.ContractVersion,
		"multigres": map[string]any{
			"repository": params.MultigresRepository, "commit": params.MultigresCommit, "commit_date": params.MultigresCommitDate,
		},
		"commitproof":  map[string]any{"commit": src.Commit, "dirty": src.Dirty},
		"dockerfiles":  dockerfiles,
		"base_images":  baseMap,
		"images":       imgs,
		"build":        map[string]any{"no_cache": noCache, "tool": "docker buildx build"},
		"mutant":       nil,
		"patch_sha256": nil,
	}
	out, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(b.Root, LockFile), append(out, '\n'), 0o644); err != nil {
		return nil, err
	}
	return lock, nil
}

// ImageID returns the local image ID of ref.
func (b *Builder) ImageID(ctx context.Context, ref string) (string, error) {
	r, err := b.cmd(ctx, "image-id", []string{"docker", "image", "inspect", "--format", "{{.Id}}", ref}, cmdlog.Opts{}, "")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(r.Stdout)), nil
}

// hashBinaries copies each binary out of a stopped container as a tar stream
// (no image code runs) and returns path → sha256.
func (b *Builder) hashBinaries(ctx context.Context, s Spec) (map[string]any, error) {
	r, err := b.cmd(ctx, "create-"+path.Base(strings.Split(s.Tag, ":")[0]), []string{"docker", "create", "--label", projectLabel, "--network", "none", s.Tag}, cmdlog.Opts{}, "")
	if err != nil {
		return nil, err
	}
	cid := strings.TrimSpace(string(r.Stdout))
	defer b.cmd(context.Background(), "rm-tmp", []string{"docker", "rm", "-f", cid}, cmdlog.Opts{}, "")
	out := map[string]any{}
	for _, bin := range s.Binaries {
		cctx, cancel := context.WithTimeout(ctx, host.QuickTimeout*6)
		cmd := exec.CommandContext(cctx, "docker", "cp", cid+":"+bin, "-")
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			cancel()
			return nil, err
		}
		sum, err := hashTarEntry(pipe, path.Base(bin))
		werr := cmd.Wait()
		cancel()
		if err := errors.Join(err, werr); err != nil {
			return nil, fmt.Errorf("hash %s:%s: %w", s.Tag, bin, err)
		}
		out[bin] = sum
		b.Rec.Note("hash-binary", map[string]any{"image": s.Tag, "path": bin, "sha256": sum, "argv": []any{"docker", "cp", cid + ":" + bin, "-"}})
	}
	return out, nil
}

func hashTarEntry(r io.Reader, name string) (string, error) {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("%s not found in tar stream", name)
		}
		if err != nil {
			return "", err
		}
		if path.Base(h.Name) == name && h.Typeflag == tar.TypeReg {
			sum := sha256.New()
			if _, err := io.Copy(sum, tr); err != nil {
				return "", err
			}
			io.Copy(io.Discard, r)
			return hex.EncodeToString(sum.Sum(nil)), nil
		}
	}
}

// ReadLock reads images.lock.json.
func ReadLock(root string) (map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(root, LockFile))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", LockFile, err)
	}
	return m, nil
}

// Mismatch is one image whose local ID differs from the lock.
type Mismatch struct {
	Tag, Lock, Local string
}

// Verify compares local image IDs with the lock. A missing lock or image is
// a mismatch (fail closed).
func (b *Builder) Verify(ctx context.Context) ([]Mismatch, map[string]any, error) {
	lock, err := ReadLock(b.Root)
	if err != nil {
		return nil, nil, err
	}
	imgs, _ := lock["images"].(map[string]any)
	if len(imgs) == 0 {
		return nil, lock, fmt.Errorf("%s lists no images", LockFile)
	}
	tags := make([]string, 0, len(imgs))
	for t := range imgs {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	var mm []Mismatch
	for _, t := range tags {
		e, _ := imgs[t].(map[string]any)
		want, _ := e["id"].(string)
		got, err := b.ImageID(ctx, t)
		if err != nil {
			got = "missing"
		}
		if got != want {
			mm = append(mm, Mismatch{Tag: t, Lock: want, Local: got})
		}
	}
	return mm, lock, nil
}
