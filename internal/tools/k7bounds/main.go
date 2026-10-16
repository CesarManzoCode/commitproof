// Command k7bounds prints CONTRACT §12.3's statistics for the cell sizes of
// K7's matrix (P15: N = 10 for V1 and V3, N = 5 for V2, V4, V5, mutants and
// H1), with internal/stats' integer arithmetic (ADR-0036): for every k the
// Wilson 95 % interval of k/N (lower end rounded down, upper end up), and
// for k = 0 the one-sided bound 1 − 0.05^(1/N) (rounded up). One JSON line
// per (N, k), then a summary line.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/stats"
)

func main() {
	lines := 0
	for _, n := range []int64{params.RepsV2V4V5, params.RepsV1V3} {
		zb, err := stats.ZeroFailureBound95(n)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for k := int64(0); k <= n; k++ {
			iv, err := stats.Wilson95(k, n)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			m := map[string]any{"n": n, "k": k, "wilson_lo": stats.Down(iv.Lo), "wilson_hi": stats.Up(iv.Hi)}
			if k == 0 {
				m["zero_failure_bound"] = stats.Up(zb)
			}
			b, _ := json.Marshal(m)
			fmt.Println(string(b))
			lines++
		}
	}
	fmt.Printf("{\"command\":\"k7bounds\",\"lines\":%d,\"exit_code\":0}\n", lines)
}
