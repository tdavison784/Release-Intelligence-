import json, glob, os, sys, collections, itertools
root = sys.argv[1]
K = os.path.join(root, "knowledge")
cands, props = {}, []
for f in glob.glob(K + "/*/*/candidates/*.json"):
    r = json.load(open(f)); cands[r["candidate"]["id"]] = r["candidate"]
for f in glob.glob(K + "/*/*/proposals/*.json"):
    props.append(json.load(open(f))["proposal"])
fails = [json.load(open(f)) for f in glob.glob(K + "/*/failures/*.json")]
pending = 0
ex = root + "/exchange"
reqs = glob.glob(ex + "/*.request.json"); resps = glob.glob(ex + "/*.response.json")
print(f"candidates {len(cands)} (unique over 8 edges); requests {len(reqs)}; answered {len(resps)}; proposals {len(props)}; recorded failures {len(fails)}")
by = collections.defaultdict(list)
for p in props: by[p["provenance"]["model"]].append(p)
A = ["subject","change","applicability","consequence"]
for m, ps in sorted(by.items()):
    asserted = collections.Counter(a for p in ps for a in A if p["assertion"].get(a))
    undet = collections.Counter(a for p in ps for a in p.get("undeterminedReason") and p.get("undetermined", []) or [])
    allabst = sum(1 for p in ps if not any(p["assertion"].get(a) for a in A))
    full = sum(1 for p in ps if all(p["assertion"].get(a) for a in A))
    sc = collections.Counter(p.get("suggestedClass","") or "-" for p in ps)
    ck = collections.Counter(p["assertion"]["consequence"]["kind"] for p in ps if p["assertion"].get("consequence"))
    fam = collections.Counter(p["assertion"]["subject"]["family"] for p in ps if p["assertion"].get("subject"))
    conf = collections.Counter(p["provenance"]["confidence"] for p in ps)
    cites = [len(p.get("citations") or []) for p in ps if any(p["assertion"].get(a) for a in A)]
    print(f"\n## {m}: {len(ps)} proposals; all four asserted {full}; full abstention {allabst}")
    print("  asserted", dict(asserted)); print("  undetermined", dict(undet))
    print("  suggestedClass", dict(sc)); print("  consequence kinds", dict(ck)); print("  subject families", dict(fam)); print("  confidence", dict(conf))
    print(f"  citations per asserting proposal: mean {sum(cites)/max(1,len(cites)):.2f}")
fk = collections.Counter((f["Model"], f["Reason"].split(":")[0]) for f in fails)
print("\nfailures by model/kind", dict(fk))
rej = collections.Counter()
for f in fails:
    r = f["Reason"]
    for key in ["not part of this family", "only valid inside", "before and after are required", "migration-required and subject family", "requirement-changed", "schema", "action-eligible", "cites", "replacedBy", "statement", "undetermined"]:
        if key in r: rej[(f["Model"], key)] += 1; break
    else: rej[(f["Model"], "other: " + r[:120])] += 1
for k, v in sorted(rej.items()): print("  ", k, v)
# Pairwise aspect agreement lives in analyze2.py (any number of models, any coverage:
# opus ran only 4 of the 8 edges, so a fixed models[:2] pair over all candidates crashes).
def digest(p, a):
    v = p["assertion"].get(a)
    if v is None: return None
    if a == "subject": return json.dumps({k: v[k] for k in sorted(v) if k != "product"})
    if a == "consequence": return json.dumps([v["kind"], v.get("severity","")])
    return json.dumps(v, sort_keys=True)
byc = collections.defaultdict(dict)
for p in props: byc[p["candidateId"]][p["provenance"]["model"]] = p
# action requests
ar = [(p["provenance"]["model"], cands[p["candidateId"]]["title"][:100]) for p in props if p.get("suggestedClass") == "action-required"]
print(f"\naction-required requests: {len(ar)}")
pairs = list(itertools.combinations(sorted(by), 2))
for m1, m2 in pairs:
    shared = [d for d in byc.values() if m1 in d and m2 in d]
    both_ar = [c for c, d in byc.items() if m1 in d and m2 in d and d[m1].get("suggestedClass") == "action-required" and d[m2].get("suggestedClass") == "action-required"]
    print(f"  {m1} + {m2}: both request action-required on {len(both_ar)} of {len(shared)} shared candidates")
    for c in both_ar[:40]: print("   -", cands[c]["product"], cands[c]["title"][:110])
