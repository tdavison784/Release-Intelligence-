# Independent verification of confirmed validation results against the raw
# ingested store (written separately from internal/semvalidate: it re-reads
# the stored release JSON and applies its own truth rules, including the
# `defaults.` wrapper normalisation the Go validators did not know about).
import json,sys,os
exec(open(os.path.join(os.path.dirname(os.path.abspath(__file__)),'audit1.py')).read())
def jcanon(s):
    if s is None: return None
    try: return json.dumps(json.loads(s),sort_keys=True,separators=(',',':'))
    except Exception: return json.dumps(s)
def jdecode(s):
    c=jcanon(s)
    # double-encoded object/array default
    try:
        v=json.loads(c)
        if isinstance(v,str):
            w=json.loads(v)
            if isinstance(w,(dict,list)): return json.dumps(w,sort_keys=True,separators=(',',':'))
    except Exception: pass
    return c
def norm(e):
    ks=list(e)
    if len(ks)>=5 and sum(k.startswith('defaults.') for k in ks)>=0.8*len(ks):
        return {k[len('defaults.'):] if k.startswith('defaults.') else k:v for k,v in e.items()}
    return e
def hasn(e,p): return has(e,p)
def truth_values(s,ch,cd):
    pr,f,t=edge_of(cd['id']); rf,rt=rel(pr,f),rel(pr,t)
    vf,vt=values_of(rf),values_of(rt)
    p=s['path']; name=s.get('name')
    charts=[c for c in vt if c in vf and (not name or name in (c,c.replace('chart-','')))]
    typ=ch['type']
    res=[]
    for c in charts:
        ef,et=norm(vf[c]),norm(vt[c])
        if not (hasn(ef,p) or hasn(et,p)): continue
        fh,th=hasn(ef,p),hasn(et,p)
        if typ=='removed': ok=fh and not th
        elif typ=='added': ok=(not fh) and th
        elif typ in('default-changed','value-changed'):
            ok=(p in ef and p in et and jdecode(ef[p])==jdecode(ch.get('before')) and jdecode(et[p])==jdecode(ch.get('after')))
        elif typ=='renamed':
            np_=ch['replacedBy']['path']; ok=fh and not th and not hasn(ef,np_) and hasn(et,np_)
        else: ok=None
        res.append((c,ok))
    if not res: return None,'key in no chart'
    oks={o for _,o in res}
    if oks=={True}: return True,'holds in %s'%[c for c,_ in res]
    if True in oks and False in oks: return None,'charts disagree %s'%res
    return False,'does not hold %s'%res
def crdv(r,g,k):
    for s in r['snapshots']:
        if s['kind']=='crds':
            for c in s['crds']['crds']:
                if c['group']==g and c['kind']==k: return c
def fieldof(v,p):
    for f in v.get('fields',[]):
        if f['path']==p: return f
def truth_crd(s,ch,cd):
    pr,f,t=edge_of(cd['id']); cf,ct=crdv(rel(pr,f),s['group'],s['kind']),crdv(rel(pr,t),s['group'],s['kind'])
    if not cf or not ct: return None,'crd missing'
    p=s['path']; typ=ch['type']
    stor=[v for v in ct['versions'] if v['storage']]
    vers=[s['version']] if s.get('version') else [v['name'] for v in ct['versions']]
    def vv(c,n):
        for v in c['versions']:
            if v['name']==n: return v
    rows=[]
    for n in vers:
        a,b=vv(cf,n),vv(ct,n)
        pa=bool(a) and p in a['schemaPaths']; pb=bool(b) and p in b['schemaPaths']
        rows.append((n,a is not None,pa,b is not None,pb,a,b))
    sv=stor[0]['name'] if stor else None
    if typ=='added':
        if s.get('version'):
            n,ha,pa,hb,pb,_,_=rows[0]
            if not ha: return None,'version itself new'
            return (pb and not pa),'v%s %s>%s'%(n,pa,pb)
        anyf=any(vv(cf,v['name']) and p in vv(cf,v['name'])['schemaPaths'] for v in ct['versions']) or any(p in v['schemaPaths'] for v in cf['versions'])
        return (not anyf and any(p in v['schemaPaths'] for v in ct['versions'])),'unversioned added; anyfrom=%s'%anyf
    if typ=='removed':
        r=[x for x in rows if x[2]]
        return (bool(r) and all(not x[4] for x in r)),str([(x[0],x[2],x[4]) for x in rows])
    if typ=='now-required':
        for n,ha,pa,hb,pb,a,b in rows:
            if a and b and pa and pb:
                fa,fb=fieldof(a,p),fieldof(b,p)
                if fa and fb: return ((not fa.get('required')) and fb.get('required',False)),n
        return None,'no field facts'
    if typ=='default-changed':
        for n,ha,pa,hb,pb,a,b in rows:
            if a and b and pa and pb:
                fa,fb=fieldof(a,p),fieldof(b,p)
                if fa and fb:
                    aft_ok = (not fb.get('default')) if jcanon(ch.get('after'))=='null' else (bool(fb.get('default')) and jdecode(fb.get('default'))==jdecode(ch.get('after')))
                    return bool(fa.get('default')) and jdecode(fa.get('default'))==jdecode(ch.get('before')) and aft_ok,n
        return None,'no field facts'
    return None,'untested type '+typ
def truth_gvk(s,ch,cd):
    pr,f,t=edge_of(cd['id']); cf,ct=crdv(rel(pr,f),s['group'],s['kind']),crdv(rel(pr,t),s['group'],s['kind'])
    typ=ch['type']; ver=s['version']
    def vv(c,n):
        if not c: return None
        for v in c['versions']:
            if v['name']==n: return v
    a,b=vv(cf,ver),vv(ct,ver)
    if typ=='removed': return (a is not None) and (b is None or (a['served'] and not b['served'])),'%s>%s'%(a is not None,b is not None)
    if typ=='added': return (a is None) and (b is not None),''
    if typ=='deprecated': return a is not None and b is not None and not a.get('deprecated') and b.get('deprecated',False),''
    if typ=='value-changed':
        sa=[v['name'] for v in cf['versions'] if v['storage']] if cf else []; sb=[v['name'] for v in ct['versions'] if v['storage']] if ct else []
        return (jcanon(ch.get('before'))==json.dumps(sa[0]) and jcanon(ch.get('after'))==json.dumps(sb[0])) if sa and sb else None,''
    return None,'untested'
