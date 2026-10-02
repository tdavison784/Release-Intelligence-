#!/usr/bin/env python3
"""Compare baseline (docs/onboarding/checks) vs live (docs/rerun/checks) ri check reports.
Prints per-product outcome counts and per-(subject,release) outcome changes."""
import json,glob,collections,sys,os
rows=[];diffs=[]
for f in sorted(glob.glob('docs/onboarding/checks/*.json')):
    p=os.path.basename(f)[:-5]; b=json.load(open(f)); l=json.load(open(f'docs/rerun/checks/{p}.json'))
    cb=collections.Counter(c['outcome'] for c in b['checks']); cl=collections.Counter(c['outcome'] for c in l['checks'])
    rows.append((p,cb,cl,b['releases'],l['releases']))
    kb={(c['subject'],c['release']):c for c in b['checks']}; kl={(c['subject'],c['release']):c for c in l['checks']}
    for k in sorted(set(kb)|set(kl)):
        x,y=kb.get(k),kl.get(k)
        ox=x['outcome'] if x else None; oy=y['outcome'] if y else None
        if ox!=oy: diffs.append((p,k[0],k[1],ox,oy,(x or {}).get('detail','')[:140],(y or {}).get('detail','')[:140]))
O=['pass','covered','not-applicable','unverifiable','fail']
print('product|rel|'+'|'.join('B:'+o for o in O)+'|'+'|'.join('L:'+o for o in O))
for p,cb,cl,rb,rl in rows:
    print(f"{p}|{len(rl)}|"+'|'.join(str(cb.get(o,0)) for o in O)+'|'+'|'.join(str(cl.get(o,0)) for o in O)+('' if rb==rl else '  RELEASES DIFFER'))
print('\nDIFFS',len(diffs))
json.dump(diffs,open('docs/rerun/raw/diffs.json','w'),indent=1)
