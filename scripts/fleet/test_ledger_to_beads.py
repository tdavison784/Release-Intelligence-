"""Tests for ledger-to-beads.py.  Run: python3 -m pytest scripts/fleet

The mapping tests need nothing. The store tests need `bd` (on PATH or ~/fleetpilot/bin) and skip
without it; the guard-equivalence tests also need ~/.claude/fleet-guard/fleet_guard.py (loaded
read-only) and skip without it.
"""
import copy
import importlib.util
import os
import shutil

import pytest
import yaml

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))
spec = importlib.util.spec_from_file_location("ledger_to_beads", os.path.join(HERE, "ledger-to-beads.py"))
l2b = importlib.util.module_from_spec(spec)
spec.loader.exec_module(l2b)

BD = shutil.which("bd") or os.path.expanduser("~/fleetpilot/bin/bd")
GUARD = os.path.expanduser("~/.claude/fleet-guard/fleet_guard.py")
needs_bd = pytest.mark.skipif(not os.access(BD, os.X_OK), reason="bd not installed")
needs_guard = pytest.mark.skipif(not os.path.isfile(GUARD), reason="fleet guard not present")

# Every readiness edge the guard distinguishes, with the ready set it must produce.
SYNTHETIC = {"items": [
    {"id": "Z-0001", "title": "done", "kind": "lane", "status": "done", "providers": ["claude"]},
    {"id": "Z-0002", "title": "ready, dep done", "kind": "lane", "status": "ready", "depends": ["Z-0001"],
     "providers": ["claude", "glm"], "glm-executable": True, "auto-start": True, "brief": "briefs/z.md"},
    {"id": "Z-0003", "title": "queued", "kind": "commander-task", "status": "queued", "providers": ["claude"]},
    {"id": "Z-0004", "title": "ready, dep queued", "kind": "review", "status": "ready", "depends": ["Z-0003"],
     "providers": ["claude", "glm"], "glm-executable": True},
    {"id": "Z-0005", "title": "dropped", "kind": "lane", "status": "dropped"},
    {"id": "Z-0006", "title": "ready, dep dropped", "kind": "lane", "status": "ready", "depends": ["Z-0005"]},
    {"id": "Z-0007", "title": "ready, dep missing", "kind": "lane", "status": "ready", "depends": ["Z-9999"]},
    {"id": "Z-0008", "title": "in progress", "kind": "lane", "status": "in-progress", "owner": "ri-z"},
    {"id": "Z-0009", "title": "ready, dep in progress", "kind": "lane", "status": "ready", "depends": ["Z-0008"]},
    {"id": "Z-0010", "title": "ready decision", "kind": "decision", "status": "ready", "decision-rights": "user",
     "providers": ["claude"]},
    {"id": "Z-0011", "title": "ready, owned", "kind": "commander-task", "status": "ready", "owner": "glm-commander",
     "providers": ["claude", "glm"], "glm-executable": True},
    {"id": "Z-0012", "title": "ready evidence-prep", "kind": "evidence-prep", "status": "ready",
     "providers": ["claude", "glm"], "glm-executable": True, "note": "offered to GLM"},
    {"id": "Z-0013", "title": "blocked", "kind": "lane", "status": "blocked"},
    {"id": "Z-0014", "title": "ready, deps done+blocked", "kind": "lane", "status": "ready",
     "depends": ["Z-0001", "Z-0013"]},
    {"title": "no id: ignored, as by the guard"},
]}
SYNTHETIC_READY = ["Z-0002", "Z-0010", "Z-0011", "Z-0012"]


def by_id(records):
    return {r["id"]: r for r in records}


def test_status_and_native_fields():
    r = by_id(l2b.to_records(SYNTHETIC))
    assert [r[i]["status"] for i in ("Z-0001", "Z-0002", "Z-0003", "Z-0005", "Z-0008", "Z-0013")] == \
        ["closed", "open", "queued", "dropped", "in_progress", "blocked"]
    assert r["Z-0002"]["issue_type"] == "lane" and r["Z-0010"]["issue_type"] == "decision"
    assert r["Z-0002"]["dependencies"] == [{"issue_id": "Z-0002", "depends_on_id": "Z-0001", "type": "blocks"}]
    assert r["Z-0002"]["spec_id"] == "briefs/z.md"
    assert r["Z-0008"]["assignee"] == "ri-z"
    assert r["Z-0012"]["notes"] == "offered to GLM"


