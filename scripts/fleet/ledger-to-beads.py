#!/usr/bin/env python3
"""One-way mirror of a fleet LEDGER.yaml into a beads (`bd`) store.

    scripts/fleet/ledger-to-beads.py [--ledger LEDGER.yaml] [--store DIR] [--bd BD] [--jsonl-only]
                                     [--compare-guard fleet_guard.py] [--fields]

The ledger stays the source of truth (commander-owned; this script only reads it). Every run
rebuilds the store from scratch in a sibling directory and swaps it in, so the mirror is exact
(an upsert import would keep dependencies and labels the ledger has since dropped).

Mapping (ledger field -> beads home; `metadata.ledger` always carries the item verbatim):

  id              -> id (explicit; the store prefix is the ids' common prefix, e.g. X)
  title           -> title
  kind            -> issue_type (lane, commander-task, review, evidence-prep registered as
                     types.custom; decision is built in) + label kind:<kind>
  status          -> status: ready->open, in-progress->in_progress, blocked->blocked,
                     done->closed, queued and dropped -> custom statuses of category frozen
  depends         -> blocks dependencies; a dependency on an id the ledger lacks becomes a stub
                     bead in custom status `missing` (frozen) with label ledger-missing
  owner           -> assignee
  note            -> notes
  brief           -> spec_id
  providers       -> labels provider:<p>
  glm-executable  -> label glm-executable (when true)
  auto-start      -> label auto-start (when true)
  decision-rights -> label decision-rights:<who>
  lane            -> label lane:<lane>
  anything else   -> metadata.ledger only (merged, sealed-at, provenance, quota-*, ready-when,
                     integrity, ...); top-level `windows:` is not mirrored

With this mapping `bd ready` equals the fleet guard's is_ready() set: open (= ledger ready) and
every blocker closed (= ledger done); frozen statuses never count as resolved. --compare-guard
loads the guard read-only and checks that equality plus its two provider-filtered views.

Run bd with BD_DISABLE_METRICS=1: bd sends anonymous usage metrics by default.
"""
import argparse
import importlib.util
import json
import os
import shutil
import subprocess
import sys

import yaml

DEFAULT_LEDGER = "docs/phase6/natural-agent/LEDGER.yaml"
DEFAULT_STORE = os.path.expanduser("~/fleetpilot/data/ledger-mirror")

STATUS = {"ready": "open", "in-progress": "in_progress", "blocked": "blocked", "done": "closed",
          "queued": "queued", "dropped": "dropped"}
CUSTOM_STATUSES = "queued:frozen,dropped:frozen,missing:frozen"
BUILTIN_TYPES = {"decision", "task", "bug", "feature", "chore", "epic", "spike", "story", "milestone"}
CUSTOM_TYPES = ["lane", "commander-task", "review", "evidence-prep"]
LABEL_FIELDS = ("providers", "glm-executable", "auto-start", "decision-rights", "lane")
NATIVE_FIELDS = ("id", "title", "kind", "status", "depends", "owner", "note", "brief")


class LedgerError(Exception):
    pass


def load_ledger(path):
    with open(path) as f:
        d = yaml.safe_load(f)
    if not isinstance(d, dict) or not isinstance(d.get("items"), list):
        raise LedgerError(f"{path}: no items list")
    return d


def items_of(d):
    """Same filter as the guard's ledger_items()."""
    return [i for i in d["items"] if isinstance(i, dict) and i.get("id")]


def prefix_of(ids):
    prefixes = {str(i).rsplit("-", 1)[0] for i in ids if "-" in str(i)}
    if len(prefixes) != 1:
        raise LedgerError(f"ids must share one '<prefix>-' (got {sorted(prefixes) or 'none'})")
    return prefixes.pop()


def labels_for(item):
    out = [f"kind:{item['kind']}"] if item.get("kind") else []
    out += [f"provider:{p}" for p in item.get("providers") or []]
    if item.get("glm-executable") is True:
        out.append("glm-executable")
    if item.get("auto-start") is True:
        out.append("auto-start")
    if item.get("decision-rights"):
        out.append(f"decision-rights:{item['decision-rights']}")
    if item.get("lane"):
        out.append(f"lane:{item['lane']}")
    return out


def to_records(d):
    """Ledger dict -> list of `bd import` JSONL records (stubs for missing dependencies last)."""
    items = items_of(d)
    known = {str(i["id"]) for i in items}
    records, missing = [], []
    for item in items:
        iid = str(item["id"])
        status = item.get("status")
        if status not in STATUS:
            raise LedgerError(f"{iid}: unknown status {status!r}")
        kind = item.get("kind") or "task"
        rec = {
            "id": iid,
            "title": str(item.get("title") or iid),
            "status": STATUS[status],
            "issue_type": kind,
            "priority": 2,
            "labels": labels_for(item),
            "metadata": {"ledger": json.loads(json.dumps(item, default=str))},
            "description": yaml.safe_dump(item, sort_keys=False, allow_unicode=True, width=110),
        }
        if item.get("owner"):
            rec["assignee"] = str(item["owner"])
        if item.get("note"):
            rec["notes"] = str(item["note"])
        if item.get("brief"):
            rec["spec_id"] = str(item["brief"])
        deps = [str(x) for x in item.get("depends") or []]
        if deps:
            rec["dependencies"] = [{"issue_id": iid, "depends_on_id": x, "type": "blocks"} for x in deps]
        missing += [x for x in deps if x not in known and x not in missing]
        records.append(rec)
    for x in missing:
        records.append({"id": x, "title": f"{x} (referenced by the ledger, not in it)", "status": "missing",
                        "issue_type": "task", "priority": 2, "labels": ["ledger-missing"]})
    return records


