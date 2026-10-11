package params

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contractRows parses the §5.7 table of docs/CONTRACT.md: the rows between the
// "### 5.7" heading and the next heading whose first cell is a P-id.
func contractRows(t *testing.T) [][]string {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "docs", "CONTRACT.md"))
	if err != nil {
		t.Fatalf("open contract: %v", err)
	}
	defer f.Close()
	var rows [][]string
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			in = strings.HasPrefix(line, "### 5.7 ")
			continue
		}
		if !in || !strings.HasPrefix(line, "| P") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), " | ")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read contract: %v", err)
	}
	return rows
}

func TestTableMatchesContract(t *testing.T) {
	rows := contractRows(t)
	table := Table()
	if len(rows) != len(table) {
		t.Fatalf("contract §5.7 has %d P-rows, params.Table has %d", len(rows), len(table))
	}
	for i, row := range rows {
		if len(row) != 4 {
			t.Fatalf("contract row %d: want 4 cells, got %d: %q", i, len(row), row)
		}
		p := table[i]
		if row[0] != p.ID || row[1] != p.Name || row[2] != p.Value {
			t.Errorf("row %s drifted:\n contract: %q | %q | %q\n code:     %q | %q | %q",
				row[0], row[0], row[1], row[2], p.ID, p.Name, p.Value)
		}
		if len(p.Derived) == 0 {
			t.Errorf("%s: no derived fragments tie typed constants to the text", p.ID)
		}
		for _, d := range p.Derived {
			if !strings.Contains(row[2], d) {
				t.Errorf("%s: typed constant renders %q, not found in contract value %q", p.ID, d, row[2])
			}
		}
	}
}

func TestTableIsFresh(t *testing.T) {
	a := Table()
	a[0].Value = "mutated"
	if Table()[0].Value == "mutated" {
		t.Fatal("Table shares state between calls")
	}
}

func TestContractVersion(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "CONTRACT.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "| Contract version | **" + ContractVersion + "**"
	if !strings.Contains(string(b), want) {
		t.Fatalf("CONTRACT.md header does not state version %s", ContractVersion)
	}
}

// TestToolchainPin enforces CONTRACT §14 / ADR-0019: go.mod declares
// go 1.26.9 (and no redundant toolchain line), and the Makefile pins
// GOTOOLCHAIN=go1.26.9.
func TestToolchainPin(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\ngo 1.26.9\n") {
		t.Error("go.mod lacks \"go 1.26.9\"")
	}
	if strings.Contains(string(b), "\ntoolchain ") {
		t.Error("go.mod has a toolchain line (ADR-0019)")
	}
	mk, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mk), "\nexport GOTOOLCHAIN := go1.26.9\n") {
		t.Error("Makefile does not export GOTOOLCHAIN := go1.26.9")
	}
}

func TestSourcePins(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "CONTRACT.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"`GIT_COMMIT=" + MultigresCommit + "`",
		"`COMMIT_DATE=" + MultigresCommitDate + "`",
		"| Multigres source | `" + MultigresCommit + "` |",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("CONTRACT.md lacks %s", want)
		}
	}
}
