#!/usr/bin/env python3
# k7-cycle-keys.py <run-dir> <elle-cli validity.json> — K7.3 triage (ADR-0046): keys whose ok W1 reads are not
# all prefixes of the key's longest read (C3's incompatible-order rule,
# §10.3), and the keys of every step of elle-cli's G0 / G-single cycles.
import json, sys
hist = sys.argv[1] + "/history.jsonl"
inv = {}
reads = {}
for line in open(hist):
    e = json.loads(line)
    if e.get("wl") != "append" or e.get("f") != "txn":
        continue
    if e["type"] == "ok":
        for m in e["value"]:
            if m[0] == "r" and m[2] is not None:
                reads.setdefault(m[1], []).append(m[2])
bad = set()
for k, rs in reads.items():
    longest = max(rs, key=len)
    for r in rs:
        if longest[:len(r)] != r:
            bad.add(k)
v = json.load(open(sys.argv[2]))
out = {}
for typ, cycles in v.get("anomalies", {}).items():
    if typ in ("incompatible-order",):
        continue
    ks = []
    for c in cycles:
        ks.append(sorted({s["key"] for s in c.get("steps", []) if "key" in s}))
    out[typ] = ks
allk = sorted({k for ks in out.values() for kk in ks for k in kk})
print(json.dumps({"incompatible_keys": sorted(bad), "cycle_keys": out, "cycle_keys_not_incompatible": [k for k in allk if k not in bad]}))
