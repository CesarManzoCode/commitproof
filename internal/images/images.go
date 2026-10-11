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
	"time"

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
	// Versions are commands run in the image whose output is locked
	// (packages installed from the base's repositories without a pin).
	Versions [][]string
	// Extends is the tag of the image this one is built FROM (a local
	// image): its layers must be a prefix of this image's (ADR-0022).
	Extends string
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
		{Tag: "commitproof/pgctld-upstream:real", Dockerfile: "build/Dockerfile.pgctld", Upstream: "Dockerfile.pgctld", Context: SourceDir,
			Binaries: []string{"/usr/local/bin/pgctld", "/usr/bin/pgbackrest"}, Pgbackrest: true, BuildArgs: mg},
		// §5.2: the node image = pgctld-upstream + fuse3 + slowfs (K3).
		{Tag: "commitproof/pgctld:real", Dockerfile: "build/Dockerfile.slowfs", Context: ".",
			Binaries: []string{"/usr/local/bin/pgctld", "/usr/bin/pgbackrest", "/usr/local/bin/slowfs"}, Pgbackrest: true,
			Versions: [][]string{{"dpkg-query", "-W", "fuse3"}}, Extends: "commitproof/pgctld-upstream:real"},
		{Tag: "commitproof/nemesis:real", Dockerfile: "build/Dockerfile.nemesis", Context: ".",
			Binaries: []string{"/usr/local/bin/cpnet"},
			Versions: [][]string{{"iptables", "--version"}, {"tc", "-V"}}},
		{Tag: "commitproof/runner:real", Dockerfile: "build/Dockerfile.runner", Context: ".",
			Binaries:  []string{"/usr/local/bin/commitproof"},
			BuildArgs: [][2]string{{"CP_COMMIT", src.Commit}, {"CP_DIRTY", fmt.Sprint(src.Dirty)}},
			Versions:  [][]string{{"docker", "--version"}}},
	}
}

// RunnerTag is the runner image tag for a set of nemesis sabotage tags
// ("real" when none).
func RunnerTag(sabotage []string) string {
	if len(sabotage) == 0 {
		return "real"
	}
	return "sab-" + strings.Join(sabotage, "-")
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
	return b.ensureTree(ctx, treeOf(VariantReal))
}

// upstreamFile returns a file of tree t at its commit.
func (b *Builder) upstreamFile(ctx context.Context, t tree, name string) ([]byte, error) {
	r, err := b.cmd(ctx, "git-show-"+name, []string{"git", "-C", t.Dir, "show", t.Commit + ":" + name}, cmdlog.Opts{}, "")
	return r.Stdout, err
}

// Build builds every image of the real build, hashes their binaries and
// writes the lock. Entries of other variants (mutants, h1) already in the
// lock are kept when their local images are still there with the locked
// IDs (otherwise they must be rebuilt with --mutant / --profile).
func (b *Builder) Build(ctx context.Context, noCache bool) (map[string]any, error) {
	src, err := host.SourceIdentity(b.Root)
	if err != nil {
		return nil, err
	}
	t := treeOf(VariantReal)
	if err := b.ensureTree(ctx, t); err != nil {
		return nil, err
	}
	imgs, dockerfiles, bases, err := b.buildSpecs(ctx, t, Specs(src), noCache, nil)
	if err != nil {
		return nil, err
	}
	if err := composeBases(b.Root, bases); err != nil {
		return nil, err
	}
	for tag, e := range imgs {
		e.(map[string]any)["variant"] = VariantReal
		imgs[tag] = e
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
	if prev, err := ReadLock(b.Root); err == nil {
		b.keepVariants(ctx, prev, lock)
	}
	return lock, writeLock(b.Root, lock)
}

func writeLock(root string, lock map[string]any) error {
	out, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, LockFile), append(out, '\n'), 0o644)
}

// composeBases adds the digest-pinned images deploy/compose.yaml pulls.
func composeBases(root string, bases map[string]string) error {
	compose, err := os.ReadFile(filepath.Join(root, "deploy", "compose.yaml"))
	if err != nil {
		return err
	}
	for _, l := range strings.Split(string(compose), "\n") {
		l = strings.TrimSpace(l)
		if ref, ok := strings.CutPrefix(l, "image: "); ok && !strings.HasPrefix(ref, "commitproof/") {
			name, digest, ok := strings.Cut(ref, "@")
			if !ok {
				return fmt.Errorf("deploy/compose.yaml: image %s is not digest-pinned", ref)
			}
			bases[name] = digest
		}
	}
	return nil
}

