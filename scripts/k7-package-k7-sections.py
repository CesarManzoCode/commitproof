#!/usr/bin/env python3
"""k7-package-k7-sections.py — writes the two K7 sections of a finding
package's README from its finding.json and evidence/K7/results.json
(CONTRACT §11.4, §15 K7.4; ADR-0045): the K7 matrix runs that repeat the
finding's mechanism (each with the fields ADR-0045's predicate matched) and
K7.4's re-execution of the replay command. It replaces the markers
<!-- K7-RECURRENCES --> … and <!-- K7-REPLAY --> … (with their sections, so
it can run again) and touches nothing else.

usage: k7-package-k7-sections.py <findings/<id>> [evidence/K7/results.json]
"""
import glob
import io
import json
import re
import sys
import tarfile
from compression import zstd


def replay_manifest(run_id):
    """The manifest of a K7.4 replay, from its stub in evidence/K7/package-replays."""
    with open(f"evidence/K7/package-replays/{run_id}.stub.tar.zst", "rb") as fh:
        data = zstd.decompress(fh.read())
    with tarfile.open(fileobj=io.BytesIO(data)) as t:
        return json.load(t.extractfile(f"{run_id}/manifest.json"))


def invalid_attempts(minimized):
    """K7.4's INVALID attempts at replaying a minimized run: the summaries in
    evidence/K7/package-replays whose checks.json says INVALID and whose
    manifest replays that run, in run-id order."""
    out = []
    for p in sorted(glob.glob("evidence/K7/package-replays/*.checks.json")):
        c = json.load(open(p, encoding="utf-8"))
        if c.get("verdict") != "INVALID":
            continue
        m = replay_manifest(c["run_id"])
        if m.get("replay_of") == minimized:
            out.append((c["run_id"], m, c))
    return out


def fields_text(g):
    f = g.get("fields") or {}
    parts = [f"classes {', '.join(g['classes'])} ({', '.join(f'{k} {v}' for k, v in sorted(g.get('counts', {}).items()))})"]
    for k in ("valid_C1", "valid_C2", "valid_C6"):
        if k in f:
            parts.append(f"{k.replace('valid_', 'valid(')}) {f[k]}")
    if "timeouts" in f:
        parts.append(f"timeouts {f['timeouts']}")
    if "confirmed_fault_kinds" in f:
        parts.append("F {" + ", ".join(f.get("confirmed_fault_kinds") or []) + "}")
    io = f.get("incompatible_order")
    if io:
        parts.append("vanished: " + "; ".join(
            f"key {x['key']}: {x['absent_from_longest']} read at i={x['read_i']}, absent from i={x['longest_read_i']}" for x in io[:4])
            + (f"; … ({len(io)} listed)" if len(io) > 4 else ""))
    ls = f.get("listener_sessions")
    if ls:
        parts.append("listener sessions: " + "; ".join(f"{c} LISTEN i={i}" for c, i in sorted(ls.items())))
    for k, v in sorted(f.items()):
        if k not in ("valid_C1", "valid_C2", "valid_C6", "timeouts", "confirmed_fault_kinds", "incompatible_order", "listener_sessions"):
            parts.append(f"{k} {json.dumps(v, separators=(',', ':'))[:200]}")
    return "; ".join(parts)


