// Package host holds host-side helpers shared by the images and cluster
// packages: repository discovery, source identity, and the timeouts every
// Docker or git call uses (AGENTS.md §4: derived from CONTRACT §5.7).
package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Timeouts derived from CONTRACT §5.7.
const (
	// QuickTimeout bounds docker version/inspect/ps/info and git queries (P01).
	QuickTimeout = params.ClientOpTimeout
	// ComposeTimeout bounds compose up/down and the HEALTHY wait (P09).
	ComposeTimeout = params.HealthyDeadline
	// BuildTimeout bounds one image build: 6 × P09 (a cold Multigres build
	// compiles eight Go binaries and installs pgBackRest).
	BuildTimeout = 6 * params.HealthyDeadline
	// LogsTimeout bounds `docker compose logs` of one service: 6 × P01.
	LogsTimeout = 6 * params.ClientOpTimeout
	// GitTimeout bounds clone/fetch of the pinned commit (P09).
	GitTimeout = params.HealthyDeadline
)

// RepoRoot walks up from the working directory to the directory holding
// go.mod and deploy/compose.yaml.
func RepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "deploy", "compose.yaml")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not inside the CommitProof repository (go.mod + deploy/compose.yaml not found)")
		}
		dir = parent
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Source is CommitProof's own source identity.
type Source struct {
	Commit string
	Dirty  bool
}

// SourceIdentity reports HEAD and whether code differs from it. Changes
// under evidence/ and to images.lock.json are outputs, not code, and do not
// count (buildinfo documents the same rule).
func SourceIdentity(root string) (Source, error) {
	ctx, cancel := context.WithTimeout(context.Background(), QuickTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return Source{}, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	st, err := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain", "--untracked-files=normal",
		"--", ".", ":(exclude)evidence", ":(exclude)images.lock.json").Output()
	if err != nil {
		return Source{}, fmt.Errorf("git status: %w", err)
	}
	return Source{Commit: strings.TrimSpace(string(out)), Dirty: len(strings.TrimSpace(string(st))) > 0}, nil
}

// UTCStamp formats t as yyyymmddThhmmssZ (CONTRACT §5.4 run id).
func UTCStamp(t time.Time) string { return t.UTC().Format("20060102T150405Z") }