// buildSpecs builds specs from source tree t: each Dockerfile is checked
// (FROM-only derivation of the tree's upstream file, digest-pinned bases),
// built, its binaries hashed and versions recorded. prev holds entries of
// images built earlier in the same variant (for Extends).
func (b *Builder) buildSpecs(ctx context.Context, t tree, specs []Spec, noCache bool, prev map[string]any) (map[string]any, map[string]any, map[string]string, error) {
	dockerfiles := map[string]any{}
	bases := map[string]string{}
	for _, s := range specs {
		content, err := os.ReadFile(filepath.Join(b.Root, s.Dockerfile))
		if err != nil {
			return nil, nil, nil, err
		}
		sum := sha256.Sum256(content)
		entry := map[string]any{"sha256": hex.EncodeToString(sum[:])}
		if s.Upstream != "" {
			up, err := b.upstreamFile(ctx, t, s.Upstream)
			if err != nil {
				return nil, nil, nil, err
			}
			rep := gate.CheckDerivation(up, content)
			if !rep.OK {
				return nil, nil, nil, fmt.Errorf("%s is not a FROM-only derivation of upstream %s: %v", s.Dockerfile, s.Upstream, rep.Violations)
			}
			entry["upstream"] = s.Upstream + "@" + t.Commit
			entry["upstream_blob"] = gate.GitBlobID(up)
		}
		dockerfiles[s.Dockerfile] = entry
		for _, ref := range gate.FromRefs(content) {
			if s.Extends != "" && ref == s.Extends {
				// A local image built by this run, checked by layers
				// after the build (ADR-0022).
				continue
			}
			name, digest, ok := strings.Cut(ref, "@")
			if !ok {
				return nil, nil, nil, fmt.Errorf("%s: FROM %s is not digest-pinned", s.Dockerfile, ref)
			}
			if p, dup := bases[name]; dup && p != digest {
				return nil, nil, nil, fmt.Errorf("base %s pinned to two digests", name)
			}
			bases[name] = digest
		}
	}
	imgs := map[string]any{}
	for k, v := range prev {
		imgs[k] = v
	}
	for _, s := range specs {
		argv := []string{"docker", "buildx", "build", "--progress=plain", "--provenance=false", "--sbom=false",
			"--file", s.Dockerfile, "--tag", s.Tag}
		for _, a := range s.BuildArgs {
			argv = append(argv, "--build-arg", a[0]+"="+a[1])
		}
		if noCache {
			argv = append(argv, "--no-cache")
		}
		argv = append(argv, s.Context)
		// Up to three attempts: the builds download Go modules and apt
		// packages, and a transient network failure (seen in K1: an IPv6
		// "network is unreachable" from proxy.golang.org) is not a property
		// of the build. Every attempt is recorded; nothing else is retried.
		name := "build-" + strings.ReplaceAll(path.Base(s.Tag), ":", "-")
		var berr error
		for attempt := 1; attempt <= 3; attempt++ {
			if _, berr = b.cmd(ctx, fmt.Sprintf("%s-attempt-%d", name, attempt), argv, cmdlog.Opts{}, "build"); berr == nil {
				break
			}
			if attempt < 3 {
				time.Sleep(10 * time.Second)
			}
		}
		if berr != nil {
			return nil, nil, nil, berr
		}
		id, err := b.ImageID(ctx, s.Tag)
		if err != nil {
			return nil, nil, nil, err
		}
		bins, err := b.hashBinaries(ctx, s)
		if err != nil {
			return nil, nil, nil, err
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
				return nil, nil, nil, err
			}
			entry["pgbackrest_version"] = strings.TrimSpace(string(r.Stdout))
		}
		if len(s.Versions) > 0 {
			vs := map[string]any{}
			for _, v := range s.Versions {
				argv := append([]string{"docker", "run", "--rm", "--label", projectLabel, "--network", "none", "--user", "0:0",
					"--entrypoint", v[0], s.Tag}, v[1:]...)
				r, err := b.cmd(ctx, "version-"+v[0], argv, cmdlog.Opts{}, "")
				if err != nil {
					return nil, nil, nil, err
				}
				vs[strings.Join(v, " ")] = strings.TrimSpace(string(r.Stdout) + string(r.Stderr))
			}
			entry["versions"] = vs
		}
		if s.Extends != "" {
			base, err := b.ImageID(ctx, s.Extends)
			if err != nil {
				return nil, nil, nil, err
			}
			pe, _ := imgs[s.Extends].(map[string]any)
			if want, _ := pe["id"].(string); want != base {
				return nil, nil, nil, fmt.Errorf("%s: base %s is %s, not the image built above (%s)", s.Tag, s.Extends, base, want)
			}
			if err := b.checkLayers(ctx, s.Extends, s.Tag); err != nil {
				return nil, nil, nil, err
			}
			entry["extends"] = map[string]any{"tag": s.Extends, "id": base, "layers_prefix": true}
		}
		imgs[s.Tag] = entry
	}
	// Return only the images built here (prev serves Extends lookups; an
	// earlier entry of a rebuilt tag is replaced, not dropped).
	built := map[string]any{}
	for _, s := range specs {
		built[s.Tag] = imgs[s.Tag]
	}
	return built, dockerfiles, bases, nil
}

