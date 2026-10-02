"""Cross-model vs same-model agreement over a knowledge store (semantic real run).

    python3 analyze_cross.py <knowledge-dir> [<same-model-store>] [--products a,b,c]

Pairs are labelled with the domain's consensus scope: cross-model when the model families differ
(domain.ModelFamily: leading name token, "glm-5.3" -> glm, "claude-opus-5-5" -> claude), else
same-model. Agreement = equal aspect digests over candidates where both answers assert the aspect
(subject: identity without product; consequence: kind + severity; others: full value).
"""
import json, glob, sys, collections, itertools, re

A = ["subject", "change", "applicability", "consequence"]
FREE = {"protocol-behavior", "migration", "api-endpoint", "product-relationship", "compatibility-boundary"}

def family(model):
    m = model.lower().split("/")[-1]
    return re.split(r"[-_.:0-9]", m, 1)[0]

PRODUCTS = None
if "--products" in sys.argv:
    i = sys.argv.index("--products")
    PRODUCTS = set(sys.argv[i + 1].split(","))
    del sys.argv[i:i + 2]

def load(K):
    props = [json.load(open(f))["proposal"] for f in glob.glob(K + "/*/*/proposals/*.json")
             if PRODUCTS is None or f[len(K):].strip("/").split("/")[0] in PRODUCTS]
    by = collections.defaultdict(lambda: collections.defaultdict(list))
    for p in props:
        by[p["candidateId"]][p["provenance"]["model"]].append(p)
    return props, by

def digest(p, a):
    v = p["assertion"].get(a)
    if v is None:
        return None
    if a == "subject":
        return json.dumps({k: v[k] for k in sorted(v) if k != "product"})
    if a == "consequence":
        return json.dumps([v["kind"], v.get("severity", "")])
    return json.dumps(v, sort_keys=True)

def pairs_of(by, x, y):
    """(px, py) per candidate both models answered (first proposal of each)."""
    for c, d in by.items():
        if x in d and y in d:
            yield c, d[x][0], d[y][0]

def table(by, x, y, restrict=None):
    out = {}
    for a in A:
        both = agree = 0
        for c, px, py in pairs_of(by, x, y):
            if restrict and c not in restrict:
                continue
            u, v = digest(px, a), digest(py, a)
            if u and v:
                both += 1
                agree += u == v
        out[a] = (agree, both)
    return out

def fmt(t):
    return "  ".join(f"{a} {g}/{n} ({100*g/n:.0f}%)" if n else f"{a} -/0" for a, (g, n) in t.items())

K = sys.argv[1]
props, by = load(K)
models = sorted({p["provenance"]["model"] for p in props})
glm = [m for m in models if family(m) == "glm"]
subset = {c for c, d in by.items() if any(m in d for m in glm)} if glm else set()
print(f"store {K}: {len(props)} proposals; models {models}")
if glm:
    print(f"comparison subset: {len(subset)} candidates answered by {glm}")
print("\n== per aspect (both asserted), restricted to the GLM subset ==")
for x, y in itertools.combinations(models, 2):
    scope = "cross-model" if family(x) != family(y) else "same-model "
    print(f"{scope} {x} | {y}:  {fmt(table(by, x, y, subset or None))}")

if len(sys.argv) > 2:
    _, sby = load(sys.argv[2])
    # the same-model store: two separate calls of one model per candidate
    both = collections.Counter()
    for c, d in sby.items():
        for m, ps in d.items():
            if len(ps) >= 2:
                for a in A:
                    u, v = digest(ps[0], a), digest(ps[1], a)
                    if u and v:
                        both[(a, "n")] += 1
                        both[(a, "agree")] += u == v
    print("same-model  separate calls of one model (strimzi):  " +
          "  ".join(f"{a} {both[(a,'agree')]}/{both[(a,'n')]}" for a in A))

print("\n== subject agreement by family (both asserted the subject; family = first model's) ==")
for x, y in itertools.combinations(models, 2):
    scope = "cross-model" if family(x) != family(y) else "same-model"
    fam = collections.defaultdict(lambda: [0, 0, 0])  # exact, same family, n
    for c, px, py in pairs_of(by, x, y):
        if subset and c not in subset:
            continue
        sx, sy = px["assertion"].get("subject"), py["assertion"].get("subject")
        if sx and sy:
            f = fam[sx["family"]]
            f[2] += 1
            f[1] += sx["family"] == sy["family"]
            f[0] += digest(px, "subject") == digest(py, "subject")
    if not fam:
        continue
    cls = {"structured": [0, 0, 0], "free-form": [0, 0, 0]}
    for k, (e, s, n) in fam.items():
        c = cls["free-form" if k in FREE else "structured"]
        c[0] += e; c[1] += s; c[2] += n
    print(f"{scope} {x} | {y}: " + "; ".join(f"{k} exact {e}/{n}, same-family {s}/{n}" for k, (e, s, n) in cls.items()))
    print("     " + ", ".join(f"{k} {e}/{n}" for k, (e, s, n) in sorted(fam.items(), key=lambda kv: -kv[1][2])))
