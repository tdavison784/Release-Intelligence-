import json, glob, os, sys, collections
root = sys.argv[1]            # run dir containing same-model/
sm = os.path.join(root, "same-model")
K = os.path.join(sm, "knowledge")
callset = {}                  # callId -> 1|2
for i in (1, 2):
    for f in glob.glob(f"{sm}/ex-{i}/*.response.json"):
        callset[json.load(open(f))["callId"]] = f"call-{i}"
cands, props = {}, []
for f in glob.glob(K + "/*/*/candidates/*.json"):
    r = json.load(open(f)); cands[r["candidate"]["id"]] = r["candidate"]
for f in glob.glob(K + "/*/*/proposals/*.json"):
    props.append(json.load(open(f))["proposal"])
fails = [json.load(open(f)) for f in glob.glob(K + "/*/failures/*.json")]
bycs = collections.Counter(callset.get(p["provenance"]["callId"], "?") for p in props)
print(f"same-model store: {len(cands)} candidates, {len(props)} proposals, {len(fails)} failures; proposals per call-set {dict(bycs)}")
unmapped = sum(1 for p in props if p["provenance"]["callId"] not in callset)
print(f"proposals whose callId maps to a call-set: {len(props)-unmapped}/{len(props)}")

A = ["subject", "change", "applicability", "consequence"]
def digest(p, a):
    v = p["assertion"].get(a)
    if v is None: return None
    if a == "subject": return json.dumps({k: v[k] for k in sorted(v) if k != "product"})
    if a == "consequence": return json.dumps([v["kind"], v.get("severity", "")])
    return json.dumps(v, sort_keys=True)
byc = collections.defaultdict(dict)
for p in props: byc[p["candidateId"]][callset[p["provenance"]["callId"]]] = p
shared = [d for d in byc.values() if "call-1" in d and "call-2" in d]
print(f"\nsonnet call-1 vs sonnet call-2 over {len(shared)} shared candidates (same model, separate calls):")
for a in A:
    both = agree = 0
    for d in shared:
        x, y = digest(d["call-1"], a), digest(d["call-2"], a)
        if x and y: both += 1; agree += x == y
    print(f"  {a:13s} {agree}/{both}" + (f"  ({100*agree/max(1,both):.0f}%)" if both else ""))
bothabst = sum(1 for d in shared if all(not any(d[c]["assertion"].get(a) for a in A) for c in ("call-1", "call-2")))
o1 = sum(1 for d in shared if any(d["call-1"]["assertion"].get(a) for a in A) and not any(d["call-2"]["assertion"].get(a) for a in A))
o2 = sum(1 for d in shared if any(d["call-2"]["assertion"].get(a) for a in A) and not any(d["call-1"]["assertion"].get(a) for a in A))
print(f"  both fully abstained {bothabst}; only call-1 asserts {o1}; only call-2 asserts {o2}")
ar1 = sum(1 for d in shared if d["call-1"].get("suggestedClass") == "action-required")
ar2 = sum(1 for d in shared if d["call-2"].get("suggestedClass") == "action-required")
arb = sum(1 for d in shared if d["call-1"].get("suggestedClass") == "action-required" and d["call-2"].get("suggestedClass") == "action-required")
print(f"  action-required requests: call-1 {ar1}, call-2 {ar2}, both {arb}")
# separate-call check at the domain level: no proposal id appears in both call-sets
ids1 = {p["id"] for p in props if callset.get(p["provenance"]["callId"]) == "call-1"}
ids2 = {p["id"] for p in props if callset.get(p["provenance"]["callId"]) == "call-2"}
print(f"  proposal-id overlap between call-sets: {len(ids1 & ids2)} (0 = every proposal is its own call)")
