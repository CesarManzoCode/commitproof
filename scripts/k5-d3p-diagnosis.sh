#!/usr/bin/env bash
# k5-d3p-diagnosis.sh — what D3′'s kills did (ADR-0033; the observations
# evidence/K5/predictions-D3p.md names). Diagnosis only (ADR-0016): never a
# verdict. For every primary D3′ run (a run directory under evidence/K5/runs,
# else its archive there, else its full archive in runs/k5-full):
#   - W1/W4 ok latency while both F7s hold and outside every fault window;
#   - per F2a: the W1/W4 operations in flight at the kill (invoked before it,
#     completed after it, or invoked within 100 ms after it) by kind,
#     completion, SQLSTATE and whether the element or row is in the final
#     reads;
#   - the C2 g1a / C3 anomalies cpcheck reported (checks.json);
#   - multiorch's first appointment after the first kill (recruitment and
#     promotion lines) and the killed leader pooler's restart lines;
#   - each gateway's buffering cycles (start, drain); a cycle that ends by
#     "max failover duration" before the workload is bootstrap's (ADR-0029).
# usage: k5-d3p-diagnosis.sh [<runs-dir>] [<full-dir>]
set -euo pipefail
shopt -s nullglob
ev=${1:-evidence/K5/runs} full=${2:-runs/k5-full}
tmp=$(mktemp -d runs/.k5-d3p-diag-XXXXXX)
trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
declare -A ids=()
for x in "$ev"/*-D3p-* "$full"/*-D3p-*; do
	id=$(basename "$x")
	id=${id%.checks.json} id=${id%.minimize.json} id=${id%.stub.tar.zst} id=${id%.tar.zst}
	ids[$id]=1
done
n=0
for id in $(printf '%s\n' "${!ids[@]}" | sort); do
	if [ -d "$ev/$id" ]; then
		d=$ev/$id
	else
		a=$ev/$id.tar.zst
		[ -f "$a" ] || a=$full/$id.tar.zst
		[ -f "$a" ] || continue
		zstd -dc "$a" | tar -C "$tmp" -xf -
		d=$tmp/$id
	fi
	grep -q '"replay_of"\|"candidate_of"' "$d/manifest.json" && continue
	echo "== $id"
	python3 -I - "$d" "$ev/$id.checks.json" <<'EOF'
import json, os, sys, collections
d, checks = sys.argv[1], sys.argv[2]
ev = [json.loads(l) for l in open(os.path.join(d, "history.jsonl"))]
start = [e["t"] for e in ev if e["type"] == "marker" and e["value"].get("name") == "workload-start"][0]
s = lambda t: round((t - start) / 1e9, 3)
faults = {}
for e in ev:
    if e["type"] == "nemesis":
        f = e["fault"]
        x = faults.setdefault(f["id"], {"kind": f["kind"], "targets": f.get("targets"), "t": {}})
        x["t"].setdefault(f["phase"], e["t"])
led, app = set(), collections.defaultdict(set)
for e in ev:
    if e["type"] == "ok" and e.get("f") == "final-read":
        if e["wl"] == "ledger":
            led.update(e["value"])
        elif e["wl"] == "append":
            for k, vals in e["value"]:
                app[int(k)].update(vals)
inv, ops = {}, []
for e in ev:
    if e["type"] == "invoke":
        inv[e["i"]] = e
    elif e["type"] in ("ok", "fail", "info") and e.get("op") in inv and inv[e["op"]].get("f") in ("insert", "append") and inv[e["op"]].get("client", 99) < 10:
        ops.append((inv[e["op"]], e))
def present(i):
    if i["f"] == "insert":
        return (i["value"] or {}).get("id") in led
    _, k, el = i["value"][0]
    return el in app.get(int(k), set())
wins = sorted((x["t"].get("inject"), x["t"].get("healed")) for x in faults.values())
kills = sorted((x["t"]["inject"], k, x["targets"]) for k, x in faults.items() if x["kind"] == "F2a")
f7 = sorted((x["t"]["confirmed"], x["t"].get("heal")) for x in faults.values() if x["kind"] == "F7")
def lat(sel):
    v = sorted((c["t"] - i["t"]) / 1e6 for i, c in ops if c["type"] == "ok" and sel(i["t"]))
    return {"n": len(v), "p50_ms": round(v[len(v) // 2], 1), "p95_ms": round(v[int(len(v) * 0.95)], 1)} if v else {"n": 0}
both = []
for k, (t, _, _) in enumerate(kills):
    c = [x for x in f7 if x[0] <= t]
    if len(c) >= 2:
        both.append((max(y[0] for y in c[-2:]), t))
print(json.dumps({"ok_latency_both_F7_holding": lat(lambda t: any(a <= t < b for a, b in both)),
                  "ok_latency_outside_faults": lat(lambda t: not any(a is not None and a <= t <= (b or a) for a, b in wins))}, sort_keys=True))
for t, fid, tg in kills:
    cnt = collections.Counter()
    for i, c in ops:
        if (i["t"] < t and c["t"] > t) or (t <= i["t"] < t + 100_000_000):
            err = c.get("err") or {}
            cnt[(i["f"], c["type"], err.get("sqlstate", ""), "present" if present(i) else "absent", "sent-before" if i["t"] < t else "sent-after")] += 1
    print(json.dumps({"kill": fid, "leader": tg, "at_s": s(t), "in_flight": [{"f": k[0], "completion": k[1], "sqlstate": k[2], "final": k[3], "sent": k[4], "n": v} for k, v in sorted(cnt.items())]}, sort_keys=True))
if os.path.exists(checks):
    cj = json.load(open(checks))
    print(json.dumps({"verdict": cj.get("verdict"), "anomalies": [{"checker": c["id"], "class": a.get("class"), "ops": a.get("ops") or a.get("op_indices") or a.get("indices"), "note": (a.get("explanation") or a.get("detail") or "")[:240]} for c in cj["checkers"] for a in (c.get("anomalies") or []) if c["id"] in ("C2", "C3")]}, sort_keys=True))
EOF
	# The first appointment after the first kill, and the killed leader's
	# pooler restarting PostgreSQL.
	first=$(python3 -I -c 'import json,sys
for l in open(sys.argv[1]):
    e=json.loads(l)
    if e["type"]=="nemesis" and e["fault"]["kind"]=="F2a" and e["fault"]["phase"]=="inject":
        print(e["fault"]["targets"][0]); break' "$d/history.jsonl")
	{
		for f in orch-z1 orch-z2 orch-z3; do
			{ grep -hE '"msg":"(recruit failed|recruited pooler|promote succeeded|primary.promotion)"' "$d/logs/$f.log" || true; } | sed "s/^/$f /"
		done
		{ grep -hE '"msg":"(MonitorPostgres: (PostgreSQL initialized but not running, starting PostgreSQL|Postgres started successfully|rule names us leader but postgres is a standby; resigning|stale primary[^"]*)|SetPrimary: stale primary, restarting as standby)"' "$d/logs/pooler-$first.log" || true; } | sed "s/^/pooler-$first /"
	} | awk '{print $2, $0}' | sort -k1,1 | cut -d' ' -f2- |
		awk '/LeaderUnhealthy/ && /"outcome":"started"/ {go=1} go {print} /"outcome":"success"/ && go {exit}' |
		sed -E 's/^([a-z0-9-]+) ([0-9T:.-]+)Z \{.*"msg":"([^"]*)"(.*)$/  \2 \1: \3 \4/' |
		sed -E 's/,"(service_instance_id|cloud_availability_zone|service_version|multigres_[a-z]+|trace_id|span_id|pooler_name)":"[^"]*"//g' | cut -c1-240 || true
	# Every buffering cycle of each gateway.
	for g in gw-z1 gw-z2 gw-z3; do
		{ grep -hE '"msg":"(failover detected, starting buffering|stopping buffering, draining entries|max failover duration exceeded, stopping buffering|skipping buffering[^"]*)"' "$d/logs/$g.log" || true; } |
			sed -E "s/^([0-9T:.-]+)Z .*\"msg\":\"([^\"]*)\".*/  \1 $g: \2/"
	done
	[ -d "$tmp/$id" ] && chmod -R u+w "$tmp/$id" && rm -rf "${tmp:?}/$id"
	n=$((n + 1))
done
[ "$n" -eq 10 ] || { echo "k5-d3p-diagnosis: $n primary D3′ runs, expected 10" >&2; exit 2; }
