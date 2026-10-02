#!/usr/bin/env python3
"""Generate the per-product tables and difference classification used by REPORT.md.
Run from the repo root: python3 docs/rerun/report_tables.py > docs/rerun/tables.md"""
import json,glob,os,collections
bs=json.load(open('docs/rerun/raw/stats-baseline.json')); ls=json.load(open('docs/rerun/stats.json'))
B={p['product']:p['check'] for p in bs['products']}; L={p['product']:p['check'] for p in ls['products']}
# classification rules: (baseline outcome, live outcome, channel-change) -> class
def classify(p,subj,chan_b,chan_l,ob,ol,detail_l):
    if ob is None:  # new in live
        if p=='karpenter' and ol=='unverifiable': return 'T'   # ECR 429 on newly-read chart contents
        if p=='karpenter': return 'E'
        if p=='kube-prometheus-stack' or p=='strimzi' or p=='istio' or p=='cert-manager': return 'E'
        return 'E'
    if ob=='unverifiable' and ol=='pass': return 'A'
    if ob=='covered' and ol=='pass': return 'A'
    if ob=='pass' and ol=='pass' and chan_b!=chan_l: return 'A'
    if ob=='unverifiable' and ol=='fail': return 'D'
    if ol=='unverifiable' and 'HTTP 429' in detail_l or (ol=='unverifiable' and 'oci: unavailable' in detail_l and p=='karpenter'): return 'T'
    if ob=='pass' and ol=='unverifiable': return 'T'
    return 'C'
tot=collections.Counter(); rows=[]
for f in sorted(glob.glob('docs/onboarding/checks/*.json')):
    p=os.path.basename(f)[:-5]; b=json.load(open(f)); l=json.load(open(f'docs/rerun/checks/{p}.json'))
    key=lambda c:(c['subject'],c['release'])
    kb=collections.Counter((c['subject'],c.get('channel'),c['release'],c['outcome']) for c in b['checks'])
    kl=collections.Counter((c['subject'],c.get('channel'),c['release'],c['outcome']) for c in l['checks'])
    ob=kb-kl; ol=kl-kb
    cls=collections.Counter()
    detl={(c['subject'],c['release'],c['outcome']):c['detail'] for c in l['checks']}
    # pair by (subject,release)
    bsr=collections.defaultdict(list); lsr=collections.defaultdict(list)
    for k,v in ob.items():
        for _ in range(v): bsr[(k[0],k[2])].append(k)
    for k,v in ol.items():
        for _ in range(v): lsr[(k[0],k[2])].append(k)
    for kk in set(bsr)|set(lsr):
        xs=bsr.get(kk,[]); ys=lsr.get(kk,[])
        for i in range(max(len(xs),len(ys))):
            x=xs[i] if i<len(xs) else None; y=ys[i] if i<len(ys) else None
            c=classify(p,kk[0],x[1] if x else None,y[1] if y else None,x[3] if x else None,y[3] if y else None,detl.get((kk[0],kk[1],'unverifiable'),''))
            if x and not y: c='E'  # baseline check no longer produced (definition/release set changed)
            cls[c]+=1
    # argo-cd release swap is C/E mix; leave as computed
    cb=collections.Counter(c['outcome'] for c in b['checks']); cl=collections.Counter(c['outcome'] for c in l['checks'])
    sb=B[p]; sl=L[p]
    rows.append((p,len(l['releases']),cb,cl,sb,sl,cls)); tot.update(cls)
print('## Per product: check outcomes baseline -> live\n')
print('| product | releases | pass | covered | n/a | unverifiable | fail | subjects validated/failing/insufficient/unverifiable (base → live) | diffs A/C/D/E/T |')
print('|---|---|---|---|---|---|---|---|---|')
for p,n,cb,cl,sb,sl,cls in rows:
    f=lambda o:f"{cb.get(o,0)} → {cl.get(o,0)}"
    print(f"| {p} | {n} | {f('pass')} | {f('covered')} | {f('not-applicable')} | {f('unverifiable')} | {f('fail')} | {sb['validated']}/{sb['failing']}/{sb['insufficient']}/{sb['unverifiable']} → {sl['validated']}/{sl['failing']}/{sl['insufficient']}/{sl['unverifiable']} | {'/'.join(str(cls.get(k,0)) for k in 'ACDET')} |")
print('\nTotals by class (A sandbox artifact, C upstream changed, D defect, E definition changed since baseline, T throttling/transient):',dict(tot))
