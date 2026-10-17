#!/usr/bin/env python3
"""k7-liveness.py — what a run that ended INVALID with "recovery deadline
P06 exceeded" shows (CONTRACT §11.1 LIVENESS_OBSERVATION, §12.1 INVALID):
its faults, the runner's recovery attempts per gateway and their outcomes,
and every node's pooler status (role, decided rule, leader, cohort, WAL
position) from the last fault's heal to the end, summarized as the first and
last sample and whether the WAL position moved. Reads the sealed run's
history.jsonl and observations.jsonl only (observations explain, never
judge, ADR-0016). One JSON object on stdout.

usage: k7-liveness.py <run-dir>
"""
import json
import sys


def lines(p):
    with open(p, encoding="utf-8") as f:
        for l in f:
            yield json.loads(l)


def s(ns):
    return round(ns / 1e9, 3) if ns is not None else None


def main():
    d = sys.argv[1].rstrip("/")
    man = json.load(open(f"{d}/manifest.json"))
    faults, markers, rec = [], {}, {}
    for e in lines(f"{d}/history.jsonl"):
        if e["type"] == "nemesis":
            fl = e["fault"]
            faults.append({"t": s(e["t"]), "id": fl.get("id"), "kind": fl.get("kind"), "phase": fl.get("phase"), "targets": fl.get("targets")})
        elif e["type"] == "marker":
            markers[e["value"].get("name")] = s(e["t"])
        elif e.get("client") in (10, 11, 12) and e.get("wl") == "ledger" and e.get("f") == "insert" and e["type"] != "invoke":
            g = rec.setdefault(e["gw"], {"ok": 0, "info": 0, "fail": 0, "errors": {}})
            g[e["type"]] = g.get(e["type"], 0) + 1
            if e.get("err"):
                k = f"{e['err'].get('phase')}/{e['err'].get('sqlstate') or e['err'].get('class')}"
                g["errors"][k] = g["errors"].get(k, 0) + 1
    healed = max((f["t"] for f in faults if f["phase"] == "healed"), default=None)
    nodes = {}
    for o in lines(f"{d}/observations.jsonl"):
        v = o.get("value") or {}
        if v.get("src") != "pooler-status" or healed is None or o["t"] / 1e9 < healed:
            continue
        n = nodes.setdefault(v["node"], {"samples": 0, "failed": 0, "first": None, "last": None})
        n["samples"] += 1
        if not v.get("ok"):
            n["failed"] += 1
            continue
        dec = v.get("decision") or {}
        smp = {"t": s(o["t"]), "role": v.get("pooler_type"), "postgres": v.get("postgres_status"),
               "rule": f"{dec.get('coordinator_term')}.{dec.get('leader_subterm')}", "leader": dec.get("leader"),
               "cohort": dec.get("cohort"), "wal": v.get("wal_position")}
        if n["first"] is None:
            n["first"] = smp
        n["last"] = smp
    for n in nodes.values():
        n["wal_moved"] = bool(n["first"] and n["last"] and n["first"]["wal"] != n["last"]["wal"])
    print(json.dumps({"run_id": man["run_id"], "campaign": man["campaign"], "seed": man["seed"], "plan_sha256": man["plan_sha256"],
                      "commitproof_commit": man["commitproof_commit"], "faults": faults, "last_healed_s": healed,
                      "markers": {k: markers.get(k) for k in ("workload-end", "recovery-begin", "recovery-end", "run-end")},
                      "recovery_attempts": rec, "nodes_after_last_heal": nodes}, sort_keys=True))


if __name__ == "__main__":
    main()
