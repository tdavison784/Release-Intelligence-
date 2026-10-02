"""Compare two knowledge dirs validated by different validator versions
(KV = before, KV_AFTER = after): which confirmations disappeared, and were
they true (the cost of caution), wrong (a false confirmation removed) or
ambiguous? Truth comes from the independent re-check in verify.py."""
import os, glob, json, collections
exec(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), 'sample.py')).read().split("if __name__=='__main__':")[0])
after = {}
for f in glob.glob(os.environ['KV_AFTER'] + '/**/validations/*.json', recursive=True):
    d = json.load(open(f))['validation']
    after[d['id']] = d
C = collections.Counter()
for v in vals:
    for ck in v['checks']:
        if ck['outcome'] != 'confirmed':
            continue
        n = [c['outcome'] for c in after[v['id']]['checks'] if c['aspect'] == ck['aspect']][0]
        if n == 'confirmed':
            continue
        fam = v['assertion'].get('subject', {}).get('family')
        r = verdict_of(v, ck) if fam in ('helm-value', 'crd-field', 'gvk') else None
        C[(v['validator'].split('.')[1].split('@')[0], ck['aspect'], n, {True: 'was-true', False: 'was-WRONG', None: 'ambiguous'}[r])] += 1
for k, n in sorted(C.items()):
    print(k, n)
