#!/usr/bin/env python3
"""Merge the real knowledge store and the proxy shadow store into an evaluation view.

    scripts/proxy-shadow-merge.py [--base-git=<ref>] <base-manifest> <real-dir> <shadow-dir> <out-dir>

The shadow store is a copy of the real store taken at one moment (the base, recorded as a
manifest of `sha256  relative/path` lines), in which the proxy decided the high-priority items
the product owner reviews in the real store. The view is a per-file three-way merge:

  shadow unchanged since the base  -> real
  real unchanged since the base    -> shadow (the shadow's decisions, items, facts)
  both changed the same way        -> that
  both changed differently         -> REAL wins (human work is never overwritten); the path is
                                      listed as a conflict
  present only in real or shadow   -> taken (a deletion is never propagated: stores are append-only)

With --base-git=<ref> (contract-6), a conflict is first tried as a FIELD-LEVEL three-way merge of the
record's payload: the base content is recovered as `git show <ref>:<real-dir>/<path>` and used only
if its sha256 equals the manifest's base digest. When real and shadow changed DIFFERENT top-level
fields of the payload (e.g. real re-derived `consensusAction` under PO-5 while the shadow's proxy
decision set `status: superseded`), both changes are kept and the path is listed as merged-fields.
Anything else (the same field changed differently, no recoverable base) stays REAL-WINS.

The output is labelled `proxy-incl-shadow` (LABEL.md at the top, which the file store ignores). It
is for evaluation at the proxy level only. It is never the real store, and nothing in it is human-verified
beyond what the real store already holds.
"""
import hashlib
import json
import os
import shutil
import subprocess
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


PAYLOADS = ("candidate", "proposal", "validation", "reviewItem", "decision", "fact")


def field_merge(base_ref, real_dir, rel, base_digest, real_path, shadow_path):
    """Field-level three-way merge of one knowledge record, or None."""
    try:
        blob = subprocess.run(["git", "show", f"{base_ref}:{os.path.join(real_dir, rel)}"],
                              check=True, capture_output=True).stdout
    except subprocess.CalledProcessError:
        return None
    if hashlib.sha256(blob).hexdigest() != base_digest:
        return None
    b = json.loads(blob)
    with open(real_path) as f:
        r = json.load(f)
    with open(shadow_path) as f:
        s = json.load(f)
    if {k: v for k, v in b.items() if k not in PAYLOADS} != {k: v for k, v in r.items() if k not in PAYLOADS} or \
            {k: v for k, v in b.items() if k not in PAYLOADS} != {k: v for k, v in s.items() if k not in PAYLOADS}:
        return None
    key = next((k for k in PAYLOADS if k in b), None)
    if key is None or key not in r or key not in s:
        return None
    bp, rp, sp = b[key], r[key], s[key]
    out = dict(bp)
    for k in set(bp) | set(rp) | set(sp):
        rc, sc = rp.get(k) != bp.get(k), sp.get(k) != bp.get(k)
        if rc and sc and rp.get(k) != sp.get(k):
            return None  # the same field changed differently: not mergeable
        src = rp if rc else sp if sc else bp
        if k in src:
            out[k] = src[k]
        else:
            out.pop(k, None)
    merged = dict(b)
    merged[key] = out
    return merged


def main():
    args = [a for a in sys.argv[1:] if not a.startswith("--base-git=")]
    base_ref = next((a.split("=", 1)[1] for a in sys.argv[1:] if a.startswith("--base-git=")), None)
    if len(args) != 4:
        sys.exit(__doc__)
    manifest, real, shadow, out = args
    base = {}
    with open(manifest) as f:
        for line in f:
            digest, rel = line.rstrip("\n").split("  ", 1)
            base[rel] = digest
    r, s = tree(real), tree(shadow)
    if os.path.exists(out):
        shutil.rmtree(out)
    stats = {"real": 0, "shadow": 0, "same": 0, "conflict-real-wins": 0, "real-only": 0, "shadow-only": 0}
    if base_ref:
        stats["merged-fields"] = 0
    conflicts = []
    merged_fields = []
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
                m = field_merge(base_ref, real, rel, hb, r[rel], s[rel]) if base_ref and hb else None
                if m is not None:
                    stats["merged-fields"] += 1
                    merged_fields.append(rel)
                    dst = os.path.join(out, rel)
                    os.makedirs(os.path.dirname(dst), exist_ok=True)
                    with open(dst, "w") as f:
                        json.dump(m, f, indent=2, ensure_ascii=False)
                        f.write("\n")
                    continue
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
                "Built by `scripts/proxy-shadow-merge.py` (three-way, per file; the real store wins every conflict"
                + (f"; with --base-git={base_ref}, conflicts touching different fields are merged field-wise" if base_ref else "")
                + ").\n\n"
                "```json\n" + json.dumps({"stats": stats, "conflicts": conflicts, "mergedFields": merged_fields}, indent=2) + "\n```\n")
    print(json.dumps(stats), f"{len(conflicts)} conflicts, {len(merged_fields)} merged field-wise")
    for c in conflicts:
        print("conflict (real kept):", c)
    for c in merged_fields:
        print("merged field-wise:", c)


if __name__ == "__main__":
    main()
