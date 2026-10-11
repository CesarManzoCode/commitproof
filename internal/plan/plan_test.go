package plan

import (
	"bytes"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

func canon(t *testing.T, seed uint64) []byte {
	t.Helper()
	p, err := Generate("smoke", seed, "baseline", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDeterministic(t *testing.T) {
	a, b := canon(t, 42), canon(t, 42)
	if !bytes.Equal(a, b) {
		t.Fatal("same inputs, different plan bytes")
	}
	if bytes.Equal(a, canon(t, 43)) {
		t.Fatal("seed 43 produced the seed 42 plan")
	}
}

func TestShape(t *testing.T) {
	p, err := Generate("smoke", 7, "baseline", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Clients) != params.ClientsTotal {
		t.Fatalf("%d clients", len(p.Clients))
	}
	want := int(params.SmokeDuration / params.Pacing)
	count := map[string]int{}
	appended := map[int64]map[int64]bool{}
	ledger := map[int64]bool{}
	regVals := map[int64]bool{}
	for _, c := range p.Clients {
		count[c.WL]++
		if len(c.Ops) != want {
			t.Fatalf("client %d has %d ops, want %d", c.Client, len(c.Ops), want)
		}
		if c.GW != Gateways[c.Client%3] {
			t.Fatalf("client %d gw %s", c.Client, c.GW)
		}
		for _, o := range c.Ops {
			switch c.WL {
			case WLAppend:
				mops := o.Value.([]any)
				if len(mops) < 1 || len(mops) > 4 {
					t.Fatalf("txn of %d micro-ops", len(mops))
				}
				for _, m := range mops {
					mo := m.([]any)
					if mo[0] == "append" {
						k, v := mo[1].(int64), mo[2].(int64)
						if appended[k] == nil {
							appended[k] = map[int64]bool{}
						}
						if appended[k][v] {
							t.Fatalf("value %d appended twice to key %d", v, k)
						}
						appended[k][v] = true
					}
				}
			case WLLedger:
				id := o.Value.(map[string]any)["id"].(int64)
				if ledger[id] {
					t.Fatalf("ledger id %d planned twice", id)
				}
				ledger[id] = true
			case WLRegister:
				m := o.Value.(map[string]any)
				for _, f := range []string{"v", "new"} {
					if v, ok := m[f]; ok {
						if regVals[v.(int64)] {
							t.Fatalf("register value %d written twice", v)
						}
						regVals[v.(int64)] = true
					}
				}
			}
		}
	}
	if count[WLAppend] != params.ClientsAppend || count[WLRegister] != params.ClientsRegister ||
		count[WLBank] != params.ClientsBank || count[WLLedger] != params.ClientsLedger {
		t.Fatalf("workload mix %v", count)
	}
	for k, vs := range appended {
		if len(vs) > params.AppendKeyRetireAt {
			t.Fatalf("key %d planned %d appends > %d", k, len(vs), params.AppendKeyRetireAt)
		}
	}
	if len(appended) <= params.AppendActiveKeys {
		t.Fatalf("no key rotation: %d keys", len(appended))
	}
}

func TestRejectsUnknown(t *testing.T) {
	if _, err := Generate("nope", 1, "baseline", ""); err == nil {
		t.Fatal("unknown campaign accepted")
	}
	if _, err := Generate("smoke", 1, "nope", ""); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
