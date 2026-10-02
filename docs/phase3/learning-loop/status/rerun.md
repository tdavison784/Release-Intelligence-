# Lane `rerun` status: DONE

Branch `p3ll/rerun`. Writes only under `docs/rerun/` + this file. Full results: `docs/rerun/REPORT.md`.

## Done
- Live `ri check` x28 (same release sets, fresh per-product state, `-refresh`), `compare.py`, `report_tables.py`.
- Discovery x25 pinned to baseline refs + old-binary controls (isolate network vs tool evolution).
- `ri drift` x28 (vs baseline, vs live, newest-3), `ri stats` over live checks, `ri eval` live (read-only).
- 129 research-claim probes + content/registry-auth probes.
- REPORT.md: classification (a sandbox / b genuine / c upstream changed / d defect / e definition changed / t throttling), recommendations.

## Headline
- 212 differences are sandbox artifacts (unverifiable/covered -> pass). Real defects: ingress-nginx controller-chroot v1.10.0
  absent upstream (D1); argo-cd v3.4.0 is a tag, not a release (D2, FINDINGS claim wrong); 429 reported as unavailable (D3);
  baselines stale vs definitions (D4); eval gates applicabilityAccuracy 0.495 and falseActionRate 0.059 fail (D5, not network).

## Incident
ENOSPC mid-run (parallel discovery/drift clones). That batch was discarded and re-run serially; nothing disk-failed is counted as unavailable.
Stale `raw/*.disc.started|finished` markers remain (rm blocked by a safety check).

## Decisions
- argo-cd 3.4.0 -> 3.4.1 for the live check. No product definitions or originals touched.

## Files outside ownership
None. No Go changes; `go build ./... && go vet ./... && go test ./...` pass.

## Open
kube-prometheus-stack old-binary control; drift vs an older baseline; body-level claim verification.
