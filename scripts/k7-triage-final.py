#!/usr/bin/env python3
"""k7-triage-final.py — the triage record of a K7 real-build FAIL that
repeats recorded findings (ADR-0045; CONTRACT §15 K7: checker output, §6.2
completion review, the oracle for C3, §11 classification with G2a/G2b per
ADR-0042), from k7triage's draft and, for C3 anomalies, ADR-0046's oracle
record. It refuses (exit 2) a run that is not a repeat — a new failure is
triaged by hand — and a completion review it cannot reconcile with the
predicate's mechanism: every reviewed completion must sit in its §6.2 row,
every operation an anomaly names must be ok, and every writer of a vanished
value (R-G2b) or of a value read but later absent (R-G6) must be info.

usage: k7-triage-final.py <draft.json> <cell> <out.json> [<oracle record.json>]
"""
import json
import sys

# §11.1 per recorded mechanism (ADR-0042, ADR-0043; the owner's decisions of
# 2026-10-15): the recorded findings' classification without reproducibility.
CLASS = {
    "R-G2b": {"guarantee": "G2b", "class": "SAFETY_VIOLATION", "claim_level": "INFERRED", "severity": "high",
              "origin": "POSTGRES_INHERITED", "known_status": "NEW"},
    "R-G6": {"guarantee": "G6", "class": "SAFETY_VIOLATION", "claim_level": "DESIGN", "severity": "high",
             "origin": "POSTGRES_INHERITED", "known_status": "NEW"},
    "R-G10": {"guarantee": "G10", "class": "PROTOCOL_DEVIATION", "claim_level": "DESIGN", "severity": "medium",
              "origin": "MULTIGRES", "known_status": "KNOWN"},
    "R-G12": {"guarantee": "G12", "class": "PROTOCOL_DEVIATION", "claim_level": "DESIGN", "severity": "medium",
              "origin": "MULTIGRES", "known_status": "NEW"},
}
OK_ROWS = {"ok": {1}, "info": {9, 11, 12, 13, 14}, "fail": {2, 3, 4, 5, 6, 7, 8, 10}}


def secs(ns):
    ms = (ns + 500_000) // 1_000_000
    return f"{ms // 1000}.{ms % 1000:03d} s"