def main():
    pkg = sys.argv[1].rstrip("/")
    res_path = sys.argv[2] if len(sys.argv) > 2 else "evidence/K7/results.json"
    f = json.load(open(f"{pkg}/finding.json", encoding="utf-8"))
    res = json.load(open(res_path, encoding="utf-8"))
    reps = {r["run"]: r for r in res.get("repeats", [])}
    out = ["## Recurrences in K7 (ADR-0045)", ""]
    recs = f.get("recurrences") or []
    preds = sorted({r["predicate"] for r in recs})
    if not recs:
        out.append(f"None. No valid run of K7's matrix (130 cells, `evidence/K7/results.json`) failed with a guarantee group that matches the predicate standing for this finding; nothing of K7 is cited here. Every K7 failure and how ADR-0045's predicates classified it is listed in `evidence/K7/results.json` and the K7 closure.")
    else:
        out.append(f"{len(recs)} valid run(s) of K7's matrix repeat this finding's mechanism: every guarantee group of their anomalies matches ADR-0045's predicate {', '.join(preds)}, which reads only `checks.json`. Each is counted in its cell's k/N and interval (§12.3), triaged (`evidence/K7/triage/`), committed as a summary (stub and `checks.json`, `evidence/K7/runs/`), and not minimized. Its full archive stays on the machine that ran it (`runs/k7/`). `cpgate K7` re-evaluates the predicate for every one.")
        out.append("")
        out.append("| Run | Cell | Seed | Seal root | Predicate | Matched fields |")
        out.append("|---|---|---|---|---|---|")
        for r in sorted(recs, key=lambda x: x["run_id"]):
            rr = reps.get(r["run_id"], {})
            gs = [g for g in (rr.get("classification") or {}).get("groups", []) if g.get("predicate") == r["predicate"]]
            ft = fields_text(gs[0]).replace("|", "\\|") if gs else "—"
            out.append(f"| `{r['run_id']}` | {r['cell']} {rr.get('campaign', '')}/{rr.get('profile', '')} | {r['seed']} | `{r['seal_root'][:16]}…` | {r['predicate']} | {ft} |")
    rec_text = "\n".join(out)

    k = f.get("k7_replay") or {}
    out = ["## K7 replay (K7.4)", ""]
    if not k.get("record"):
        out.append("Not yet executed.")
    else:
        cl = f["classification"]
        ms = [replay_manifest(r["run_id"]) for r in k["runs"]]
        commits = sorted({m["commitproof_commit"][:7] for m in ms})
        versions = sorted({m["contract_version"] for m in ms})
        out.append(f"The package's replay command was executed once more in K7 (record `{k['record']}` in `evidence/K7/commands.jsonl`; "
                   f"CommitProof `{', '.join(commits)}`, contract {', '.join(versions)} (ADR-0047), with ADR-0038's metrics exporter on every Multigres component, LIM-41). "
                   f"The plan replayed is this package's minimized plan, unchanged. Its five replays are stored as summaries in `evidence/K7/package-replays/`:")
        out.append("")
        out.append("| Replay | Verdict | Classes | Seal root |")
        out.append("|---|---|---|---|")
        for r in k["runs"]:
            out.append(f"| `{r['run_id']}` | {r['verdict']} | {', '.join(f'`{c}`' for c in r['classes']) or '—'} | `{r['seal_root'][:16]}…` |")
        out.append("")
        out.append(f"**{k['replays_with_class']} of {len(k['runs'])}** show `{f['anomaly_class']}` (§11.1 on these replays: {k['reproducibility']}). "
                   f"The recorded classification stays {cl['reproducibility']} ({f['replays_with_class']} of 5 recorded replays): this re-execution is reported beside it, not instead of it.")
        inv = invalid_attempts(f["minimized_run"]["run_id"])
        if inv:
            out.append("")
            out.append(f"{len(inv)} INVALID attempt(s) of these replays were re-run with the same plan (§12.1) and count toward nothing; "
                       "each is stored as a summary beside the replays, and its cause is in `evidence/K7/NOTES.md` §8:")
            out.append("")
            out.append("| Attempt | Replay | Verdict | Seal root |")
            out.append("|---|---|---|---|")
            for run_id, m, c in inv:
                out.append(f"| `{run_id}` | {m.get('replay_index')} | {c['verdict']} | `{c['seal_root'][:16]}…` |")
    rep_text = "\n".join(out)

    p = f"{pkg}/README.md"
    s = open(p, encoding="utf-8").read()
    for marker, text in (("K7-RECURRENCES", rec_text), ("K7-REPLAY", rep_text)):
        pat = re.compile(r"<!-- " + marker + r" -->.*?<!-- /" + marker + r" -->", re.S)
        block = f"<!-- {marker} -->\n{text}\n<!-- /{marker} -->"
        if pat.search(s):
            s = pat.sub(lambda _: block, s)
        elif f"<!-- {marker} -->" in s:
            s = s.replace(f"<!-- {marker} -->", block, 1)
        else:
            sys.exit(f"{p}: no {marker} marker")
    open(p, "w", encoding="utf-8").write(s)
    print(json.dumps({"package": pkg, "recurrences": len(recs), "k7_replay": k.get("record", "")}))


if __name__ == "__main__":
    main()
