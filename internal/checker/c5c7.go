package checker

import (
	"fmt"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// CheckBank is C5 (CONTRACT §10.5): every ok read-all and every ok bank final
// read must sum to accounts × initial balance (P22, from the plan's setup)
// with no negative balance (bank-sum, bank-negative).
func CheckBank(h *History) Result {
	c := newCollector()
	total := h.BankAccounts * h.BankInitial
	var checked int64
	for _, o := range h.Ops {
		if o.WL != "bank" || o.Type != history.TypeOK || (o.F != "read-all" && o.F != "final-read") {
			continue
		}
		rows, ok := o.Value.([]any)
		if !ok {
			c.add("bank-sum", []int64{o.Index}, "%s i=%d returned no account list", o.F, o.Index)
			continue
		}
		checked++
		var sum int64
		var neg []string
		for _, r := range rows {
			row, _ := r.([]any)
			if len(row) != 2 {
				continue
			}
			id, _ := Int(row[0])
			bal, _ := Int(row[1])
			sum += bal
			if bal < 0 {
				neg = append(neg, fmt.Sprintf("account %d = %d", id, bal))
			}
		}
		if sum != total && !sabotageBankSum {
			c.add("bank-sum", []int64{o.Index}, "%s i=%d sums to %d, not %d (%d rows)", o.F, o.Index, sum, total, len(rows))
		}
		if len(neg) > 0 {
			c.add("bank-negative", []int64{o.Index}, "%s i=%d has a negative balance: %s", o.F, o.Index, strings.Join(neg, ", "))
		}
	}
	return c.result("C5", "", map[string]any{"reads_checked": checked, "expected_total": total})
}

// C7 CDC (CONTRACT §10.7) reads the W7 consumer's events (wl "cdc"):
//
//	connect    ok value {conn, slot, start_lsn}
//	recv       ok value {conn, table, row, lsn, xid, flushed}: one received
//	           change; table "cp.ledger" (row {id}) or "cp.append" (row {k,
//	           vals}, the appended element is the last of vals); lsn is the
//	           change's commit LSN, flushed the consumer's confirmed-flush LSN
//	           when the change was received
//	ack        ok value {conn, flush}: standby status update sent
//	caught-up  ok value {conn, target_lsn, received_lsn}: everything committed
//	           up to target_lsn (the server's WAL position after the final
//	           reads) has been received
//
// A change is identified by its table and appended element (ledger id, or
// append (k, v)). The final state is the stable final read through the
// first gateway in sorted order.
//
//   - cdc-gap: an ok write present in the final state never received, once
//     the consumer caught up to the final LSN;
//   - cdc-phantom: a received change absent from the final state;
//   - cdc-duplicate: a change received again with commit LSN ≤ the confirmed
//     flush at that second receipt (the server re-sent a change the consumer
//     had confirmed; later duplicates are at-least-once and allowed);
//   - cdc-order: within one connection, changes of one key received out of
//     commit-LSN order.
type cdcChange struct {
	table string
	k, v  int64 // ledger: k = v = id
}

func (c cdcChange) String() string {
	if c.table == "cp.ledger" {
		return fmt.Sprintf("cp.ledger id %d", c.v)
	}
	return fmt.Sprintf("cp.append key %d element %d", c.k, c.v)
}

func cdcRecv(o *Op) (cdcChange, int64, int64, int64, bool) {
	m := obj(o.Value)
	conn, ok1 := Int(m["conn"])
	lsn, ok2 := Int(m["lsn"])
	flushed, ok3 := Int(m["flushed"])
	row := obj(m["row"])
	table := str(m, "table")
	if !(ok1 && ok2 && ok3) {
		return cdcChange{}, 0, 0, 0, false
	}
	switch table {
	case "cp.ledger":
		id, ok := Int(row["id"])
		return cdcChange{table, id, id}, conn, lsn, flushed, ok
	case "cp.append":
		k, ok := Int(row["k"])
		vals, ok2 := IntList(row["vals"])
		if !ok || !ok2 || len(vals) == 0 {
			return cdcChange{}, 0, 0, 0, false
		}
		return cdcChange{table, k, vals[len(vals)-1]}, conn, lsn, flushed, true
	}
	return cdcChange{}, 0, 0, 0, false
}

// CheckCDC is C7.
func CheckCDC(h *History) Result {
	c := newCollector()
	cdc := h.opsOf("cdc")
	stats := map[string]any{"cdc_ops": int64(len(cdc))}
	if len(cdc) == 0 {
		return c.result("C7", "", stats)
	}
	stable, missing := h.StableFinal()
	gws := h.Gateways()
	if len(missing) > 0 || len(gws) == 0 {
		return c.result("C7", "no stable final read for "+strings.Join(missing, ", "), stats)
	}
	final := stable[gws[0]]
	fa, fl := appendFinal(final["append"].Value), ledgerFinal(final["ledger"].Value)
	present := func(ch cdcChange) bool {
		if ch.table == "cp.ledger" {
			return fl[ch.v]
		}
		return fa[ch.k][ch.v]
	}
	type delivery struct {
		op           *Op
		conn         int64
		lsn, flushed int64
	}
	first := map[cdcChange][]delivery{}
	lastLSN := map[[2]int64]map[string]delivery{} // (conn) → key → last delivery
	caughtUp := false
	var recvs int64
	for _, o := range cdc {
		if o.Type != history.TypeOK {
			continue
		}
		switch o.F {
		case "caught-up":
			caughtUp = true
		case "recv":
			ch, conn, lsn, flushed, ok := cdcRecv(o)
			if !ok {
				return c.result("C7", fmt.Sprintf("malformed recv i=%d", o.Index), stats)
			}
			recvs++
			d := delivery{o, conn, lsn, flushed}
			if !present(ch) && !sabotageCDCPhantom {
				c.add("cdc-phantom", []int64{o.Index}, "received change %s (lsn %d) is absent from the final state", ch, lsn)
			}
			if prev := first[ch]; len(prev) > 0 && cdcDuplicateViolates(lsn, flushed) {
				c.add("cdc-duplicate", []int64{prev[0].op.Index, o.Index}, "change %s (lsn %d) received again at i=%d with confirmed flush %d ≥ its lsn (first received at i=%d)", ch, lsn, o.Index, flushed, prev[0].op.Index)
			}
			first[ch] = append(first[ch], d)
			ck := [2]int64{conn, 0}
			if lastLSN[ck] == nil {
				lastLSN[ck] = map[string]delivery{}
			}
			key := fmt.Sprintf("%s/%d", ch.table, ch.k)
			if p, ok := lastLSN[ck][key]; ok && lsn < p.lsn {
				c.add("cdc-order", []int64{p.op.Index, o.Index}, "connection %d received %s with lsn %d after lsn %d of the same key", conn, ch, lsn, p.lsn)
			}
			lastLSN[ck][key] = d
		}
	}
	stats["changes_received"] = recvs
	// cdc-gap needs the consumer to have caught up.
	var okWrites []cdcChange
	var okOps []*Op
	for _, o := range h.Ops {
		if o.Type != history.TypeOK || o.F == "final-read" {
			continue
		}
		switch o.WL {
		case "ledger":
			if id, ok := ledgerID(o); ok {
				okWrites = append(okWrites, cdcChange{"cp.ledger", id, id})
				okOps = append(okOps, o)
			}
		case "append":
			for _, a := range appendsOf(o) {
				okWrites = append(okWrites, cdcChange{"cp.append", a.K, a.V})
				okOps = append(okOps, o)
			}
		}
	}
	if !caughtUp {
		return c.result("C7", "the consumer never caught up to the final LSN; gaps cannot be decided", stats)
	}
	for i, ch := range okWrites {
		if present(ch) && len(first[ch]) == 0 {
			c.add("cdc-gap", []int64{okOps[i].Index}, "ok change %s is in the final state but was never received", ch)
		}
	}
	return c.result("C7", "", stats)
}

// cdcDuplicateViolates is the duplicate bound: a re-delivery is a violation
// iff its commit LSN ≤ the consumer's confirmed flush at that receipt.
func cdcDuplicateViolates(lsn, flushed int64) bool {
	if sabotageCDCDuplicateBound {
		return lsn < flushed
	}
	return lsn <= flushed
}
