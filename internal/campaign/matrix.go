package campaign

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
)

// Matrix is a campaign matrix file (`commitproof campaign --matrix`).
type Matrix struct {
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
	// MinimizeFailures: every FAIL run of the matrix is minimized and its
	// minimal plan replayed Replays times (CONTRACT §12.2, K4.5).
	MinimizeFailures bool `json:"minimize_failures"`
	Replays          int  `json:"replays"`
}

// Entry is one matrix row: a campaign (or directed schedule, or the probe
// suite) on a profile and build, for each seed.
type Entry struct {
	Campaign string   `json:"campaign"`
	Profile  string   `json:"profile"`
	Mutant   string   `json:"mutant"`
	Seeds    []uint64 `json:"seeds"`
}

// ReadMatrix reads and validates a matrix file.
func ReadMatrix(path string) (*Matrix, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Matrix
	dec := json.NewDecoder(bytesReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if m.Replays == 0 {
		m.Replays = params.Replays
	}
	if m.Replays != params.Replays {
		return nil, fmt.Errorf("%s: replays %d, CONTRACT P13 says %d", path, m.Replays, params.Replays)
	}
	for i := range m.Entries {
		e := &m.Entries[i]
		if e.Profile == "" {
			e.Profile = "baseline"
		}
		if len(e.Seeds) == 0 {
			return nil, fmt.Errorf("%s: entry %d (%s) has no seeds", path, i, e.Campaign)
		}
		if _, err := plan.Generate(e.Campaign, e.Seeds[0], e.Profile, ""); err != nil {
			return nil, fmt.Errorf("%s: entry %d: %w", path, i, err)
		}
	}
	return &m, nil
}

// Cell is the outcome of one (entry, seed).
type Cell struct {
	Entry   Entry
	Seed    uint64
	Run     *Run
	Valid   bool
	Invalid []*Run
}

// RunMatrix runs every cell (skipping cells that already have a valid
// sealed run), then minimizes and replays every FAIL when asked. It returns
// the cells and whether every cell got a valid run and every follow-up
// completed.
func (d *Driver) RunMatrix(ctx context.Context, m *Matrix) ([]Cell, bool, error) {
	var cells []Cell
	complete := true
	for _, e := range m.Entries {
		for _, seed := range e.Seeds {
			p, err := plan.Generate(e.Campaign, seed, e.Profile, "")
			if err != nil {
				return cells, false, err
			}
			r, inv, ok, err := d.Ensure(ctx, Spec{Plan: p, Profile: e.Profile, Mutant: e.Mutant})
			if err != nil {
				return cells, false, err
			}
			c := Cell{Entry: e, Seed: seed, Run: r, Valid: ok, Invalid: inv}
			cells = append(cells, c)
			complete = complete && ok
			d.emit(map[string]any{"event": "cell", "campaign": e.Campaign, "profile": e.Profile, "mutant": mutantName(e.Mutant),
				"seed": seed, "run": idOf(r), "run_dir": d.Out + "/" + idOf(r), "verdict": verdictOf(r), "valid": ok,
				"classes": strAny(classesOf(r)), "invalid_attempts": ids(inv)})
		}
	}
	if !m.MinimizeFailures {
		return cells, complete, nil
	}
	for _, c := range cells {
		if !c.Valid || c.Run.Verdict != "FAIL" {
			continue
		}
		ok, err := d.FollowUp(ctx, c.Run, m.Replays)
		if err != nil {
			return cells, false, err
		}
		complete = complete && ok
	}
	return cells, complete, nil
}

// FollowUp minimizes a FAIL run and replays its minimal plan (K4.5).
func (d *Driver) FollowUp(ctx context.Context, r *Run, replays int) (bool, error) {
	mz, err := d.Minimize(ctx, r)
	if err != nil {
		return false, fmt.Errorf("minimize %s: %w", r.ID, err)
	}
	reps, hits, err := d.Replay(ctx, mz.Run, replays, mz.Class)
	if err != nil {
		return false, fmt.Errorf("replay %s: %w", mz.Run.ID, err)
	}
	d.emit(map[string]any{"event": "follow-up", "run": r.ID, "class": mz.Class, "minimized_run": mz.Run.ID,
		"replays": ids(reps), "replays_with_class": int64(hits), "reproducibility": Reproducibility(hits)})
	return true, nil
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
