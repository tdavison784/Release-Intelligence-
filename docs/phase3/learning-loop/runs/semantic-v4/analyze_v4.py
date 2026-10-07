"""semantic-4 outcome: what the v2 re-proposal did on the property-selected candidates.

    python3 analyze_v4.py <knowledge-dir> <selection-only-dir> <failures-dir>

Old = proposals with prompt version semantic-*/v1*; new = semantic-*/v2*. Per rule (R1 fact leaf,
R1b proposal leaf, R2 all-abstained, R3 new L4 candidate) and per model: proposals, refusals
(incl. avoidable-undecidable refusals), undecidable leaves by reason old vs new, how many
candidates lost every undecidable leaf (replaced by decidable predicates), how many previously
all-abstained candidates now carry an assertion.
"""
import json, glob, os, sys, collections
K, ONLY, FD = sys.argv[1], sys.argv[2], sys.argv[3]
A = ["subject", "change", "applicability", "consequence"]
rule_of = {}
for f in glob.glob(os.path.join(ONLY, "*.txt")):
    r = os.path.basename(f)[:-4].split("__")[-1]
    for l in open(f):
        if l.strip() and not l.startswith("#"):
            rule_of[l.strip()] = r

def leaves(c, out):
    if not c:
        return
    if c.get("op") == "undecidable":
        out.append(c.get("reason"))
    for s in c.get("of") or []:
        leaves(s, out)

def und(p):
    out = []
    ap = (p.get("assertion") or {}).get("applicability") or {}
    leaves(ap.get("exposure"), out); leaves(ap.get("overlap"), out)
    return out

def decidable_leaves(p):
    cnt = [0]
    def walk(c):
        if not c: return
        if not c.get("of") and c.get("op") not in ("undecidable",):
            cnt[0] += 1
        for s in c.get("of") or []: walk(s)
    ap = (p.get("assertion") or {}).get("applicability") or {}
    walk(ap.get("exposure")); walk(ap.get("overlap"))
    return cnt[0]

old, new = collections.defaultdict(list), collections.defaultdict(list)
for f in glob.glob(K + "/*/*/proposals/*.json"):
    p = json.load(open(f))["proposal"]
    if p["candidateId"] not in rule_of:
        continue
    (new if "/v2" in p["provenance"]["promptVersion"] else old)[p["candidateId"]].append(p)
fails = [json.load(open(f)) for f in glob.glob(FD + "/*/failures/*.json")]
fails = [f for f in fails if f["CandidateID"] in rule_of]

print(f"selected {len(rule_of)}; with v2 proposals {len(new)}; v2 proposals {sum(len(v) for v in new.values())}; refusals {len(fails)}")
fk = collections.Counter((f["Model"], "avoidable-undecidable" if "avoidable undecidable" in f["Reason"] else f["Reason"].split(":")[0]) for f in fails)
print("refusals by model/kind:", dict(sorted(fk.items())))
for rule in ["R1", "R1b", "R2", "R3"]:
    cids = [c for c, r in rule_of.items() if r == rule]
    o_leaf = collections.Counter(x for c in cids for p in old[c] for x in und(p))
    n_leaf = collections.Counter(x for c in cids for p in new[c] for x in und(p))
    had = [c for c in cids if any(und(p) for p in old[c])]
    cleared = [c for c in had if new[c] and not any(und(p) for p in new[c])]
    cleared_dec = [c for c in cleared if any(decidable_leaves(p) for p in new[c])]
    asserting = [c for c in cids if any(any((p["assertion"] or {}).get(a) for a in A) for p in new[c])]
    print(f"\n{rule}: {len(cids)} candidates, {sum(len(new[c]) for c in cids)} v2 proposals; candidates where a v2 model asserts something: {len(asserting)}")
    print(f"  undecidable leaves old {dict(o_leaf)} (total {sum(o_leaf.values())}) -> new {dict(n_leaf)} (total {sum(n_leaf.values())})")
    if had:
        print(f"  candidates that had an undecidable leaf: {len(had)}; every v2 proposal free of undecidable: {len(cleared)} "
              f"(of which with a decidable predicate instead: {len(cleared_dec)})")
    for m in sorted({p['provenance']['model'] for c in cids for p in new[c]}):
        ps = [p for c in cids for p in new[c] if p["provenance"]["model"] == m]
        ab = sum(1 for p in ps if not any((p["assertion"] or {}).get(a) for a in A))
        ul = sum(len(und(p)) for p in ps)
        app = sum(1 for p in ps if (p["assertion"] or {}).get("applicability"))
        print(f"    {m}: {len(ps)} proposals, full abstention {ab}, applicability asserted {app}, undecidable leaves {ul}")