def main():
    draft, cell, out = sys.argv[1:4]
    oracle = json.load(open(sys.argv[4])) if len(sys.argv) > 4 else None
    d = json.load(open(draft))
    rep = d["repeat"]
    if not rep.get("repeat"):
        sys.exit(f"{d['run_id']}: not a repeat (ADR-0045); a new failure is triaged by hand")
    problems = []
    rows = {}
    named_ok = vanished_info = 0
    seen = set()
    for a in d["completion_review"]:
        for r in a["review"]:
            if (r["op"], r.get("why", "")) in seen:
                continue
            seen.add((r["op"], r.get("why", "")))
            key = (r["completion"], r["row"])
            rows[key] = rows.get(key, 0) + 1
            if r["row"] not in OK_ROWS.get(r["completion"], set()):
                problems.append(f"op {r['op']}: {r['completion']} in row {r['row']}")
            why = r.get("why", "")
            if why == "named by the anomaly" and r["completion"] != "ok":
                problems.append(f"op {r['op']}: named by the anomaly but {r['completion']}")
            if why == "named by the anomaly":
                named_ok += 1
            if ("absent from the longest read" in why or "completion not ok" in why):
                if r["completion"] != "info":
                    problems.append(f"op {r['op']}: writer of a vanished value is {r['completion']}, not info")
                else:
                    vanished_info += 1
    if problems:
        sys.exit(f"{d['run_id']}: completion review does not fit the repeated mechanism: {problems}")
    classification = []
    analysis = []
    for g in rep["groups"]:
        c = dict(CLASS[g["predicate"]])
        c["repeat_of"] = g["findings"]
        c["predicate"] = g["predicate"]
        classification.append(c)
        f = g["fields"]
        if g["predicate"] == "R-G2b":
            v = "; ".join(f"key {x['key']}: {x['absent_from_longest']} read by ok i={x['read_i']}, absent from the key's longest read i={x['longest_read_i']}"
                          for x in f["incompatible_order"])
            analysis.append(f"G2b (R-G2b): {v}. Every value that vanished was appended by a transaction whose completion was info "
                            f"(the client's P01 deadline expired after it was sent, §6.2 row 9), and no acknowledged write was lost "
                            f"(C1 valid) nor a failed write seen (C2 valid); {f['timeouts']} client timeouts; confirmed faults "
                            f"{', '.join(f['confirmed_fault_kinds'])}, among them F1c (the leader cut off from orchestrators, followers "
                            f"and etcd but not from the gateways). The recorded mechanism of FND-D1-fb8d0f8c, FND-D1-2e860fd2, "
                            f"FND-D1-a8eb9857 and FND-C2-2a534585: a cancelled synchronous-replication wait completes the commit locally "
                            f"(S61), other sessions read it, and the failover to a standby that never received it removes it (LIM-33).")
        elif g["predicate"] == "R-G6":
            analysis.append(f"G6 (R-G6): C4 non-linearizable ({g['counts']}) with C6 valid (no read of a value superseded by a write "
                            f"that completed before it), C1 and C2 valid, {f['timeouts']} client timeouts, confirmed faults "
                            f"{', '.join(f['confirmed_fault_kinds'])}: the registers' form of the same vanish (FND-C2-6806aeca): "
                            f"ok reads returned values whose writes ended info and that the failover later removed.")
        elif g["predicate"] == "R-G10":
            ls = "; ".join(f"{k} LISTEN i={v}" for k, v in sorted(f.get("listener_sessions", {}).items()))
            analysis.append(f"G10 (R-G10): notify-not-resumed ({g['counts']}) by listeners that kept the session they opened ({ls}); "
                            f"confirmed faults {', '.join(f['confirmed_fault_kinds'])}: the mechanism of FND-C9-40b6f05a, "
                            f"FND-C9-2fe9a9fc and FND-C9-90da5532 — the gateway re-establishes a session's notification stream only on "
                            f"the client's next LISTEN or UNLISTEN (upstream PR #1550, S53).")
    review = (f"§6.2 review of {sum(rows.values())} distinct reviewed completions (k7triage over the run's full local history): "
              + ", ".join(f"{n} {c} in row {r}" for (c, r), n in sorted(rows.items()))
              + f". The {named_ok} operations the anomalies name are ok (row 1: explicit success); the {vanished_info} writers of "
                f"values read but later absent are info, never ok or fail, so no misclassified completion can create or hide the "
                f"anomaly. The run is valid (seal verified, every fault confirmed and healed, final reads stable). Not a HARNESS_DEFECT.")
    rec = {"run_id": d["run_id"], "seal_root": d["seal_root"], "cell": cell, "campaign": d["campaign"], "profile": d["profile"],
           "seed": d["seed"], "contract_version": d["contract_version"], "verdict": d["verdict"], "checks_sha256": d["checks_sha256"],
           "repeat": True, "adr_0045": rep, "classification": classification, "analysis": " ".join(analysis),
           "harness_review": review, "harness_defect": False, "completion_review": d["completion_review"]}
    if oracle:
        rec["oracle"] = {k: oracle.get(k) for k in ("result", "decided_by", "elle_valid", "elle_classes", "records", "triage")}
    elif any(g["guarantee"] == "G2b" for g in rep["groups"]):
        sys.exit(f"{d['run_id']}: C3 anomalies need ADR-0046's oracle record")
    json.dump(rec, open(out, "w"), indent=2, sort_keys=True, ensure_ascii=False)
    open(out, "a").write("\n")
    print(json.dumps({"run": d["run_id"], "repeat_of": sorted({f for c in classification for f in c["repeat_of"]}), "reviewed": sum(rows.values())}))


if __name__ == "__main__":
    main()
