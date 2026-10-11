package gate

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// GitBlobID is the git object id of content (sha1 of "blob <len>\0<content>"),
// so a copy of an upstream file can be checked against the pinned tree
// without git.
func GitBlobID(content []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

var (
	fromLine   = regexp.MustCompile(`^FROM\s+(\S+)(\s+AS\s+(\S+))?\s*$`)
	pinnedRef  = regexp.MustCompile(`^[a-z0-9./_-]+(:[A-Za-z0-9._-]+)?@sha256:[0-9a-f]{64}$`)
	latestPart = regexp.MustCompile(`:latest(@|$)`)
)

// DerivationReport is the result of CheckDerivation.
type DerivationReport struct {
	OK         bool     `json:"ok"`
	Lines      int      `json:"lines"`
	Changed    []string `json:"changed"`
	Violations []string `json:"violations"`
}

// CheckDerivation enforces CONTRACT §5.2 / K1.3: derived differs from
// upstream only in FROM lines, line for line; every changed FROM line keeps
// its stage name and references a digest-pinned image (no "latest"), and
// every FROM line of derived is digest-pinned.
func CheckDerivation(upstream, derived []byte) DerivationReport {
	r := DerivationReport{Changed: []string{}, Violations: []string{}}
	u := strings.Split(string(upstream), "\n")
	d := strings.Split(string(derived), "\n")
	r.Lines = len(d)
	if len(u) != len(d) {
		r.Violations = append(r.Violations, fmt.Sprintf("line count differs: upstream %d, derived %d", len(u), len(d)))
	}
	n := min(len(u), len(d))
	for i := 0; i < n; i++ {
		dm := fromLine.FindStringSubmatch(d[i])
		if dm != nil && (!pinnedRef.MatchString(dm[1]) || latestPart.MatchString(dm[1])) {
			r.Violations = append(r.Violations, fmt.Sprintf("line %d: FROM %s is not digest-pinned", i+1, dm[1]))
		}
		if u[i] == d[i] {
			continue
		}
		um := fromLine.FindStringSubmatch(u[i])
		if um == nil || dm == nil {
			r.Violations = append(r.Violations, fmt.Sprintf("line %d: non-FROM line differs: upstream %q, derived %q", i+1, u[i], d[i]))
			continue
		}
		if um[3] != dm[3] {
			r.Violations = append(r.Violations, fmt.Sprintf("line %d: stage name changed from %q to %q", i+1, um[3], dm[3]))
		}
		r.Changed = append(r.Changed, fmt.Sprintf("line %d: %s -> %s", i+1, um[1], dm[1]))
	}
	r.OK = len(r.Violations) == 0
	return r
}

// FromRefs returns the image references of every FROM line.
func FromRefs(dockerfile []byte) []string {
	var refs []string
	for _, l := range strings.Split(string(dockerfile), "\n") {
		if m := fromLine.FindStringSubmatch(l); m != nil {
			refs = append(refs, m[1])
		}
	}
	return refs
}
