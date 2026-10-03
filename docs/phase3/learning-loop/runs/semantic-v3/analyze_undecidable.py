#!/usr/bin/env python3
"""Undecidable-exposure analysis over the committed knowledge store.

LOOP-DIAGNOSIS-2 found that 72% of UNKNOWN exposure evaluations come from an
`undecidable` leaf the proposal itself wrote, and that `cli-flag` leaves alone
account for 50 of 178 UNKNOWNs. Those leaves are authored by this lane's
proposals, so the fix (channel context in the prompt, L1; vocabulary for
decidable shapes, L5) is ours. This script quantifies the target before any
prompt change: which models, prompt versions, subject families and note texts
produce the undecidable leaves, and what a decidable leaf set looks like today.

Usage: python3 analyze_undecidable.py <knowledge-dir>
"""

import json
import sys
from collections import Counter, defaultdict
from pathlib import Path


def walk_leaves(node):
    """Yield every leaf of a condition tree (op nodes with `of` are branches)."""
    if isinstance(node, dict) and "op" in node:
        if isinstance(node.get("of"), list):
            for child in node["of"]:
                yield from walk_leaves(child)
            return
        yield node
        return
    if isinstance(node, list):
        for child in node:
            yield from walk_leaves(child)


def main(store):
    proposals = []
    for p in Path(store).rglob("proposals/*.json"):
        d = json.loads(p.read_text())
        rec = d.get("proposal", d)
        rec["_path"] = str(p)
        proposals.append(rec)

    with_exposure = [p for p in proposals if p.get("assertion", {}).get("applicability", {}).get("exposure")]
    leaf_ops = Counter()
    und_by_model = Counter()
    und_by_prompt = Counter()
    und_by_family = Counter()
    und_by_task = Counter()
    notes = Counter()
    reasons = Counter()
    per_product = Counter()
    pure_undecidable = 0
    with_und = 0
    examples = defaultdict(list)

    for p in with_exposure:
        exposure = p["assertion"]["applicability"]["exposure"]
        leaves = list(walk_leaves(exposure))
        for leaf in leaves:
            leaf_ops[leaf["op"]] += 1
        und = [l for l in leaves if l["op"] == "undecidable"]
        if not und:
            continue
        with_und += 1
        if all(l["op"] == "undecidable" for l in leaves):
            pure_undecidable += 1
        prov = p.get("provenance", {})
        fam = p["assertion"].get("subject", {}).get("family", "?")
        product = p.get("product") or p["_path"].split("/")[-4]
        und_by_model[prov.get("model", "?")] += len(und)
        und_by_prompt[prov.get("promptVersion", "?")] += len(und)
        und_by_family[fam] += len(und)
        und_by_task[p.get("task", "?")] += len(und)
        per_product[product] += len(und)
        for l in und:
            # `needed` is the free-text what-is-missing; `reason` the enum.
            reasons[str(l.get("reason", ""))] += 1
            note = " ".join(str(l.get("needed", "")).split()) or str(l.get("reason", ""))
            notes[note] += 1
            if len(examples[note]) < 2:
                examples[note].append(f"{p['id']} ({fam}, {prov.get('model','?')})")

    total_und = sum(und_by_model.values())
    print(f"store: {len(proposals)} proposals, {len(with_exposure)} with an exposure condition")
    print(f"proposals with >=1 undecidable leaf: {with_und} "
          f"({with_und / len(with_exposure) * 100:.0f}% of exposure-bearing)")
    print(f"pure-undecidable exposures (no decidable leaf at all): {pure_undecidable}")
    print(f"undecidable leaves total: {total_und}")
    print("\nleaf op census (exposure-bearing proposals):")
    for op, n in leaf_ops.most_common():
        print(f"  {op:24s} {n:5d}")
    print("\nundecidable leaves by model:")
    for k, v in und_by_model.most_common():
        print(f"  {k:24s} {v:5d}")
    print("\nundecidable leaves by prompt version:")
    for k, v in und_by_prompt.most_common():
        print(f"  {k:30s} {v:5d}")
    print("\nundecidable leaves by task:")
    for k, v in und_by_task.most_common():
        print(f"  {k:12s} {v:5d}")
    print("\nundecidable leaves by subject family:")
    for k, v in und_by_family.most_common(12):
        print(f"  {k:28s} {v:5d}")
    print("\nundecidable leaves by product (top 12):")
    for k, v in per_product.most_common(12):
        print(f"  {k:28s} {v:5d}")
    print("\nundecidable reasons (enum):")
    for note, n in reasons.most_common():
        print(f"  {note:32s} {n:5d}")
    print("\nundecidable `needed` texts (top 30, with example proposals):")
    for note, n in notes.most_common(30):
        ex = "; ".join(examples[note])
        print(f"  {n:3d}x {note[:150]}  [{ex}]")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(__doc__)
        sys.exit(2)
    main(sys.argv[1])
