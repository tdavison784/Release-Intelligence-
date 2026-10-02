#!/usr/bin/env python3
"""Model-comparison metrics for the Phase 3 enrichment benchmark.

Reads docs/phase3/model-comparison/runs/<model>/ (r4.json, r4.stderr, smoke.json,
smoke.stderr, eval.json) and prints the funnel, contract-pressure, verdict and
agreement tables as Markdown. With --packet it also writes the blinded judging
packet and its unblinding key from the T1 (report-4) runs.

Deterministic: the packet shuffle uses a fixed seed. Standard library only.
"""
import argparse
import json
import random
import re
import sys
from collections import Counter
from itertools import combinations
from pathlib import Path

HERE = Path(__file__).resolve().parent
RUNS = HERE / "runs"
VERDICTS = ["plausibly-applies", "not-applicable", "undetermined"]
SUMMARY_RE = re.compile(
    r"(\d+) candidates, (\d+) prompts, (\d+) enrichments accepted, (\d+) rejected, "
    r"(\d+) pending, (\d+) failed; (\d+) unknown"
)
# GLM's live smoke run as documented in docs/IMPACT-ENRICHMENT.md (pre-round-4 state).
GLM_SMOKE_DOC = dict(candidates=46, prompts=30, accepted=28, rejected=2, pending=0, failed=0, suggestions=19)


def load(path):
    try:
        return json.loads(Path(path).read_text())
    except (FileNotFoundError, json.JSONDecodeError):
        return None


def summary_line(path):
    try:
        text = Path(path).read_text()
    except FileNotFoundError:
        return None
    for line in text.splitlines():
        m = SUMMARY_RE.search(line)
        if m:
            keys = ["candidates", "prompts", "accepted", "rejected", "pending", "failed", "suggestions"]
            return dict(zip(keys, map(int, m.groups())))
    return None


def reason_class(reason):
    """Bucket a refusal reason. Contract-pressure buckets first, then generic schema."""
    r = reason.lower()
    if "action" in r and ("required" in r or "forbidden" in r):
        return "forbidden: action-required"
    if "maximum 8" in r or "maxitems" in r or "at most 8" in r or "more than 8" in r:
        return "citations > 8"
    if "inputevidence" in r or "not among" in r or "not in the prompt" in r or "outside" in r or "unknown evidence" in r:
        return "citation outside inputEvidence"
    if "additional properties" in r:
        return "schema: extra property"
    if "missing property" in r:
        return "schema: missing property"
    if "schema" in r:
        return "schema: other"
    if "parse" in r or "json" in r:
        return "unparseable"
    return "other"


def run_metrics(doc, stderr_path):
    if doc is None:
        return None
    er = doc.get("enrichmentRun") or {}
    line = summary_line(stderr_path) or {}
    applic = [e for e in doc.get("enrichments", []) if e.get("kind") in VERDICTS]
    verdicts = {}
    for e in applic:
        rule = (e.get("provenance") or {}).get("rule", "")
        verdicts[rule.removeprefix("cand:")] = e["kind"]
    rejected = er.get("rejected") or []
    suggestions = sum(1 for f in doc.get("findings", []) if f.get("suggestedClassification"))
    accepted = er.get("accepted", 0)
    prompts = er.get("requests", 0)
    return dict(
        candidates=line.get("candidates"),
        prompts=prompts,
        accepted=accepted,
        rejected=len(rejected),
        rejected_by=Counter(reason_class(r.get("reason", "")) for r in rejected),
        rejected_raw=[(r.get("group"), r.get("reason")) for r in rejected],
        rejected_groups={r.get("group"): reason_class(r.get("reason", "")) for r in rejected},
        pending=er.get("pending", 0),
        failed=line.get("failed", 0),
        suggestions=suggestions,
        acceptance=(accepted / prompts) if prompts else None,
        verdicts=verdicts,
        verdict_dist=Counter(verdicts.values()),
        applic=applic,
    )


def cohen(a, b):
    keys = sorted(set(a) & set(b))
    n = len(keys)
    if n == 0:
        return None, 0, None
    po = sum(a[k] == b[k] for k in keys) / n
    ca, cb = Counter(a[k] for k in keys), Counter(b[k] for k in keys)
    pe = sum(ca[c] * cb[c] for c in VERDICTS) / (n * n)
    kappa = None if pe == 1 else (po - pe) / (1 - pe)
    return kappa, n, po