def custom_types(records):
    return sorted(set(CUSTOM_TYPES) | {r["issue_type"] for r in records} - BUILTIN_TYPES)


def carried_fields(d):
    """{ledger field: where it lives in beads} for every field the ledger actually uses."""
    used = []
    for item in items_of(d):
        used += [k for k in item if k not in used]
    home = {}
    for k in used:
        if k in NATIVE_FIELDS:
            home[k] = "native"
        elif k in LABEL_FIELDS:
            home[k] = "label (+ metadata.ledger)"
        else:
            home[k] = "metadata.ledger only"
    for k in d:
        if k != "items":
            home[f"(top-level) {k}"] = "not mirrored"
    return home


def bd(bd_bin, cwd, *args, stdin=None):
    env = dict(os.environ, BD_DISABLE_METRICS="1", DO_NOT_TRACK="1", BD_NON_INTERACTIVE="1")
    r = subprocess.run([bd_bin, *args], cwd=cwd, input=stdin, capture_output=True, text=True, env=env)
    if r.returncode != 0:
        raise RuntimeError(f"bd {' '.join(args)} failed ({r.returncode}): {r.stderr.strip()[-500:]}")
    return r.stdout


def build_store(records, prefix, store, bd_bin):
    store = os.path.abspath(store)
    if os.path.exists(store) and not os.path.isdir(os.path.join(store, ".beads")):
        raise RuntimeError(f"{store} exists and is not a beads store; refusing to replace it")
    new, old = f"{store}.new-{os.getpid()}", f"{store}.old-{os.getpid()}"
    os.makedirs(new)
    try:
        bd(bd_bin, new, "init", "-q", "--non-interactive", "-p", prefix)
        bd(bd_bin, new, "config", "set", "status.custom", CUSTOM_STATUSES)
        bd(bd_bin, new, "config", "set", "types.custom", ",".join(custom_types(records)))
        jsonl = "".join(json.dumps(r) + "\n" for r in records)
        bd(bd_bin, new, "import", "-", stdin=jsonl)
    except Exception:
        shutil.rmtree(new, ignore_errors=True)
        raise
    if os.path.exists(store):
        os.rename(store, old)
    os.rename(new, store)
    if os.path.isdir(old):
        shutil.rmtree(old)
    return store


def bd_ready(store, bd_bin, *filters):
    out = bd(bd_bin, store, "ready", "--json", "-n", "0", *filters)
    return sorted(i["id"] for i in json.loads(out or "[]"))


def load_guard(path):
    spec = importlib.util.spec_from_file_location("fleet_guard_ro", path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def compare_guard(d, store, bd_bin, guard_path):
    """[(view, guard ids, bd ids, bd command)] for the guard's three ready views."""
    g = load_guard(guard_path)
    by = g._by_id(d)
    guard_ready = sorted(str(i["id"]) for i in g.ledger_items(d) if g.is_ready(i, by))
    guard_claude = sorted(str(i["id"]) for i in g.ledger_claude_ready(d))
    guard_offered = sorted(str(i["id"]) for i in g.ledger_offered(d))
    providers = sorted({p for i in items_of(d) for p in i.get("providers") or []})
    others = [f"provider:{p}" for p in providers if p != "claude"]
    claude_filters = ["--label", "provider:claude"] + (["--exclude-label", ",".join(others)] if others else [])
    bd_claude = bd_ready(store, bd_bin, *claude_filters)
    offered_filters = ["--label", "glm-executable,provider:glm", "--unassigned", "--exclude-type", "decision"]
    bd_offered = [x for x in bd_ready(store, bd_bin, *offered_filters)
                  if (by.get(x) or {}).get("kind") in g.OFFER_KINDS]
    return [
        ("is_ready (topo-ready)", guard_ready, bd_ready(store, bd_bin), "bd ready"),
        ("ledger_claude_ready", guard_claude, bd_claude, "bd ready " + " ".join(claude_filters)),
        ("ledger_offered", guard_offered, sorted(bd_offered),
         "bd ready " + " ".join(offered_filters) + " (then kind in OFFER_KINDS)"),
    ]


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--ledger", default=DEFAULT_LEDGER)
    ap.add_argument("--store", default=DEFAULT_STORE)
    ap.add_argument("--bd", default=shutil.which("bd") or os.path.expanduser("~/fleetpilot/bin/bd"))
    ap.add_argument("--jsonl-only", action="store_true", help="print the import records; touch no store")
    ap.add_argument("--fields", action="store_true", help="print where each ledger field lives in beads")
    ap.add_argument("--compare-guard", metavar="FLEET_GUARD_PY",
                    help="check bd ready against the guard's ready views (exit 1 on mismatch)")
    a = ap.parse_args(argv)

    d = load_ledger(a.ledger)
    records = to_records(d)
    if a.jsonl_only:
        sys.stdout.write("".join(json.dumps(r) + "\n" for r in records))
        return 0
    prefix = prefix_of(r["id"] for r in records)
    store = build_store(records, prefix, a.store, a.bd)
    print(f"mirrored {len(items_of(d))} ledger items ({len(records)} beads) from {a.ledger} into {store}")
    if a.fields:
        for k, v in carried_fields(d).items():
            print(f"  {k:24} {v}")
    if a.compare_guard:
        rc = 0
        for view, gids, bids, cmd in compare_guard(d, store, a.bd, a.compare_guard):
            ok = gids == bids
            rc |= not ok
            print(f"  {'OK  ' if ok else 'DIFF'} {view:24} guard={gids} bd={bids}   [{cmd}]")
        return rc
    return 0


if __name__ == "__main__":
    sys.exit(main())
