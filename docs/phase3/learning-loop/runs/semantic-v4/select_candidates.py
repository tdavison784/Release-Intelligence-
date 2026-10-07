"""semantic-4 property selection for the v2 re-proposal (never by eval link).

    python3 select_candidates.py <repo-root> <out-dir>

Rules (each candidate is assigned to the first edge, in EDGES order, that generates it):
  R1  an existing VERIFIED FACT of the candidate has an `undecidable` leaf (exposure or overlap)
  R1b an existing PROPOSAL of the candidate has an `undecidable` leaf (and R1 does not hold)
  R2  the candidate has proposals and every one of them fully abstained (no aspect asserted)
  R3  a NEW candidate (not in the store) produced by L4: a member is a `lines:*` diff or a routine note
Writes <out-dir>/only/<product>__<from>__<to>__<rule>.txt (ids for `ri semantic propose -only`) and
prints counts per rule, per product, plus the leaf reasons.
"""
import json, glob, os, subprocess, sys, collections

ROOT, OUT = sys.argv[1], sys.argv[2]
STATE = "/Users/tommydavison/repos/Release-Intelligence-/.ri"
EDGES = [("argo-cd", "v2.14.5", "v3.0.0"), ("cert-manager", "v1.16.0", "v1.17.0"), ("cert-manager", "v1.17.0", "v1.18.0"),
         ("cilium", "v1.15.6", "v1.17.0"), ("cilium", "v1.16.1", "v1.17.0"), ("istio", "1.23.4", "1.24.0"),
         ("karpenter", "v0.37.8", "v1.0.0"), ("strimzi", "0.45.0", "0.46.0"), ("crossplane", "v1.20.1", "v2.0.0"),
         ("external-secrets", "v0.15.0", "v0.16.0"), ("flux", "v2.6.4", "v2.7.0"), ("kyverno", "v1.12.6", "v1.13.0"),
         ("loki", "v2.9.6", "v3.0.0"), ("prometheus-operator", "v0.85.0", "v0.86.2"), ("traefik", "v2.11.2", "v3.0.0")]
K = os.path.join(ROOT, "knowledge")
ri = os.path.join(ROOT, "bin", "ri")

def run(*args):
    return json.loads(subprocess.run([ri, "-offline", "-state", STATE, *args], capture_output=True, text=True, check=True).stdout)

def leaves(cond, out):
    if not cond:
        return
    if cond.get("op") == "undecidable":
        out.append(cond.get("reason", ""))
    for c in cond.get("of") or []:
        leaves(c, out)

def undecidable(assertion):
    out = []
    ap = (assertion or {}).get("applicability") or {}
    leaves(ap.get("exposure"), out)
    leaves(ap.get("overlap"), out)
    return out

stored = {json.load(open(f))["candidate"]["id"] for f in glob.glob(K + "/*/*/candidates/*.json")}
props = collections.defaultdict(list)
for f in glob.glob(K + "/*/*/proposals/*.json"):
    p = json.load(open(f))["proposal"]
    props[p["candidateId"]].append(p)
facts = collections.defaultdict(list)
for f in glob.glob(K + "/*/*/facts/*.json"):
    fct = json.load(open(f))["fact"]
    for c in fct.get("candidates") or []:
        facts[c].append(fct)

assigned, rules, reasons = {}, {}, collections.Counter()
drift_new = collections.Counter()
for prod, fr, to in EDGES:
    edge = run("upgrade", "-o", "json", prod, fr, to)
    ch = {c["id"]: c for c in edge["changes"]}
    for cand in run("semantic", "candidates", prod, fr, to, "-o", "json")["candidates"]:
        cid = cand["id"]
        if cid in assigned:
            continue
        rule = None
        fl = [r for fct in facts.get(cid, []) for r in undecidable(fct["assertion"])]
        pl = [r for p in props.get(cid, []) for r in undecidable(p["assertion"])]
        if fl:
            rule = "R1"; reasons.update(fl)
        elif pl:
            rule = "R1b"; reasons.update(pl)
        elif props.get(cid) and all(not any((p["assertion"] or {}).get(a) for a in ("subject", "change", "applicability", "consequence")) for p in props[cid]):
            rule = "R2"
        elif cid not in stored:
            l4 = any(ch.get(m["changeId"], {}).get("routine") or ch.get(m["changeId"], {}).get("provenance", {}).get("rule", "").startswith("lines:")
                     for m in cand["members"])
            if l4:
                rule = "R3"
            else:
                drift_new[prod] += 1
        if rule:
            assigned[cid] = (prod, fr, to)
            rules[cid] = rule

os.makedirs(os.path.join(OUT, "only"), exist_ok=True)
byfile = collections.defaultdict(list)
for cid, (prod, fr, to) in assigned.items():
    byfile[(prod, fr, to, rules[cid])].append(cid)
for (prod, fr, to, rule), ids in byfile.items():
    with open(os.path.join(OUT, "only", f"{prod}__{fr}__{to}__{rule}.txt"), "w") as fh:
        fh.write(f"# semantic-4 selection rule {rule}\n" + "\n".join(sorted(ids)) + "\n")
cnt = collections.Counter(rules.values())
print("selected candidates by rule:", dict(sorted(cnt.items())), "total", len(rules))
per = collections.defaultdict(collections.Counter)
for cid, r in rules.items():
    per[assigned[cid][0]][r] += 1
for prod in sorted(per):
    print(f"  {prod:20s} {dict(sorted(per[prod].items()))}")
print("undecidable leaf reasons on R1/R1b candidates:", dict(reasons))
print("new candidates not from L4 (integration edge drift; not selected):", dict(drift_new))
