import os
exec(open(os.path.join(os.path.dirname(os.path.abspath(__file__)),'verify.py')).read())
from collections import Counter
C=Counter(); wrong=[]
for v in vals:
    cd=cands[v['candidateId']]; a=v['assertion']; s=a.get('subject') or {}; ch=a.get('change') or {}
    fam=s.get('family')
    for ck in v['checks']:
        if ck['outcome']!='confirmed': continue
        key=(v['validator'].split('.')[1],ck['aspect'],fam)
        r=why=None
        if v['validator']=='semvalidate.canonical@v1' and ck['aspect']=='consequence' or v['validator']=='semvalidate.canonical@v1' and ck['aspect']=='applicability':
            # needs underlying subject+change truth
            if fam=='helm-value': r,why=truth_values(s,ch,cd)
            elif fam=='crd-field': r,why=truth_crd(s,ch,cd)
            elif fam=='gvk': r,why=truth_gvk(s,ch,cd)
            else: r,why=None,'family '+str(fam)
        elif fam=='helm-value' and v['validator'] in ('semvalidate.values@v1','semvalidate.restatement@v1'):
            if ck['aspect']=='change': r,why=truth_values(s,ch,cd)
            else:
                pr,f,t=edge_of(cd['id']); vf,vt=values_of(rel(pr,f)),values_of(rel(pr,t))
                r=any(hasn(norm(vf.get(c,{})),s['path']) or hasn(norm(vt[c]),s['path']) for c in vt); why='exists'
        elif fam=='crd-field' and v['validator'] in ('semvalidate.crd@v1','semvalidate.restatement@v1'):
            if ck['aspect']=='change': r,why=truth_crd(s,ch,cd)
            else:
                pr,f,t=edge_of(cd['id']); cf,ct=crdv(rel(pr,f),s['group'],s['kind']),crdv(rel(pr,t),s['group'],s['kind'])
                r=any(s['path'] in x['schemaPaths'] for c in (cf,ct) if c for x in c['versions']); why='exists'
        elif fam=='gvk' and v['validator'] in ('semvalidate.crd@v1','semvalidate.restatement@v1'):
            if ck['aspect']=='change': r,why=truth_gvk(s,ch,cd)
            else: r,why=True,'gvk subject (manual)'
        else: r,why=None,'manual'
        C[(key,r)]+=1
        if r is not True: wrong.append((key,r,why,subj(a),chg(a)[:50],cd['title'][:60],v['id']))
for k,n in sorted(C.items(),key=str): print(k,n)
print()
for w in wrong: print(w)
