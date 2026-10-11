package images

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Variants of the images (CONTRACT §5.2, §5.5, §9.1): the real build of the
// pinned commit; a mutant M<n> = the pinned commit + exactly
// mutants/M<n>.patch, of which only commitproof/multigres:M<n> is built
// (every patched package is linked only into Multigres binaries of that
// image); the profile h1 = the historical commit (§9.4).
const (
	VariantReal = "real"
	VariantH1   = "h1"
)

var mutantRE = regexp.MustCompile(`^M[0-9]+$`)

// IsMutant reports whether v names a mutant.
func IsMutant(v string) bool { return mutantRE.MatchString(v) }

// MultigresImageBinaries are the binaries of a commitproof/multigres image.
var MultigresImageBinaries = []string{"/multigres/bin/multigres", "/multigres/bin/multiadmin", "/multigres/bin/multigateway",
	"/multigres/bin/multiorch", "/multigres/bin/multipooler", "/usr/bin/pgbackrest"}

// tree is the source tree a variant is built from.
type tree struct {
	Variant string
	Dir     string // repository-relative
	Commit  string
	Patch   string // repository-relative mutant patch, or ""
}

func treeOf(v string) tree {
	switch {
	case v == VariantH1:
		return tree{Variant: v, Dir: ".cache/multigres-h1", Commit: params.H1Commit}
	case IsMutant(v):
		return tree{Variant: v, Dir: ".cache/multigres-" + v, Commit: params.MultigresCommit, Patch: "mutants/" + v + ".patch"}
	}
	return tree{Variant: VariantReal, Dir: SourceDir, Commit: params.MultigresCommit}
}

