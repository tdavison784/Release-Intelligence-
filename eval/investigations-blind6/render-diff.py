#!/usr/bin/env python3
"""Ground-truth tooling (never read by a system under test).

render-diff.py FROM.yaml TO.yaml OUT.diff
    Canonical, per-object unified diff of two multi-document renders of the same environment (the
    release rendered with the environment's values at `from` and at `to`). Objects are keyed by
    kind/namespace/name, dumped with sorted keys, and compared object by object, so a hunk always
    belongs to exactly one object. Prints a hunk index (line range of OUT.diff, object, +/- counts).
"""
import difflib
import sys

import yaml


def objects(path):
    out = {}
    with open(path) as f:
        for doc in yaml.safe_load_all(f):
            if not doc:
                continue
            md = doc.get("metadata") or {}
            key = f'{doc.get("kind")}/{md.get("namespace", "-")}/{md.get("name")}'
            out[key] = yaml.safe_dump(doc, sort_keys=True, width=100000).splitlines(keepends=True)
    return out


def main(a, b, out_path):
    old, new = objects(a), objects(b)
    lines, index = [], []
    for key in sorted(set(old) | set(new)):
        o, n = old.get(key, []), new.get(key, [])
        if o == n:
            continue
        status = "added" if not o else "removed" if not n else "changed"
        body = list(difflib.unified_diff(o, n, f"from/{key}", f"to/{key}", n=3))
        start = len(lines) + 1
        lines.append(f"# {key} ({status})\n")
        lines.extend(body)
        hunk_start = None
        plus = minus = 0
        for i, l in enumerate(body):
            if l.startswith("@@"):
                if hunk_start is not None:
                    index.append((hunk_start, start + i, key, plus, minus))
                hunk_start, plus, minus = start + 1 + i, 0, 0
            elif l.startswith("+") and not l.startswith("+++"):
                plus += 1
            elif l.startswith("-") and not l.startswith("---"):
                minus += 1
        if hunk_start is not None:
            index.append((hunk_start, start + len(body), key, plus, minus))
    with open(out_path, "w") as f:
        f.writelines(lines)
    for s, e, key, p, m in index:
        print(f"L{s}-L{e}\t{key}\t+{p} -{m}")


if __name__ == "__main__":
    main(*sys.argv[1:4])
