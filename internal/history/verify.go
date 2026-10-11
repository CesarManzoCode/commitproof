package history

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
)

// Event is a decoded history line. Optional integer fields are pointers so
// that "absent" and "0" differ.
type Event struct {
	I       int64
	Prev    string
	T       int64
	Type    string
	Proc    *int64
	Client  *int64
	GW      string
	WL      string
	F       string
	Value   any
	Iso     string
	Err     map[string]any
	Notices []any
	Op      *int64
	Fault   map[string]any
	Raw     []byte
}

// MaxLine bounds a single line (a final read of the whole ledger is the
// largest legitimate line).
const MaxLine = 64 << 20

// ReadChain reads a chained stream and verifies, line by line: canonical
// encoding, i == line number, prev == sha256 of the previous line, t
// non-decreasing and non-negative. It returns the decoded events up to the
// first problem, and the problem (nil if the whole stream verifies).
func ReadChain(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	return readChain(f)
}

func readChain(r io.Reader) ([]Event, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	var evs []Event
	prev := ZeroHash
	var lastT int64
	for n := int64(0); ; n++ {
		line, err := br.ReadBytes('\n')
		if err == io.EOF {
			if len(line) != 0 {
				return evs, fmt.Errorf("line %d: missing trailing newline", n)
			}
			return evs, nil
		}
		if err != nil {
			return evs, fmt.Errorf("line %d: read: %w", n, err)
		}
		line = line[:len(line)-1]
		if len(line) > MaxLine {
			return evs, fmt.Errorf("line %d: longer than %d bytes", n, MaxLine)
		}
		if !canonjson.IsCanonical(line) {
			return evs, fmt.Errorf("line %d: not canonical JSON", n)
		}
		ev, err := DecodeEvent(line)
		if err != nil {
			return evs, fmt.Errorf("line %d: %w", n, err)
		}
		if ev.I != n {
			return evs, fmt.Errorf("line %d: i = %d", n, ev.I)
		}
		if ev.Prev != prev {
			return evs, fmt.Errorf("line %d: prev = %s, want %s (chain broken)", n, ev.Prev, prev)
		}
		if ev.T < 0 || ev.T < lastT {
			return evs, fmt.Errorf("line %d: t = %d decreases (previous %d)", n, ev.T, lastT)
		}
		lastT = ev.T
		sum := sha256.Sum256(line)
		prev = hex.EncodeToString(sum[:])
		evs = append(evs, ev)
	}
}

// DecodeEvent parses one canonical line.
func DecodeEvent(line []byte) (Event, error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return Event{}, fmt.Errorf("decode: %w", err)
	}
	ev := Event{Raw: line}
	var err error
	if ev.I, err = reqInt(m, "i"); err != nil {
		return ev, err
	}
	if ev.T, err = reqInt(m, "t"); err != nil {
		return ev, err
	}
	var ok bool
	if ev.Prev, ok = m["prev"].(string); !ok || len(ev.Prev) != 64 {
		return ev, fmt.Errorf("prev missing or not 64 hex chars")
	}
	if ev.Type, ok = m["type"].(string); !ok {
		return ev, fmt.Errorf("type missing")
	}
	switch ev.Type {
	case TypeInvoke, TypeOK, TypeFail, TypeInfo, TypeNemesis, TypeMarker, TypeObs:
	default:
		return ev, fmt.Errorf("unknown type %q", ev.Type)
	}
	if ev.Proc, err = optInt(m, "proc"); err != nil {
		return ev, err
	}
	if ev.Client, err = optInt(m, "client"); err != nil {
		return ev, err
	}
	if ev.Op, err = optInt(m, "op"); err != nil {
		return ev, err
	}
	ev.GW, _ = m["gw"].(string)
	ev.WL, _ = m["wl"].(string)
	ev.F, _ = m["f"].(string)
	ev.Iso, _ = m["iso"].(string)
	ev.Value = m["value"]
	ev.Err, _ = m["err"].(map[string]any)
	ev.Notices, _ = m["notices"].([]any)
	ev.Fault, _ = m["fault"].(map[string]any)
	return ev, nil
}

