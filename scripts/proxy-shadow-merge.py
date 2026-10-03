#!/usr/bin/env python3
"""Merge the real knowledge store and the proxy shadow store into an evaluation view.

    scripts/proxy-shadow-merge.py <base-manifest> <real-dir> <shadow-dir> <out-dir>

The shadow store is a copy of the real store taken at one moment (the base, recorded as a
manifest of `sha256  relative/path` lines), in which the proxy decided the high-priority items
the product owner reviews in the real store. The view is a per-file three-way merge:

  shadow unchanged since the base  -> real
  real unchanged since the base    -> shadow (the shadow's decisions, items, facts)
  both changed the same way        -> that
  both changed differently         -> REAL wins (human work is never overwritten); the path is
                                      listed as a conflict
  present only in real or shadow   -> taken (a deletion is never propagated: stores are append-only)

The output is labelled `proxy-incl-shadow` (LABEL.md at the top, which the file store ignores). It
is for evaluation at the proxy level only. It is never the real store, and nothing in it is human-verified
beyond what the real store already holds.
"""
import hashlib
import json
import os
import shutil
import sys


def sha(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def tree(root):
    out = {}
    for dirpath, _, files in os.walk(root):
        for name in files:
            if not name.endswith(".json"):
                continue
            p = os.path.join(dirpath, name)
            out[os.path.relpath(p, root)] = p
    return out


def main():
    if len(sys.argv) != 5:
        sys.exit(__doc__)
    manifest, real, shadow, out = sys.argv[1:]
    base = {}
    with open(manifest) as f:
        for line in f:
            digest, rel = line.rstrip("\n").split("  ", 1)
            base[rel] = digest
    r, s = tree(real), tree(shadow)
    if os.path.exists(out):
        shutil.rmtree(out)
    stats = {"real": 0, "shadow": 0, "same": 0, "conflict-real-wins": 0, "real-only": 0, "shadow-only": 0}
    conflicts = []
    for rel in sorted(set(r) | set(s)):
        src = None
        if rel in r and rel not in s:
            src, k = r[rel], "real-only"
        elif rel in s and rel not in r:
            src, k = s[rel], "shadow-only"
        else:
            hr, hs, hb = sha(r[rel]), sha(s[rel]), base.get(rel)
            if hr == hs:
                src, k = r[rel], "same"
            elif hs == hb:
                src, k = r[rel], "real"
            elif hr == hb:
                src, k = s[rel], "shadow"
            else:
                src, k = r[rel], "conflict-real-wins"
                conflicts.append(rel)
        stats[k] += 1
        dst = os.path.join(out, rel)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copyfile(src, dst)
    with open(os.path.join(out, "LABEL.md"), "w") as f:
        f.write("# proxy-incl-shadow: evaluation view, NOT the knowledge store\n\n"
                "The real `knowledge/` store merged with the proxy shadow pass "
                "(`docs/phase3/learning-loop/proxy-shadow/knowledge/`). In the shadow pass a blind AI proxy "
                "(claude-opus-5-5) decided the high-priority review items that the product owner reviews in "
                "the real store. Every decision from the shadow pass is `reviewerKind: proxy`, so the facts it "
                "produced are at most proxy level (capped at REVIEW by the trust ladder). Evaluate this view at "
                "`-min-verification proxy` and report it as **proxy-incl-shadow**, never as human-verified.\n\n"
                "Built by `scripts/proxy-shadow-merge.py` (three-way, per file; the real store wins every conflict).\n\n"
                "```json\n" + json.dumps({"stats": stats, "conflicts": conflicts}, indent=2) + "\n```\n")
    print(json.dumps(stats), f"{len(conflicts)} conflicts")
    for c in conflicts:
        print("conflict (real kept):", c)


if __name__ == "__main__":
    main()