def fleiss(raters):
    keys = sorted(set.intersection(*(set(r) for r in raters))) if raters else []
    n, m = len(keys), len(raters)
    if n == 0 or m < 2:
        return None, n
    totals = Counter()
    pis = []
    for k in keys:
        counts = Counter(r[k] for r in raters)
        totals.update(counts)
        pis.append((sum(c * c for c in counts.values()) - m) / (m * (m - 1)))
    pbar = sum(pis) / n
    pe = sum((totals[c] / (n * m)) ** 2 for c in VERDICTS)
    return (None if pe == 1 else (pbar - pe) / (1 - pe)), n


def fmt(x, pct=False):
    if x is None:
        return "—"
    if pct:
        return f"{x * 100:.0f}%"
    if isinstance(x, float):
        return f"{x:.2f}"
    return str(x)


def models():
    order = ["glm-5.3-flash",
             "claude-opus-5-5-raw", "claude-sonnet-5-5-raw", "claude-haiku-4-5-raw",
             "claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-4-5",
             "gpt-6-astra", "gpt-6.1-sol", "gpt-6-luna"]
    present = [p.name for p in RUNS.iterdir() if p.is_dir()] if RUNS.exists() else []
    return [m for m in order if m in present] + sorted(set(present) - set(order))


def report():
    data = {}
    for m in models():
        d = RUNS / m
        data[m] = dict(
            r4=run_metrics(load(d / "r4.json"), d / "r4.stderr"),
            smoke=run_metrics(load(d / "smoke.json"), d / "smoke.stderr"),
            eval=load(d / "eval.json"),
            t3b=load(d / "eval-t3b.json"),
        )
    out = []
    for run, label in [("r4", "T1 — report-4 fixture (digest-comparable)"), ("smoke", "T2 — customer-repo smoke")]:
        out.append(f"### Funnel: {label}\n")
        out.append("| model | candidates | prompts | accepted | rejected | pending | failed | suggestions | acceptance |")
        out.append("|---|---|---|---|---|---|---|---|---|")
        if run == "smoke":
            g = GLM_SMOKE_DOC
            out.append(f"| glm-5.3-flash (documented, live, pre-round-4) | {g['candidates']} | {g['prompts']} | {g['accepted']} | "
                       f"{g['rejected']} | {g['pending']} | {g['failed']} | {g['suggestions']} | {fmt(g['accepted'] / g['prompts'], True)} |")
        for m, v in data.items():
            r = v[run]
            if r is None:
                continue
            out.append(f"| {m} | {fmt(r['candidates'])} | {r['prompts']} | {r['accepted']} | {r['rejected']} | "
                       f"{r['pending']} | {r['failed']} | {r['suggestions']} | {fmt(r['acceptance'], True)} |")
        out.append("")

    out.append("### Contract pressure: refusals by reason (T1 + T2)\n")
    out.append("| model | run | reason class | candidate group | raw reason |")
    out.append("|---|---|---|---|---|")
    for m, v in data.items():
        for run in ("r4", "smoke"):
            r = v[run]
            if r is None:
                continue
            for g, reason in r["rejected_raw"]:
                out.append(f"| {m} | {run} | {reason_class(reason)} | `{g}` | {reason.replace('|', '/')[:220]} |")
    out.append("")

    for run, label in [("r4", "T1"), ("smoke", "T2")]:
        out.append(f"### Verdict distribution ({label}, accepted answers)\n")
        out.append("| model | plausibly-applies | not-applicable | undetermined | n |")
        out.append("|---|---|---|---|---|")
        for m, v in data.items():
            r = v[run]
            if r is None:
                continue
            dist = r["verdict_dist"]
            out.append(f"| {m} | {dist['plausibly-applies']} | {dist['not-applicable']} | {dist['undetermined']} | {sum(dist.values())} |")
        out.append("")

    out.append("### Agreement on T1 verdicts (Cohen's κ, per candidate both models answered)\n")
    r4 = {m: v["r4"]["verdicts"] for m, v in data.items() if v["r4"]}
    out.append("| pair | n | raw agreement | κ |")
    out.append("|---|---|---|---|")
    for a, b in combinations(r4, 2):
        k, n, po = cohen(r4[a], r4[b])
        out.append(f"| {a} × {b} | {n} | {fmt(po, True)} | {fmt(k)} |")
    k, n = fleiss(list(r4.values()))
    out.append(f"\nFleiss' κ across {len(r4)} models ({', '.join(r4)}), on the {n} candidates every model answered: **{fmt(k)}**\n")
    claude = [r4[m] for m in r4 if m.startswith("claude-")]
    if len(claude) >= 2:
        k, n = fleiss(claude)
        out.append(f"Fleiss' κ across the Claude models only, n={n}: **{fmt(k)}**\n")

    out.append("### Per-candidate T1 verdicts\n")
    cands = sorted(set().union(*(set(v) for v in r4.values())) | set().union(
        *(set(data[m]["r4"]["rejected_groups"]) for m in r4)))
    abbrev = {"plausibly-applies": "PA", "not-applicable": "NA", "undetermined": "UD"}
    out.append("| candidate | " + " | ".join(r4) + " |")
    out.append("|---|" + "---|" * len(r4))
    for c in cands:
        cells = []
        for m in r4:
            if c in r4[m]:
                cells.append(abbrev[r4[m][c]])
            elif c in data[m]["r4"]["rejected_groups"]:
                cells.append("rej")
            else:
                cells.append("pend")
        out.append(f"| `{c}` | " + " | ".join(cells) + " |")
    out.append("\nPA plausibly-applies (becomes a suggestion), NA not-applicable, UD undetermined, rej refused by ri, "
               "pend no answer (GLM's 4 pending prompts were never answered by the recording run).\n")

    out.append("### T3 — eval suggestion scoring (brief's literal command)\n")
    out.append("| model | suggestionPrecision | suggestionRecall | raw |")
    out.append("|---|---|---|---|")
    for m, v in data.items():
        e = v["eval"]
        if e is None:
            continue
        found = {}

        def walk(x):
            if isinstance(x, dict):
                for k2, v2 in x.items():
                    if k2.lower().startswith("suggestion"):
                        found[k2] = v2
                    walk(v2)
            elif isinstance(x, list):
                for i in x:
                    walk(i)
        walk(e)
        out.append(f"| {m} | {json.dumps(found.get('suggestionPrecision'))} | {json.dumps(found.get('suggestionRecall'))} | "
                   f"`{json.dumps(found)[:200]}` |")
    out.append("")
    out.append("### T3b — eval suggestion scoring, digest-matched paths (run from internal/app)\n")
    out.append("| model | suggestions | labelled | correct | wrong | unlabelled | suggestionPrecision (n) | suggestionRecall (n) |")
    out.append("|---|---|---|---|---|---|---|---|")
    for m, v in data.items():
        e = v["t3b"]
        if e is None:
            continue
        a = e.get("aggregate", {})
        sm = a.get("suggestions") or {}
        out.append(f"| {m} | {sm.get('suggestions', 0)} | {sm.get('labelled', 0)} | {sm.get('labeledCorrect', 0)} | "
                   f"{sm.get('labeledWrong', 0)} | {sm.get('unlabeled', 0)} | {fmt(a.get('suggestionPrecision'))} ({sm.get('labelled', 0)}) | "
                   f"{fmt(a.get('suggestionRecall'))} ({sm.get('unknownLabeled', 0)}) |")
    out.append("")
    return "\n".join(out), data