// ensureTree makes t.Dir exactly the tree to build, fail closed: the commit
// checked out (cloned if absent; a mutant tree is a git worktree of the real
// one) and, for a mutant, the patch applied and nothing else changed.
func (b *Builder) ensureTree(ctx context.Context, t tree) error {
	dir := t.Dir
	if _, err := os.Stat(filepath.Join(b.Root, dir, ".git")); errors.Is(err, os.ErrNotExist) {
		var steps [][]string
		if t.Patch != "" {
			if err := b.ensureTree(ctx, treeOf(VariantReal)); err != nil {
				return err
			}
			steps = [][]string{{"git", "-C", SourceDir, "worktree", "add", "-q", "--detach", "../" + filepath.Base(dir), t.Commit}}
		} else {
			if err := os.MkdirAll(filepath.Join(b.Root, dir), 0o755); err != nil {
				return err
			}
			steps = [][]string{
				{"git", "-C", dir, "init", "-q"},
				{"git", "-C", dir, "remote", "add", "origin", params.MultigresRepository},
				{"git", "-C", dir, "fetch", "-q", "--depth", "1", "origin", t.Commit},
				{"git", "-C", dir, "checkout", "-q", "FETCH_HEAD"},
			}
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
	if got := strings.TrimSpace(string(r.Stdout)); got != t.Commit {
		return fmt.Errorf("%s is at %s, not %s", dir, got, t.Commit)
	}
	if t.Patch != "" {
		// Back to the pristine commit (CommitProof's own build cache), then
		// exactly the patch.
		for _, s := range [][]string{
			{"git", "-C", dir, "reset", "-q", "--hard", t.Commit},
			{"git", "-C", dir, "clean", "-q", "-f", "-d", "-x"},
			{"git", "-C", dir, "apply", "--whitespace=nowarn", filepath.Join("..", "..", t.Patch)},
		} {
			if _, err := b.cmd(ctx, "git-"+s[3], s, cmdlog.Opts{}, ""); err != nil {
				return err
			}
		}
	}
	r, err = b.cmd(ctx, "git-status", []string{"git", "-C", dir, "status", "--porcelain", "--ignored"}, cmdlog.Opts{}, "")
	if err != nil {
		return err
	}
	status := strings.TrimRight(string(r.Stdout), "\n")
	if t.Patch == "" {
		if status != "" {
			return fmt.Errorf("%s has local changes or untracked files; the build context must be the pinned tree:\n%s", dir, status)
		}
		return nil
	}
	files, err := patchFiles(filepath.Join(b.Root, t.Patch))
	if err != nil {
		return err
	}
	var want []string
	for _, f := range files {
		want = append(want, " M "+f)
	}
	if status != strings.Join(want, "\n") {
		return fmt.Errorf("%s after applying %s: status %q, want exactly %q", dir, t.Patch, status, want)
	}
	return nil
}

// patchFiles lists the files a patch modifies (its "diff --git" headers).
func patchFiles(p string) ([]string, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(l, "diff --git a/"); ok {
			a, _, _ := strings.Cut(rest, " b/")
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s modifies no file", p)
	}
	sort.Strings(out)
	return out, nil
}

// variantSpecs are the images of a non-real variant.
func variantSpecs(v string) []Spec {
	t := treeOf(v)
	if IsMutant(v) {
		return []Spec{{Tag: "commitproof/multigres:" + v, Dockerfile: "build/Dockerfile", Upstream: "Dockerfile", Context: t.Dir,
			Binaries: MultigresImageBinaries, Pgbackrest: true,
			BuildArgs: [][2]string{{"GIT_COMMIT", params.MultigresCommit}, {"COMMIT_DATE", params.MultigresCommitDate}}}}
	}
	// h1: the historical commit's Dockerfiles (FROM-only derivations under
	// build/h1/, ADR-0025) and the same slowfs node layer as the real build.
	return []Spec{
		{Tag: "commitproof/multigres:h1", Dockerfile: "build/h1/Dockerfile", Upstream: "Dockerfile", Context: t.Dir,
			Binaries: MultigresImageBinaries, Pgbackrest: true},
		{Tag: "commitproof/pgctld-upstream:h1", Dockerfile: "build/h1/Dockerfile.pgctld", Upstream: "Dockerfile.pgctld", Context: t.Dir,
			Binaries: []string{"/usr/local/bin/pgctld", "/usr/bin/pgbackrest"}, Pgbackrest: true},
		{Tag: "commitproof/pgctld:h1", Dockerfile: "build/h1/Dockerfile.slowfs", Context: ".",
			Binaries: []string{"/usr/local/bin/pgctld", "/usr/bin/pgbackrest", "/usr/local/bin/slowfs"}, Pgbackrest: true,
			Versions: [][]string{{"dpkg-query", "-W", "fuse3"}}, Extends: "commitproof/pgctld-upstream:h1"},
	}
}

// VariantTags lists the image tags a variant adds to the real ones.
func VariantTags(v string) []string {
	if v == "" || v == VariantReal {
		return nil
	}
	var out []string
	for _, s := range variantSpecs(v) {
		out = append(out, s.Tag)
	}
	return out
}

// BuildVariant builds a mutant's or h1's images on top of an existing real
// lock and records them in it (entries with "variant": v, and the
// variant's record under "variants"). A mutant build fails unless every
// binary of its image that does not link a patched package has the real
// build's hash (CONTRACT §9.1, K4.4).
func (b *Builder) BuildVariant(ctx context.Context, v string, noCache bool) (map[string]any, error) {
	if v != VariantH1 && !IsMutant(v) {
		return nil, fmt.Errorf("unknown variant %q", v)
	}
	lock, err := ReadLock(b.Root)
	if err != nil {
		return nil, fmt.Errorf("a variant build needs the real build's lock: %w", err)
	}
	imgs, _ := lock["images"].(map[string]any)
	t := treeOf(v)
	if IsMutant(v) {
		if _, err := os.Stat(filepath.Join(b.Root, t.Patch)); err != nil {
			return nil, fmt.Errorf("mutant %s: %w", v, err)
		}
	}
	if err := b.ensureTree(ctx, t); err != nil {
		return nil, err
	}
	built, dockerfiles, bases, err := b.buildSpecs(ctx, t, variantSpecs(v), noCache, imgs)
	if err != nil {
		return nil, err
	}
	bmv := map[string]any{}
	for k, d := range bases {
		bmv[k] = d
	}
	rec := map[string]any{"variant": v, "source_tree": t.Dir, "commit": t.Commit, "repository": params.MultigresRepository,
		"base_images": bmv}
	if IsMutant(v) {
		raw, err := os.ReadFile(filepath.Join(b.Root, t.Patch))
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(raw)
		files, _ := patchFiles(filepath.Join(b.Root, t.Patch))
		iso, ok, err := b.isolation(ctx, t, files, imgs, built)
		if err != nil {
			return nil, err
		}
		rec["patch"], rec["patch_sha256"], rec["files"], rec["isolation"] = t.Patch, hex.EncodeToString(sum[:]), strList(files), iso
		if !ok {
			return nil, fmt.Errorf("mutant %s: a binary without a patched package differs from the real build: %v", v, iso)
		}
	}
	for tag, e := range built {
		m := e.(map[string]any)
		m["variant"] = v
		imgs[tag] = m
	}
	dfs, _ := lock["dockerfiles"].(map[string]any)
	for k, x := range dockerfiles {
		if _, real := dfs[k]; !real {
			dfs[k] = x
		}
	}
	bm, _ := lock["base_images"].(map[string]any)
	for k, d := range bases {
		if prev, ok := bm[k].(string); ok && prev != d {
			return nil, fmt.Errorf("variant %s pins base %s to %s, the real build to %s", v, k, d, prev)
		}
		bm[k] = d
	}
	vs, _ := lock["variants"].(map[string]any)
	if vs == nil {
		vs = map[string]any{}
	}
	vs[v] = rec
	lock["variants"] = vs
	return lock, writeLock(b.Root, lock)
}

func strList(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// isolation compares each binary of the mutant image with the real build:
// binaries that do not link a patched package must be identical. Which
// binaries link which packages is computed with `go list -deps` on the
// mutant tree.
func (b *Builder) isolation(ctx context.Context, t tree, files []string, real, built map[string]any) (map[string]any, bool, error) {
	pkgs := map[string]bool{}
	for _, f := range files {
		pkgs["github.com/multigres/multigres/"+filepath.ToSlash(filepath.Dir(f))] = true
	}
	re, _ := real["commitproof/multigres:real"].(map[string]any)
	rb, _ := re["binaries"].(map[string]any)
	me, _ := built["commitproof/multigres:"+t.Variant].(map[string]any)
	mb, _ := me["binaries"].(map[string]any)
	if rb == nil || mb == nil {
		return nil, false, fmt.Errorf("isolation: binary hashes missing (real %v, mutant %v)", rb != nil, mb != nil)
	}
	out := map[string]any{}
	ok := true
	for _, bin := range MultigresImageBinaries {
		contains := false
		var linked []string
		if strings.HasPrefix(bin, "/multigres/bin/") {
			name := filepath.Base(bin)
			r, err := b.cmd(ctx, "go-list-deps-"+name, []string{"go", "-C", t.Dir, "list", "-deps", "./go/cmd/" + name},
				cmdlog.Opts{Env: []string{"GOTOOLCHAIN=go1.26.9", "CGO_ENABLED=0"}}, "build")
			if err != nil {
				return nil, false, err
			}
			for _, d := range strings.Fields(string(r.Stdout)) {
				if pkgs[d] {
					contains = true
					linked = append(linked, d)
				}
			}
		}
		equal := rb[bin] == mb[bin]
		e := map[string]any{"baseline_sha256": rb[bin], "mutant_sha256": mb[bin], "contains_patched_package": contains,
			"patched_packages_linked": strList(linked), "equal_to_baseline": equal}
		if !contains && !equal {
			ok = false
		}
		out[bin] = e
	}
	return out, ok, nil
}

// keepVariants copies into next the variant entries of prev whose local
// images still carry the locked IDs; for mutants the real multigres image
// must also be unchanged (their isolation was computed against it).
func (b *Builder) keepVariants(ctx context.Context, prev, next map[string]any) {
	pv, _ := prev["variants"].(map[string]any)
	pi, _ := prev["images"].(map[string]any)
	ni, _ := next["images"].(map[string]any)
	nd, _ := next["dockerfiles"].(map[string]any)
	pd, _ := prev["dockerfiles"].(map[string]any)
	nb, _ := next["base_images"].(map[string]any)
	realID := func(m map[string]any) string {
		e, _ := m["commitproof/multigres:real"].(map[string]any)
		id, _ := e["id"].(string)
		return id
	}
	kept := map[string]any{}
	for v, rec := range pv {
		ok := true
		if IsMutant(v) && realID(pi) != realID(ni) {
			ok = false
		}
		for _, tag := range VariantTags(v) {
			e, _ := pi[tag].(map[string]any)
			want, _ := e["id"].(string)
			if got, err := b.ImageID(ctx, tag); err != nil || got != want || want == "" {
				ok = false
			}
		}
		if !ok {
			continue
		}
		kept[v] = rec
		if r, _ := rec.(map[string]any); r != nil {
			vb, _ := r["base_images"].(map[string]any)
			for k, d := range vb {
				if _, has := nb[k]; !has {
					nb[k] = d
				}
			}
		}
		for _, tag := range VariantTags(v) {
			ni[tag] = pi[tag]
			if e, _ := pi[tag].(map[string]any); e != nil {
				if df, _ := e["dockerfile"].(string); df != "" {
					if _, has := nd[df]; !has {
						nd[df] = pd[df]
					}
				}
			}
		}
	}
	if len(kept) > 0 {
		next["variants"] = kept
	}
}

// RequiredTags are the lock entries a cluster of variant v uses: every real
// entry and the variant's own.
func RequiredTags(lock map[string]any, v string) []string {
	imgs, _ := lock["images"].(map[string]any)
	var out []string
	for tag, e := range imgs {
		m, _ := e.(map[string]any)
		if ev, _ := m["variant"].(string); ev == "" || ev == VariantReal {
			out = append(out, tag)
		}
	}
	out = append(out, VariantTags(v)...)
	sort.Strings(out)
	return out
}
