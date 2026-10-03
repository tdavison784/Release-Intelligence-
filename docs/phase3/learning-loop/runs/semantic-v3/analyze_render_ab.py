#!/usr/bin/env python3
"""Controlled with/without rendered-evidence comparison (run4 vs run3).

Compares proposals for the SAME candidate ids by the SAME models, once proposed
with release-level rendered evidence (run3, `semantic-full/v1+rendered`, in the
committed knowledge/ tree) and once without (run4, plain `semantic-full/v1`, in
the run4 -out store). Reports, per model: proposal counts, aspects asserted,
abstentions, and the paired per-aspect flip table (asserted in both / only with
render / only without / neither). Also counts rendered-evidence citations on the
with-render side (evidence ids whose candidate record carries Render.scope=release).

Usage: python3 analyze_render_ab.py <with-render-store> <without-render-store>
"""

import json
import sys
from collections import Counter, defaultdict
from pathlib import Path

ASPECTS = ["subject", "change", "applicability", "consequence"]


def load(store):
    """proposals by (candidateId, model); render-evidence ids per candidate."""
    props = {}
    render_ev = {}
    for f in Path(store).rglob("proposals/*.json"):
        d = json.loads(f.read_text())
        p = d.get("proposal", d)
        key = (p["candidateId"], p.get("provenance", {}).get("model", "?"))
        props[key] = p
    for f in Path(store).rglob("candidates/*.json"):
        d = json.loads(f.read_text())
        c = d.get("candidate", d)
        rids = {e["id"] for e in (c.get("evidence") or [])
                if isinstance(e.get("render"), dict) and e["render"].get("scope") == "release"}
        if rids:
            render_ev[c["id"]] = rids
    return props, render_ev


def asserted(p, aspect):
    a = p.get("assertion", {})
    if aspect == "applicability":
        return bool(a.get("applicability"))
    if aspect == "consequence":
        return bool(a.get("consequence"))
    return bool(a.get(aspect))


def main(with_store, without_store):
    wp, wrev = load(with_store)
    nop, _ = load(without_store)
    wkeys = {k for k, p in wp.items() if "rendered" in p.get("provenance", {}).get("promptVersion", "")}
    models = sorted({m for _, m in wkeys | set(nop)})
    print(f"with-render proposals (v1+rendered): {len(wkeys)}; without (run4): {len(nop)}")
    for m in models:
        mk = [k for k in wkeys if k[1] == m]
        nk = [k for k in nop if k[1] == m]
        pairs = sorted(set(mk) & set(nk))
        print(f"\n== {m}: with {len(mk)}, without {len(nk)}, paired {len(pairs)} ==")
        flips = Counter()
        cite_render = 0
        for cid, mm in pairs:
            for asp in ASPECTS:
                flips[(asp, asserted(wp[(cid, mm)], asp), asserted(nop[(cid, mm)], asp))] += 1
            if wrev.get(cid, set()) & set(wp[(cid, mm)].get("citations", [])):
                cite_render += 1
        hdr = f"{'aspect':14s} {'both':>5s} {'only-with':>9s} {'only-without':>12s} {'neither':>7s}"
        print(hdr)
        for asp in ASPECTS:
            b = flips[(asp, True, True)]
            ow = flips[(asp, True, False)]
            on = flips[(asp, False, True)]
            n = flips[(asp, False, False)]
            print(f"{asp:14s} {b:5d} {ow:9d} {on:12d} {n:7d}")
        und_w = sum(1 for c, _ in pairs if wp[(c, m)].get("undetermined"))
        und_n = sum(1 for c, _ in pairs if nop[(c, m)].get("undetermined"))
        print(f"full abstention (undetermined non-empty): with {und_w}, without {und_n}")
        print(f"with-render proposals citing rendered evidence: {cite_render}/{len(pairs)}")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(2)
    main(sys.argv[1], sys.argv[2])