def packet(data, seed=20261001):
    items = []
    for m, v in data.items():
        r = v["r4"]
        if r is None:
            continue
        doc = load(RUNS / m / "r4.json")
        findings = {f["id"]: f for f in doc.get("findings", [])}
        evidence = {e["id"]: e for e in doc.get("evidence", []) + doc.get("environmentEvidence", [])}
        for e in r["applic"]:
            if e["kind"] != "plausibly-applies":
                continue
            items.append(dict(
                model=m,
                enrichment=e["id"],
                candidate=(e.get("provenance") or {}).get("rule", "").removeprefix("cand:"),
                findings=[dict(id=fid, title=findings.get(fid, {}).get("changeTitle") or findings.get(fid, {}).get("title"),
                               detail=findings.get(fid, {}).get("detail"),
                               breaking=findings.get(fid, {}).get("changeBreaking"),
                               category=findings.get(fid, {}).get("changeCategory"))
                          for fid in e.get("relatesTo", [])],
                suggestion=dict(title=e.get("title"), content=e.get("content"),
                                confidence=(e.get("provenance") or {}).get("confidence")),
                cited=[dict(id=c, uri=evidence.get(c, {}).get("uri"), locator=evidence.get(c, {}).get("locator"),
                            excerpt=evidence.get(c, {}).get("excerpt")) for c in e.get("citations", [])],
            ))
    random.Random(seed).shuffle(items)
    key, blind = [], []
    for i, it in enumerate(items, 1):
        sid = f"S{i:02d}"
        key.append(dict(item=sid, model=it["model"], enrichment=it["enrichment"], candidate=it["candidate"],
                        findings=[f["id"] for f in it["findings"]]))
        blind.append(dict(item=sid, findings=it["findings"], suggestion=it["suggestion"], cited_evidence=it["cited"]))
    return blind, key


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--packet", metavar="DIR", help="write judging/packet.json and judging/key.json into DIR")
    args = ap.parse_args()
    text, data = report()
    print(text)
    if args.packet:
        blind, key = packet(data)
        d = Path(args.packet)
        d.mkdir(parents=True, exist_ok=True)
        (d / "packet.json").write_text(json.dumps(blind, indent=2) + "\n")
        (d / "key.json").write_text(json.dumps(key, indent=2) + "\n")
        print(f"wrote {len(blind)} blinded items to {d}/packet.json (key: {d}/key.json)", file=sys.stderr)


