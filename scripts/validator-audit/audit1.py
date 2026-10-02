exec(open(os.path.join(os.path.dirname(os.path.abspath(__file__)),'load.py')).read())
import re
STORE=os.environ['STORE']  # <state>/store
def rel(p,v): return json.load(open(f'{STORE}/{p}/releases/{v}.json'))
# edges as in fullrun (product, from, to)
EDGES=[('argo-cd','2.14.5','3.0.0'),('cert-manager','1.16.0','1.17.0'),('cert-manager','1.17.0','1.18.0'),('cilium','1.15.6','1.17.0'),('cilium','1.16.1','1.17.0'),('istio','1.23.4','1.24.0'),('karpenter','0.37.8','1.0.0'),('strimzi','0.45.0','0.46.0')]
# candidate -> list of edges (by regenerating ids is heavy); use candidate release to pick edges
def edges_for(cand):
    p=cand['product']; rl=cand.get('release','').lstrip('v')
    return [(a,b,c) for (a,b,c) in EDGES if a==p and c==rl] or [(a,b,c) for (a,b,c) in EDGES if a==p]
def values_of(r): return {s['artifactId']:s['values']['entries'] for s in r['snapshots'] if s['kind']=='helm-values' and s.get('values')}
def has(e,p): return p in e or any(k.startswith(p+'.') or k.startswith(p+'[') for k in e)
EDGEMAP=json.load(open(os.environ['EDGEMAP']))  # candidate id -> product:from:to (first requested edge wins)
def edge_of(cid):
    p,f,t=EDGEMAP[cid].split(':'); return (p,f.lstrip('v'),t.lstrip('v'))
def edges_for(cand): return [edge_of(cand['id'])]