def test_labels_and_verbatim_metadata():
    r = by_id(l2b.to_records(SYNTHETIC))
    assert r["Z-0002"]["labels"] == ["kind:lane", "provider:claude", "provider:glm", "glm-executable", "auto-start"]
    assert r["Z-0010"]["labels"] == ["kind:decision", "provider:claude", "decision-rights:user"]
    for item in l2b.items_of(SYNTHETIC):
        assert r[item["id"]]["metadata"]["ledger"] == item


def test_missing_dependency_becomes_frozen_stub():
    r = by_id(l2b.to_records(SYNTHETIC))
    assert r["Z-9999"]["status"] == "missing" and r["Z-9999"]["labels"] == ["ledger-missing"]
    assert len(r) == 15  # 14 items with ids + 1 stub


def test_unknown_status_and_mixed_prefix_fail_closed():
    with pytest.raises(l2b.LedgerError):
        l2b.to_records({"items": [{"id": "Z-1", "status": "doing"}]})
    with pytest.raises(l2b.LedgerError):
        l2b.prefix_of(["W-0001", "X-0001"])
    assert l2b.prefix_of(["W-0001", "W-0020a"]) == "W"


def test_carried_fields():
    home = l2b.carried_fields({"version": 1, "items": [{"id": "Z-1", "status": "ready", "providers": ["glm"],
                                                        "quota-estimate": "~1M"}]})
    assert home == {"id": "native", "status": "native", "providers": "label (+ metadata.ledger)",
                    "quota-estimate": "metadata.ledger only", "(top-level) version": "not mirrored"}


@needs_bd
def test_bd_ready_synthetic(tmp_path):
    records = l2b.to_records(SYNTHETIC)
    store = l2b.build_store(records, "Z", str(tmp_path / "store"), BD)
    assert l2b.bd_ready(store, BD) == SYNTHETIC_READY


@needs_bd
def test_rebuild_drops_stale_state(tmp_path):
    store = str(tmp_path / "store")
    l2b.build_store(l2b.to_records(SYNTHETIC), "Z", store, BD)
    d = copy.deepcopy(SYNTHETIC)
    d["items"][3]["depends"] = []  # Z-0004 loses its queued dependency -> ready
    l2b.build_store(l2b.to_records(d), "Z", store, BD)
    assert l2b.bd_ready(store, BD) == sorted(SYNTHETIC_READY + ["Z-0004"])
    assert sorted(os.listdir(tmp_path)) == ["store"]


def test_refuses_to_replace_a_non_store(tmp_path):
    (tmp_path / "precious").mkdir()
    with pytest.raises(RuntimeError):
        l2b.build_store([], "Z", str(tmp_path / "precious"), BD)


def phase6_whatif():
    d = l2b.load_ledger(os.path.join(REPO, "docs/phase6/natural-agent/LEDGER.yaml"))
    for item in d["items"]:  # X-0001 done, X-0002 done, X-0003 ready: the next step becomes ready
        if item["id"] in ("X-0001", "X-0002"):
            item["status"] = "done"
        elif item["id"] == "X-0003":
            item["status"] = "ready"
    return d


@needs_bd
@needs_guard
@pytest.mark.parametrize("ledger", ["synthetic", "phase5", "phase6", "phase6-whatif"])
def test_bd_ready_equals_guard(tmp_path, ledger):
    d = {"synthetic": lambda: SYNTHETIC,
         "phase5": lambda: l2b.load_ledger(os.path.join(REPO, "docs/phase5/learning-loop/LEDGER.yaml")),
         "phase6": lambda: l2b.load_ledger(os.path.join(REPO, "docs/phase6/natural-agent/LEDGER.yaml")),
         "phase6-whatif": phase6_whatif}[ledger]()
    records = l2b.to_records(d)
    store = l2b.build_store(records, l2b.prefix_of(r["id"] for r in records), str(tmp_path / "s"), BD)
    for view, guard_ids, bd_ids, _ in l2b.compare_guard(d, store, BD, GUARD):
        assert bd_ids == guard_ids, view
