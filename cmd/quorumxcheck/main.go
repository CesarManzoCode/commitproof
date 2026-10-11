// Command quorumxcheck is the FlexPaxos quorum-table cross-check of CONTRACT
// §10.8. For every cohort size n ∈ [1,9], AT_LEAST_N policy N ∈ [1,n] and
// recruited-set size r ∈ [0,n] it computes, from the definitions alone:
//
//	(a) term uniqueness: any two recruited sets of size r intersect (2r > n);
//	(b) no rogue quorum: the unrecruited n − r cohort members cannot hold
//	    N acknowledgements (n − r < N);
//	(c) FlexPaxos (S76): every recruited set intersects every durability
//	    quorum of size N (r + N > n);
//
// asserts (b) ⇔ (c), and compares sufficient = (a) ∧ (b) with upstream's
// consensus.CheckSufficientRecruitment at the pinned commit. It also lists
// the (n, N) for which the majority term ⌊n/2⌋+1 and the revocation term
// n−N+1 of the recruitment threshold coincide (I3).
//
// This is the only binary besides the observation code that imports a
// Multigres package; cpcheck, cpgate and cpreport stay independent of it
// (§5.3, enforced by internal/independence).
//
// Exit codes: 0 no mismatch, 1 a mismatch or a violated (b) ⇔ (c), 3 usage.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/multigres/multigres/go/common/consensus"
	clustermetadatapb "github.com/multigres/multigres/go/pb/clustermetadata"
)

const maxN = 9

func ids(k int) []*clustermetadatapb.ID {
	out := make([]*clustermetadatapb.ID, k)
	for i := range out {
		out[i] = &clustermetadatapb.ID{Component: clustermetadatapb.ID_MULTIPOOLER, Cell: "zone1", Name: fmt.Sprintf("p%d", i+1)}
	}
	return out
}

// Table computes the cross-check.
func Table() (rows []any, mismatches []any, bcViolations int64, coincide []any, err error) {
	for n := 1; n <= maxN; n++ {
		cohort := ids(n)
		for N := 1; N <= n; N++ {
			policy, perr := consensus.NewPolicyFromProto(&clustermetadatapb.DurabilityPolicy{
				QuorumType: clustermetadatapb.QuorumType_QUORUM_TYPE_AT_LEAST_N, RequiredCount: int32(N)})
			if perr != nil {
				return nil, nil, 0, nil, fmt.Errorf("n=%d N=%d: %w", n, N, perr)
			}
			majority, revocation := n/2+1, n-N+1
			if majority == revocation {
				coincide = append(coincide, map[string]any{"n": int64(n), "N": int64(N), "term": int64(majority)})
			}
			for r := 0; r <= n; r++ {
				a := 2*r > n
				b := n-r < N
				c := r+N > n
				if b != c {
					bcViolations++
				}
				sufficient := a && b
				uerr := consensus.CheckSufficientRecruitment(policy, cohort, cohort[:r])
				upstream := uerr == nil
				row := map[string]any{"n": int64(n), "N": int64(N), "r": int64(r), "a_term_uniqueness": a, "b_no_rogue_quorum": b,
					"c_flexpaxos_intersection": c, "sufficient": sufficient, "upstream_sufficient": upstream,
					"threshold": int64(max(majority, revocation))}
				if uerr != nil {
					row["upstream_error"] = uerr.Error()
				}
				rows = append(rows, row)
				if upstream != sufficient {
					mismatches = append(mismatches, row)
				}
			}
		}
	}
	return rows, mismatches, bcViolations, coincide, nil
}

func main() {
	fs := flag.NewFlagSet("quorumxcheck", flag.ContinueOnError)
	out := fs.String("out", "", "write the full table (canonical JSON) here; default: stdout before the summary")
	if err := fs.Parse(os.Args[1:]); err != nil || fs.NArg() != 0 {
		fmt.Println(`{"command":"quorumxcheck","error":"usage: quorumxcheck [--out <file>]","exit_code":3}`)
		os.Exit(3)
	}
	rows, mm, bc, co, err := Table()
	if err != nil {
		fmt.Printf("{\"command\":\"quorumxcheck\",\"error\":%q,\"exit_code\":3}\n", err.Error())
		os.Exit(3)
	}
	table := map[string]any{"multigres_commit": params.MultigresCommit, "function": "consensus.CheckSufficientRecruitment",
		"policy": "AT_LEAST_N", "n_range": []any{int64(1), int64(maxN)}, "rows": rows, "mismatches": mm,
		"b_iff_c_violations": bc, "coincident_terms": co}
	tb, err := canonjson.Marshal(table)
	if err != nil {
		fmt.Printf("{\"command\":\"quorumxcheck\",\"error\":%q,\"exit_code\":3}\n", err.Error())
		os.Exit(3)
	}
	if *out != "" {
		if err := os.WriteFile(*out, append(tb, '\n'), 0o644); err != nil {
			fmt.Printf("{\"command\":\"quorumxcheck\",\"error\":%q,\"exit_code\":3}\n", err.Error())
			os.Exit(3)
		}
	} else {
		fmt.Println(string(tb))
	}
	code := int64(0)
	if len(mm) > 0 || bc > 0 {
		code = 1
	}
	sum := map[string]any{"command": "quorumxcheck", "rows": int64(len(rows)), "mismatches": int64(len(mm)), "b_iff_c_violations": bc,
		"coincident_terms": co, "multigres_commit": params.MultigresCommit, "exit_code": code}
	if *out != "" {
		ts := sha256.Sum256(append(tb, '\n'))
		sum["table"], sum["table_sha256"] = *out, hex.EncodeToString(ts[:])
	}
	sb, _ := canonjson.Marshal(sum)
	fmt.Println(string(sb))
	os.Exit(int(code))
}
