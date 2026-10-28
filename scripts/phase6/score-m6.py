#!/usr/bin/env python3
"""Score the Milestone 6 blind runs per docs/phase6/decision-intel/GATE-M6.md (and amendments A1-A4).

    python3 -I scripts/phase6/score-m6.py <repo-root>

Reads each run's stored score.json.gz (the harness's own decisions, agent verdicts and judgements) and each case's
expected.yaml (the decisive item). Prints a Markdown report. It applies only the pre-registered rules; judgement
calls the gate leaves to a human (P3 consumers, P4 "names the missing input", P5 advisory flagging, the D grading)
are listed for review rather than decided here.
"""
import glob
import gzip
import json
import os
import sys

import yaml

ROOT = sys.argv[1]
DIR = os.path.join(ROOT, "eval/investigations-blind6")
ARMS = {
    "A": ["M6-A2"],
    "B": ["M6-B2-r1", "M6-B2-r2", "M6-B2-r3"],
    "C/D": ["M6-CD2-r1", "M6-CD2-r2", "M6-CD2b-r1"],  # A4: M6-CD2-r3 void
    "E (A + KE)": ["M6-E-A"],
}
RANK = {"not-affected": 0, "insufficient-evidence": 1, "potentially-affected": 2, "affected": 3}


def cases():
    out = {}
    for p in sorted(glob.glob(os.path.join(DIR, "*--*", "expected.yaml"))):
        e = yaml.safe_load(open(p))
        items = e.get("items") or []
        dec = e.get("decisive")
        if isinstance(dec, dict):
            dec = dec.get("item")
        if not dec:
            flagged = [i["item"] for i in items if i.get("decisive")]
            dec = flagged[0] if flagged else (items[0]["item"] if len(items) == 1 else None)
        exp = next((i for i in items if i["item"] == dec), None)
        out[e["case"]] = {"item": dec, "expected": exp["classification"] if exp else None}
    return out


def load(run):
    p = os.path.join(DIR, "results", run, "score.json.gz")
    return json.load(gzip.open(p)) if os.path.exists(p) else None


def outcome(agent, expected):
    """GATE-M6 'Agent assessment outcome'."""
    if not agent:
        return "no-verdict"
    if agent == expected:
        return "correct"
    if expected == "insufficient-evidence" and agent == "potentially-affected":
        return "correct?"  # correct only if it names the missing input: human check (listed)
    if agent == "not-affected":
        return "false-safe"
    if agent == "affected" and expected == "not-affected":
        return "false-alarm"
    if agent == "affected":
        return "overstated"
    return "hedged"


def main():
    C = cases()
    print("# M6 scores (scripts/phase6/score-m6.py)\n")
    print("| case | decisive | expected |")
    print("|---|---|---|")
    for c, v in C.items():
        print(f"| {c} | {v['item']} | {v['expected']} |")
    summary = {}
    checks = []
    for arm, runs in ARMS.items():
        print(f"\n## Arm {arm}\n")
        print("| run | case | authoritative | agent | outcome | unsafe clearance |")
        print("|---|---|---|---|---|---|")
        per = []
        for run in runs:
            s = load(run)
            if s is None:
                print(f"| {run} | (missing) | | | | |")
                continue
            dec = {(d["case"], d["item"]): d for d in s.get("decisions") or []}
            ver = {(d["case"], d["item"]): d for d in s.get("agentVerdicts") or []}
            n_ok = n_fs = n_unsafe = 0
            for c, v in C.items():
                d = dec.get((c, v["item"])) or {}
                sysc = d.get("system")
                a = ver.get((c, v["item"]), {}).get("agent")
                o = outcome(a, v["expected"]) if arm in ("B", "C/D") else "-"
                unsafe = sysc == "not-affected" and v["expected"] != "not-affected"
                n_ok += o == "correct"
                n_fs += o == "false-safe"
                n_unsafe += unsafe
                if o == "correct?":
                    checks.append((run, c, "P4/insufficient-evidence: does the agent name the missing input?",
                                   ver[(c, v["item"])].get("mechanism", "")))
                print(f"| {run} | {c} | {sysc} | {a or '-'} | {o} | {'**YES**' if unsafe else 'no'} |")
            m = s.get("metrics") or {}
            ue = m.get("unsafeEligibilityRate", {})
            per.append({"run": run, "correct": n_ok, "fs": n_fs, "unsafe": n_unsafe,
                        "unsafeElig": f"{ue.get('num', '?')}/{ue.get('den', '?')}",
                        "judgements": s.get("judgements") or []})
        summary[arm] = per
    print("\n## Summary\n")
    print("| arm | runs | mean correct /8 | mean false-safe | unsafe clearances | unsafeEligibility |")
    print("|---|---|---|---|---|---|")
    for arm, per in summary.items():
        if not per:
            continue
        k = len(per)
        print(f"| {arm} | {k} | {sum(p['correct'] for p in per) / k:.2f} | {sum(p['fs'] for p in per) / k:.2f} | "
              f"{sum(p['unsafe'] for p in per)} | {', '.join(p['unsafeElig'] for p in per)} |")
    print("\n## D judgements (for P5 and the A3 grading)\n")
    print("| run | case | status | recommendation | proposed | chosen | minimum |")
    print("|---|---|---|---|---|---|---|")
    for p in summary.get("C/D", []):
        for j in p["judgements"]:
            print(f"| {p['run']} | {j.get('case')} | {j.get('status')} | {j.get('recommendation')} | "
                  f"{j.get('proposed')} | {j.get('chosen')} | {j.get('minimum')} |")
    if checks:
        print("\n## Human checks\n")
        for run, c, q, mech in checks:
            print(f"- **{run} {c}**: {q} Agent mechanism: {mech}")


main()
