import os,random
exec(open(os.path.join(os.path.dirname(os.path.abspath(__file__)),'verify.py')).read())
random.seed(20261002)
quota={('rendered-diff','subject'):3,('canonical','applicability'):99,('canonical','consequence'):6,('compat','subject'):3,('crd','change'):6,('crd','subject'):5,('image','subject'):1,('restatement','change'):6,('restatement','subject'):5,('values','change'):6,('values','subject'):5}
pool={}
for v in sorted(vals,key=lambda v:v['id']):
    for ck in v['checks']:
        if ck['outcome']=='confirmed':
            k=(v['validator'].split('.')[1].split('@')[0].replace('rendered-diff','rendered-diff'),ck['aspect'])
            k=(('rendered-diff' if 'render' in v['validator'] else v['validator'].split('.')[1].split('@')[0]),ck['aspect'])
            pool.setdefault(k,[]).append((v,ck))
sample=[]
for k,n in quota.items():
    items=pool.get(k,[])
    sample+= [(k,)+x for x in (items if n>=len(items) else random.sample(items,n))]
print(len(sample))
def raw(v,cd):
    a=v['assertion']; s=a['subject']; out=[]
    pr,f,t=edge_of(cd['id']); rf,rt=rel(pr,f),rel(pr,t)
    if s['family']=='helm-value':
        for c in values_of(rt):
            ef,et=values_of(rf).get(c,{}),values_of(rt)[c]
            ks=[k for k in set(ef)|set(et) if k==s['path'] or k.startswith(s['path']+'.')]
            if ks: out.append('%s: from=%s to=%s'%(c,[(k,ef.get(k)) for k in sorted(ks)[:3]],[(k,et.get(k)) for k in sorted(ks)[:3]]))
    elif s['family'] in('crd-field','gvk'):
        cf,ct=crdv(rf,s['group'],s['kind']),crdv(rt,s['group'],s['kind'])
        for n,c in (('from',cf),('to',ct)):
            if not c: out.append(n+': no crd'); continue
            for ver in c['versions']:
                if s.get('version') and ver['name']!=s['version']: continue
                if s['family']=='gvk': out.append('%s v:%s served=%s storage=%s'%(n,ver['name'],ver['served'],ver['storage']))
                else:
                    fl=fieldof(ver,s['path']); out.append('%s %s %s'%(n,ver['name'],fl if fl else ('path present' if s['path'] in ver['schemaPaths'] else 'absent')))
    return out
def verdict_of(v,ck):
    cd=cands[v['candidateId']]; a=v['assertion']; s=a['subject']; ch=a['change']; fam=s['family']
    if ck['aspect'] in ('applicability','consequence') or ck['aspect']=='change':
        if fam=='helm-value': r,_=truth_values(s,ch,cd)
        elif fam=='crd-field': r,_=truth_crd(s,ch,cd)
        elif fam=='gvk': r,_=truth_gvk(s,ch,cd)
        else: r=None
        return r
    # subject
    pr,f,t=edge_of(cd['id'])
    if fam=='helm-value':
        vf,vt=values_of(rel(pr,f)),values_of(rel(pr,t))
        return any(hasn(norm(vf.get(c,{})),s['path']) or hasn(norm(vt[c]),s['path']) for c in vt)
    if fam=='crd-field':
        cf,ct=crdv(rel(pr,f),s['group'],s['kind']),crdv(rel(pr,t),s['group'],s['kind'])
        return any(s['path'] in x['schemaPaths'] for c in (cf,ct) if c for x in c['versions'])
    if fam=='gvk': return True
    return None
if __name__=='__main__':
    for i,(k,v,ck) in enumerate(sample):
        cd=cands[v['candidateId']]; a=v['assertion']
        print('#%d %s %s %s | %s'%(i,k,v['id'],ck['rule'],ck['detail'][:110]))
        print('   T:',cd['title'][:120]); print('   A:',subj(a),'|',chg(a)[:110])
        if a.get('applicability'): print('   APP:',json.dumps(a['applicability'])[:220])
        if a.get('consequence'): print('   CONS:',a['consequence']['kind'])
        if a['subject']['family'] in ('helm-value','crd-field','gvk'):
            for r in raw(v,cd)[:5]: print('   RAW:',str(r)[:230])

if __name__=='__main__':
    import collections
    T=collections.defaultdict(collections.Counter)
    for k,v,ck in sample:
        r=verdict_of(v,ck)
        T[k][{True:'correct',False:'WRONG',None:'ambiguous/manual'}[r]]+=1
    for k in quota: print(k,dict(T[k]))
