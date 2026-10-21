#!/usr/bin/env python3
"""Recover a run's assessment from its own denied Write (harness defect: a persistent `cd` moved the session cwd,
so the relative allow rule Write(./scratch/**) no longer matched). Copies the content of the session's last
denied Write to scratch/assessment.md, verbatim, and records the recovery in meta.json. Usage: <run-dir>"""
import json, sys, pathlib, hashlib
run = pathlib.Path(sys.argv[1])
last = None
for line in open(run / "transcript.jsonl"):
    j = json.loads(line)
    if j.get("type") == "result":
        for d in j.get("permission_denials") or []:
            inp = d.get("tool_input", {})
            if d.get("tool_name") == "Write" and inp.get("file_path", "").endswith("/ws/scratch/assessment.md"):
                last = inp["content"]
if last is None:
    sys.exit("no denied Write of scratch/assessment.md in the result event")
for p in (run / "scratch" / "assessment.md", run / "assessment.md"):
    p.parent.mkdir(exist_ok=True)
    p.write_text(last)
m = json.loads((run / "meta.json").read_text())
m["assessmentWritten"] = False
m["assessmentRecovered"] = {
    "source": "content of the session's denied Write to ws/scratch/assessment.md (result event permission_denials)",
    "cause": "an earlier allowed `cd <ws>/rendered` persisted the session cwd, so Write(./scratch/**) resolved to rendered/scratch",
    "sha256": hashlib.sha256(last.encode()).hexdigest(),
}
(run / "meta.json").write_text(json.dumps(m, indent=2) + "\n")
print("recovered", len(last), "chars ->", run / "assessment.md")
