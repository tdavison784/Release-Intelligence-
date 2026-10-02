#!/usr/bin/env python3
"""Join `ri eval -render -render-json STATS -o json > EVAL` with the eval's
per-link results: which expected impact links (eval/cases expectedImpact) have
their change restated by a complete customer render. Measurement only — the
expectations are read to score, never to author knowledge (FLEET.md).

usage: r17_join.py EVAL.json STATS.json
"""
import json, re, sys

d = json.load(open(sys.argv[1])); stats = json.load(open(sys.argv[2]))
by_case = {}
for s in stats:
    m = re.search(r'\[(.+)\]$', s['case'])
    if m:
        by_case[m.group(1)] = s
tot = dict(links=0, missed=0, missedRestated=0, hitRestated=0, notAffected=0, notAffectedRestated=0)
rows = []
for r in d['results']:
    s = by_case.get(r['caseId'])
    if not s or not r.get('envImpact'):
        continue
    restated = set(s.get('restatedChanges') or [])
    chg = {m['expectedId']: {h['changeId'] for h in (m.get('hits') or [])} for m in (r.get('matches') or [])}
    for l in r['envImpact']:
        rs = bool(chg.get(l['expectedId'], set()) & restated)
        if l['relevance'] == 'not-affected':
            tot['notAffected'] += 1; tot['notAffectedRestated'] += rs
            if rs: rows.append(f"not-affected link restated by a render: {r['caseId']} {l['expectedId']}")
            continue
        tot['links'] += 1
        if l['hit']:
            tot['hitRestated'] += rs
        else:
            tot['missed'] += 1; tot['missedRestated'] += rs
            if rs: rows.append(f"missed {l['relevance']} link restated by a complete render: {r['caseId']} {l['expectedId']}")
print(json.dumps(tot))
print("\n".join(rows))
