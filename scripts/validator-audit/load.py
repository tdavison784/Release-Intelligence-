import json,glob,os,collections
KV=os.environ['KV']  # a knowledge dir with validations written by ri knowledge validate
cands={};props={};vals=[]
for f in glob.glob(KV+'/**/*.json',recursive=True):
    if '/failures/' in f: continue
    d=json.load(open(f))
    k=d.get('kind')
    if k=='candidate': cands[d['candidate']['id']]=d['candidate']
    elif k=='proposal': props[d['proposal']['id']]=d['proposal']
    elif k=='validation': vals.append(d['validation'])
def subj(a):
    s=a.get('subject') or {}
    return '%s %s/%s/%s %s%s'%(s.get('family'),s.get('group',''),s.get('kind',''),s.get('name',''),s.get('path',''),'' if not s.get('version') else ' @'+s['version'])
def chg(a):
    c=a.get('change') or {}
    return '%s %s→%s%s'%(c.get('type'),c.get('before'),c.get('after'),' repl='+json.dumps(c['replacedBy']) if c.get('replacedBy') else '')
