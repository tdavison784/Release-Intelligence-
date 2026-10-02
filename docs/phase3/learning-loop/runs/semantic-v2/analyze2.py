import json, glob, os, sys, collections
root = sys.argv[1]
K = os.path.join(root, "knowledge")
cands, props = {}, []
for f in glob.glob(K + "/*/*/candidates/*.json"):
    r = json.load(open(f)); cands[r["candidate"]["id"]] = r["candidate"]
for f in glob.glob(K + "/*/*/proposals/*.json"):
    props.append(json.load(open(f))["proposal"])
fails = [json.load(open(f)) for f in glob.glob(K + "/*/failures/*.json")]
print(f"store: {len(cands)} unique candidates, {len(props)} proposals, {len(fails)} recorded failures")

# per model per edge
byme = collections.defaultdict(lambda: collections.Counter())
for p in props:
    byme[p["provenance"]["model"]][p["candidateId"].split("_")[0] if False else p["candidateId"]] += 1
edges = collections.defaultdict(lambda: collections.Counter())
for p in props:
    c = cands.get(p["candidateId"], {})
    edges[(c.get("product", "?"), c.get("to", c.get("release", "?")))][p["provenance"]["model"]] += 1
print("\nproposals per edge per model:")
for e in sorted(edges): print("  ", e, dict(edges[e]))

A = ["subject", "change", "applicability", "consequence"]
def digest(p, a):
    v = p["assertion"].get(a)
    if v is None: return None
    if a == "subject": return json.dumps({k: v[k] for k in sorted(v) if k != "product"})
    if a == "consequence": return json.dumps([v["kind"], v.get("severity", "")])
    return json.dumps(v, sort_keys=True)

byc = collections.defaultdict(dict)
for p in props: byc[p["candidateId"]][p["provenance"]["model"]] = p
models = sorted(byme)
print("\npairwise aspect agreement (both models asserted the aspect on the same candidate):")
for i in range(len(models)):
    for j in range(i + 1, len(models)):
        m1, m2 = models[i], models[j]
        shared = [d for d in byc.values() if m1 in d and m2 in d]
        if not shared: continue
        print(f"\n{m1} vs {m2} over {len(shared)} shared candidates")
        for a in A:
            both = agree = 0
            for d in shared:
                x, y = digest(d[m1], a), digest(d[m2], a)
                if x and y: both += 1; agree += x == y
            print(f"  {a:13s} {agree}/{both}" + (f"  ({100*agree/max(1,both):.0f}%)" if both else ""))
        bothabst = sum(1 for d in shared if all(not any(d[m]["assertion"].get(a) for a in A) for m in (m1, m2)))
        only1 = sum(1 for d in shared if any(d[m1]["assertion"].get(a) for a in A) and not any(d[m2]["assertion"].get(a) for a in A))
        only2 = sum(1 for d in shared if any(d[m2]["assertion"].get(a) for a in A) and not any(d[m1]["assertion"].get(a) for a in A))
        print(f"  both fully abstained {bothabst}; only {m1.split('-')[1]} asserts {only1}; only {m2.split('-')[1]} asserts {only2}")
        ar1 = sum(1 for d in shared if d[m1].get("suggestedClass") == "action-required")
        ar2 = sum(1 for d in shared if d[m2].get("suggestedClass") == "action-required")
        arb = [cid for cid, d in byc.items() if m1 in d and m2 in d and d[m1].get("suggestedClass") == "action-required" and d[m2].get("suggestedClass") == "action-required"]
        print(f"  action-required requests: {m1.split('-')[1]} {ar1}, {m2.split('-')[1]} {ar2}, both {len(arb)}")

# abstention and class distribution per model over shared 4 edges vs own edges already in analyze.py