// checkLayers requires the layers of base to be a prefix of derived's: the
// derived image is base plus its own layers, with base unchanged.
func (b *Builder) checkLayers(ctx context.Context, base, derived string) error {
	layers := func(ref string) ([]string, error) {
		r, err := b.cmd(ctx, "layers", []string{"docker", "image", "inspect", "--format", "{{json .RootFS.Layers}}", ref}, cmdlog.Opts{}, "")
		if err != nil {
			return nil, err
		}
		var ls []string
		return ls, json.Unmarshal(r.Stdout, &ls)
	}
	lb, err := layers(base)
	if err != nil {
		return err
	}
	ld, err := layers(derived)
	if err != nil {
		return err
	}
	if len(ld) <= len(lb) {
		return fmt.Errorf("%s has %d layers, not more than its base %s (%d)", derived, len(ld), base, len(lb))
	}
	for i := range lb {
		if lb[i] != ld[i] {
			return fmt.Errorf("%s layer %d is %s, base %s has %s", derived, i, ld[i], base, lb[i])
		}
	}
	return nil
}

// BuildSabotageRunner builds the runner image with one nemesis sabotage
// tag set (K3.2, K3.3), tagged commitproof/runner:<RunnerTag> and labelled
// with the tags and the commit. These images are test fixtures: never in
// the lock.
func (b *Builder) BuildSabotageRunner(ctx context.Context, tags []string) (string, error) {
	src, err := host.SourceIdentity(b.Root)
	if err != nil {
		return "", err
	}
	var full []string
	for _, t := range tags {
		full = append(full, "cp_sabotage_"+t)
	}
	tag := "commitproof/runner:" + RunnerTag(tags)
	argv := []string{"docker", "buildx", "build", "--progress=plain", "--provenance=false", "--sbom=false",
		"--file", "build/Dockerfile.runner", "--tag", tag,
		"--build-arg", "CP_COMMIT=" + src.Commit, "--build-arg", fmt.Sprintf("CP_DIRTY=%v", src.Dirty),
		"--build-arg", "CP_BUILD_TAGS=" + strings.Join(full, ","), "."}
	if _, err := b.cmd(ctx, "build-sabotage-runner", argv, cmdlog.Opts{}, "build"); err != nil {
		return "", err
	}
	return b.ImageID(ctx, tag)
}

// ImageLabel returns one label of a local image.
func (b *Builder) ImageLabel(ctx context.Context, ref, label string) (string, error) {
	r, err := b.cmd(ctx, "image-label", []string{"docker", "image", "inspect", "--format", "{{index .Config.Labels \"" + label + "\"}}", ref}, cmdlog.Opts{}, "")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(r.Stdout)), nil
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

// Verify compares local image IDs with the lock: the entries of variant
// (RequiredTags; "" = every entry of the lock). A missing lock, entry or
// image is a mismatch (fail closed).
func (b *Builder) Verify(ctx context.Context, variant string) ([]Mismatch, map[string]any, error) {
	lock, err := ReadLock(b.Root)
	if err != nil {
		return nil, nil, err
	}
	imgs, _ := lock["images"].(map[string]any)
	if len(imgs) == 0 {
		return nil, lock, fmt.Errorf("%s lists no images", LockFile)
	}
	var tags []string
	if variant == "" {
		for t := range imgs {
			tags = append(tags, t)
		}
		sort.Strings(tags)
	} else {
		tags = RequiredTags(lock, variant)
	}
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