if __name__ == "__main__" and not {"--packet2", "--judging"} & set(sys.argv):
    main()


def packet_round2(data, judged_dir, anchors=8, seed=20261002):
    """Round 2: every T1 suggestion not yet judged, mixed with `anchors` round-1 items
    re-blinded under new ids (test-retest check of the judge). Writes packet-r2/key-r2."""
    d = Path(judged_dir)
    r1_blind = json.loads((d / "packet.json").read_text())
    r1_key = json.loads((d / "key.json").read_text())
    judged = {(k["model"], k["enrichment"]) for k in r1_key}
    blind_all, key_all = packet(data)
    new = [(b, k) for b, k in zip(blind_all, key_all) if (k["model"], k["enrichment"]) not in judged]
    rng = random.Random(seed)
    picks = rng.sample(range(len(r1_blind)), anchors)
    pool = [(b, dict(k, anchor_of=None)) for b, k in new]
    pool += [(r1_blind[i], dict(r1_key[i], anchor_of=r1_key[i]["item"])) for i in picks]
    rng.shuffle(pool)
    blind, key = [], []
    for i, (b, k) in enumerate(pool, 1):
        rid = f"R{i:02d}"
        blind.append(dict(b, item=rid))
        key.append(dict(k, item=rid))
    (d / "packet-r2.json").write_text(json.dumps(blind, indent=2) + "\n")
    (d / "key-r2.json").write_text(json.dumps(key, indent=2) + "\n")
    print(f"round 2: {len(new)} new + {anchors} anchors = {len(blind)} items", file=sys.stderr)


if __name__ == "__main__" and "--packet2" in sys.argv:
    packet_round2(report()[1], HERE / "judging")


