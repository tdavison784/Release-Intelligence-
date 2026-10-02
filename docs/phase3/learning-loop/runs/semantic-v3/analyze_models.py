"""Per-model outcomes for one run's products: proposals, abstention, action requests, refusals,
prompt versions (+rendered), rendered evidence on candidates and in citations.

    python3 analyze_models.py <knowledge-dir> <failures-dir> --products a,b,c
"""
import json, glob, sys, collections
K, FD = sys.argv[1], sys.argv[2]
P = set(sys.argv[sys.argv.index("--products") + 1].split(","))
A = ["subject", "change", "applicability", "consequence"]
cands = {}
for f in glob.glob(K + "/*/*/candidates/*.json"):
    c = json.load(open(f))["candidate"]
    if c["product"] in P:
        cands[c["id"]] = c
props = [json.load(open(f))["proposal"] for f in glob.glob(K + "/*/*/proposals/*.json")]
props = [p for p in props if p["candidateId"] in cands]
fails = [json.load(open(f)) for f in glob.glob(FD + "/*/failures/*.json")]
fails = [f for f in fails if f["CandidateID"] in cands]
rend = {c: {e["id"] for e in cands[c]["evidence"] if e.get("render")} for c in cands}
print(f"candidates {len(cands)}; with rendered evidence {sum(1 for v in rend.values() if v)}; per product "
      + str(dict(collections.Counter(c['product'] for c in cands.values()))))
by = collections.defaultdict(list)
for p in props:
    by[(p["provider"], p["provenance"]["model"])].append(p)
fby = collections.Counter((f["Provider"], f["Model"]) for f in fails)
for k, ps in sorted(by.items()):
    abst = sum(1 for p in ps if not any(p["assertion"].get(a) for a in A))
    full = sum(1 for p in ps if all(p["assertion"].get(a) for a in A))
    ar = sum(1 for p in ps if p.get("suggestedClass") == "action-required")
    pv = collections.Counter(p["provenance"]["promptVersion"] for p in ps)
    citesr = sum(1 for p in ps if set(p.get("citations") or []) & rend[p["candidateId"]])
    onr = sum(1 for p in ps if rend[p["candidateId"]])
    print(f"{k[0]}/{k[1]}: {len(ps)} proposals, {fby[k]} refusals; full abstention {abst}; all four {full}; "
          f"action requests {ar}; prompt versions {dict(pv)}; cites rendered evidence {citesr}/{onr} on render candidates")
