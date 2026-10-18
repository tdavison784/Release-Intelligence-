# Lane ws6 status

Branch `p6na/ws6`. Ledger X-0001. Brief `briefs/ws6.md`.

## Done
- `scripts/phase6/materialise.sh`: raw RI sources per case into `~/ri-p6/evidence/<case-id>` (digests = arm-B bundle).
- `scripts/phase6/build-workspace.sh`: workspaces in `~/ri-p6/ws/<case-id>` (reproducible MANIFEST digest).
- `scripts/phase6/run-natural.sh` + `session.py`: runner with isolation, budget stop, meta.json.
- `scripts/phase6/canary.sh`: canary PASS at `runs/canary/` (12/12 checks).

## In progress (next step)
- extract.sh, guided.sh, smoke run on shop-de, EVIDENCE.md per case, HARNESS.md.