def judging_report(jdir=HERE / "judging"):
    """T4: worth-a-look precision = yes / adjudicated, per answering model x judge x persona.
    Rounds are pooled per judge (round-2 anchors are excluded from the pool and used only for
    the test-retest check). Disagreements are listed, never averaged."""
    from collections import defaultdict
    rounds = [("packet.json", "key.json", ""), ("packet-r2.json", "key-r2.json", "-r2")]
    scores = defaultdict(dict)      # judge -> (model, enrichment) -> item answer
    anchors = defaultdict(list)     # judge -> (r1 answer, r2 answer, key)
    meta = {}
    for _, keyf, suffix in rounds:
        if not (jdir / keyf).exists():
            continue
        key = {k["item"]: k for k in json.loads((jdir / keyf).read_text())}
        for f in sorted(jdir.glob(f"answers-*{suffix}.json")):
            if not suffix and f.stem.endswith("-r2"):
                continue
            a = json.loads(f.read_text())
            judge = a["judge"]
            meta[judge] = a.get("model")
            for it in a["items"]:
                k = key[it["item"]]
                if k.get("anchor_of"):
                    anchors[judge].append((k, it))
                    continue
                scores[judge][(k["model"], k["enrichment"])] = dict(it, _key=k)
    out = ["### T4 — \"worth a look\" precision (yes / adjudicated), per answering model × judge × persona\n",
           "| answering model | judge (model) | self-family? | n | platform-engineer yes/maybe/no | PE precision | SRE yes/maybe/no | SRE precision | grounding supported/overstated/unsupported | grounding rate |",
           "|---|---|---|---|---|---|---|---|---|---|"]
    for judge, items in scores.items():
        by_model = defaultdict(list)
        for (m, _), it in items.items():
            by_model[m].append(it)
        for m in models():
            its = by_model.get(m)
            if not its:
                continue
            n = len(its)
            c = {p: Counter(i[p]["worth_look"] for i in its) for p in ("platform-engineer", "sre")}
            g = Counter(i["grounding"] for i in its)
            fam = "**yes**" if (meta[judge] or "").split("-")[0] == m.split("-")[0] else "no"
            out.append(f"| {m} | {judge} ({meta[judge]}) | {fam} | {n} | "
                       f"{c['platform-engineer']['yes']}/{c['platform-engineer']['maybe']}/{c['platform-engineer']['no']} | "
                       f"{fmt(c['platform-engineer']['yes'] / n, True)} | {c['sre']['yes']}/{c['sre']['maybe']}/{c['sre']['no']} | "
                       f"{fmt(c['sre']['yes'] / n, True)} | {g['supported']}/{g['overstated']}/{g['unsupported']} | {fmt(g['supported'] / n, True)} |")
    out.append("\nReference: GLM's published proxy precision, scored by the original Phase 3 proxy reviewers on the 13 packet-time "
               "suggestions: SRE 8/13 = 62%, platform engineer 9/13 = 69%.\n")
    # test-retest
    out.append("### Judge test-retest (round-1 items re-scored blind in round 2)\n")
    out.append("| judge | anchor | round-1 item | model | PE r1→r2 | SRE r1→r2 | grounding r1→r2 |")
    out.append("|---|---|---|---|---|---|---|")
    for judge, pairs in anchors.items():
        for k, it2 in pairs:
            it1 = scores[judge].get((k["model"], k["enrichment"]))
            if not it1:
                continue
            cell = lambda p: f"{it1[p]['worth_look']}→{it2[p]['worth_look']}" + ("" if it1[p]["worth_look"] == it2[p]["worth_look"] else " ⚠")
            gcell = f"{it1['grounding']}→{it2['grounding']}" + ("" if it1["grounding"] == it2["grounding"] else " ⚠")
            out.append(f"| {judge} | {k['item']} | {k['anchor_of']} | {k['model']} | {cell('platform-engineer')} | {cell('sre')} | {gcell} |")
    # cross-judge disagreements
    judges = list(scores)
    out.append("\n### Judge disagreements (same item, different worth_look)\n")
    if len(judges) < 2:
        out.append(f"Only one judge has scored so far ({', '.join(judges)}); cross-judge disagreements will be listed when judge-codex runs.\n")
    else:
        out.append("| model | enrichment | persona | " + " | ".join(judges) + " |")
        out.append("|---|---|---|" + "---|" * len(judges))
        for key_ in sorted(set.intersection(*(set(scores[j]) for j in judges))):
            for p in ("platform-engineer", "sre"):
                vals = [scores[j][key_][p]["worth_look"] for j in judges]
                if len(set(vals)) > 1:
                    out.append(f"| {key_[0]} | {key_[1]} | {p} | " + " | ".join(vals) + " |")
    # persona disagreements per judge
    out.append("\n### Persona splits (same judge, PE yes vs SRE no or the reverse)\n")
    out.append("| judge | model | item | PE | SRE | PE comment | SRE comment |")
    out.append("|---|---|---|---|---|---|---|")
    for judge, items in scores.items():
        for (m, e), it in sorted(items.items()):
            a, b = it["platform-engineer"]["worth_look"], it["sre"]["worth_look"]
            if {a, b} == {"yes", "no"}:
                out.append(f"| {judge} | {m} | {it['_key']['item']} | {a} | {b} | {it['platform-engineer'].get('comment','')} | {it['sre'].get('comment','')} |")
    # per-finding matrix: which findings drew a "yes", from which models
    out.append("\n### Per-finding worth_look (PE then SRE; y yes, m maybe, n no, · not suggested by that model)\n")
    cells = defaultdict(dict)
    for judge, items in scores.items():
        for (m, _), it in items.items():
            f = it["_key"]["findings"][0]
            cells[(judge, f)][m] = it["platform-engineer"]["worth_look"][0] + it["sre"]["worth_look"][0]
    titles = {f["id"]: (f.get("changeTitle") or "")[:60] for f in (load(RUNS / "glm-5.3-flash" / "r4.json") or {}).get("findings", [])}
    ms = [m for m in models() if any(m in r for r in cells.values())]
    out.append("| judge | finding | change | " + " | ".join(ms) + " |")
    out.append("|---|---|---|" + "---|" * len(ms))
    rank = lambda kv: -sum(v.count("y") for v in kv[1].values())
    for (judge, f), r in sorted(cells.items(), key=rank):
        out.append(f"| {judge} | `{f}` | {titles.get(f, '').replace('|', '/')} | " + " | ".join(r.get(m, "·") for m in ms) + " |")
    return "\n".join(out)


if __name__ == "__main__" and "--judging" in sys.argv:
    print(judging_report())