func reqInt(m map[string]any, k string) (int64, error) {
	p, err := optInt(m, k)
	if err != nil {
		return 0, err
	}
	if p == nil {
		return 0, fmt.Errorf("%s missing", k)
	}
	return *p, nil
}

func optInt(m map[string]any, k string) (*int64, error) {
	v, ok := m[k]
	if !ok {
		return nil, nil
	}
	n, ok := v.(json.Number)
	if !ok {
		return nil, fmt.Errorf("%s is not a number", k)
	}
	x, err := strconv.ParseInt(string(n), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", k, err)
	}
	return &x, nil
}

// Workloads of §6.1 (field wl).
var Workloads = map[string]bool{
	"append": true, "register": true, "bank": true, "ledger": true, "session": true,
	"cdc": true, "notify": true, "probe": true, "overload": true,
}

// WellFormed checks the Jepsen history discipline of §6.1 on a verified
// chain: every invoke has exactly one completion (ok/fail/info) whose op is
// the invoke's i and whose proc matches; completions follow their invoke; a
// process has at most one outstanding op; a process that completed with info
// never invokes again; wl values are in the closed set. It returns every
// problem found (empty = well formed).
func WellFormed(evs []Event) []string {
	var probs []string
	add := func(f string, a ...any) {
		if len(probs) < 50 {
			probs = append(probs, fmt.Sprintf(f, a...))
		}
	}
	invokes := map[int64]Event{}
	completed := map[int64]int64{}   // invoke i -> completion i
	outstanding := map[int64]int64{} // proc -> invoke i
	retired := map[int64]bool{}
	for _, ev := range evs {
		switch ev.Type {
		case TypeInvoke:
			if ev.Proc == nil || ev.Client == nil || ev.WL == "" || ev.F == "" || ev.GW == "" {
				add("i=%d: invoke lacks proc/client/wl/f/gw", ev.I)
				continue
			}
			if ev.Op != nil {
				add("i=%d: invoke carries op", ev.I)
			}
			if !Workloads[ev.WL] {
				add("i=%d: unknown wl %q", ev.I, ev.WL)
			}
			p := *ev.Proc
			if retired[p] {
				add("i=%d: proc %d invokes after an info completion", ev.I, p)
			}
			if prev, busy := outstanding[p]; busy {
				add("i=%d: proc %d invokes while op %d is outstanding", ev.I, p, prev)
			}
			outstanding[p] = ev.I
			invokes[ev.I] = ev
		case TypeOK, TypeFail, TypeInfo:
			if ev.Op == nil || ev.Proc == nil {
				add("i=%d: completion lacks op/proc", ev.I)
				continue
			}
			inv, ok := invokes[*ev.Op]
			if !ok {
				add("i=%d: completion of op %d, which is not an earlier invoke", ev.I, *ev.Op)
				continue
			}
			if c, dup := completed[*ev.Op]; dup {
				add("i=%d: second completion of op %d (first at i=%d)", ev.I, *ev.Op, c)
				continue
			}
			completed[*ev.Op] = ev.I
			if *inv.Proc != *ev.Proc {
				add("i=%d: completion proc %d != invoke proc %d", ev.I, *ev.Proc, *inv.Proc)
			}
			if ev.WL != inv.WL || ev.F != inv.F {
				add("i=%d: completion wl/f %s/%s != invoke %s/%s", ev.I, ev.WL, ev.F, inv.WL, inv.F)
			}
			if ev.Type == TypeFail || ev.Type == TypeInfo {
				if ev.Err == nil {
					add("i=%d: %s completion lacks err", ev.I, ev.Type)
				}
			}
			delete(outstanding, *ev.Proc)
			if ev.Type == TypeInfo {
				retired[*ev.Proc] = true
			}
		}
	}
	var unanswered []int64
	for i := range invokes {
		if _, ok := completed[i]; !ok {
			unanswered = append(unanswered, i)
		}
	}
	sort.Slice(unanswered, func(a, b int) bool { return unanswered[a] < unanswered[b] })
	for _, i := range unanswered {
		add("invoke i=%d has no completion", i)
	}
	return probs
}
