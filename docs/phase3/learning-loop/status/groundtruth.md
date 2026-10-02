# Lane `groundtruth` — status

**State: in progress** (GLM-5.3 handoff window; see "GLM handoff log").

## Done
- Format (step 1): `internal/eval/labels.go` (new) — `semantics` per expected item, `exposure` / `overlap` /
  `environmentEvidence` per link, `environment.undecidedImpact`; strict decoding into the domain types;
  validation. `Case.TransferOf` (transfer environments, G12) in `case.go` + `transfer.go`. Tests:
  `labels_test.go`. Docs: `eval/FORMAT.md` ("Semantic labels", "Transfer environments").
- Labels + corrections (steps 2–3, commit 4ee35f7): `semantics` + `exposure` + `environmentEvidence` on
  the 21 existing links, upstream-grounded corrections recorded in `eval/CHANGELOG.md` (pre/post + why).
  D1 left as-is per brief.
- Sub-briefs for hand-off authoring: `briefs/groundtruth-newcase.md`, `briefs/groundtruth-transfer.md`.
- New cases (step 5): seven committed — crossplane-1.20-2.0, external-secrets-0.15-0.16, flux-2.6-2.7,
  kyverno-1.12-1.13, loki-2.9-3.0, prometheus-operator-0.85-0.86, traefik-2.11-3.0. Each: 8–10 items
  with full semantics, realistic fixture + inventory.yaml, ≥5 expectedImpact links incl. ≥2 not-affected,
  undecided links where honest. Authored blind by sub-agents per the new-case sub-brief; dataset tests
  green before commit. G22 categories covered: prose-only defaults, feature gates, cross-product version
  dependencies, RBAC, migrations, behaviour changes, deprecations, required new config, compatibility in
  prose.

## Next
- Transfer environments (step 4): four in flight via blind sub-agents per the transfer sub-brief, bases
  cert-manager-1.16-1.17, cilium-1.16-1.17, karpenter-0.37.8-1.0.0, strimzi-0.45-0.46; commit after
  review + validation.
- One `ri eval` run (step 6) after all expectations are committed; record first numbers here, no tuning.

## Decisions
- Transfer cases as sibling ids `<base>--<suffix>` with `transferOf` (least invasive; chosen in step 1).
- New cases committed individually (one commit per case) so any later correction can be reverted alone.

## Files touched outside ownership
- None beyond the brief's file list.

## Open questions
- None blocking.

## Test status
- `go build ./...`, `go vet ./...`, `go test ./...` green at every commit in this window (last check:
  after committing the seven new cases).

## GLM handoff log
(GLM-5.3 continuing the lane during the Claude agent's usage-limit window, 2026-10-01.)
- Took over with steps 2–3 already committed (4ee35f7) and seven authored-but-uncommitted new case
  directories on disk. Reviewed them against `briefs/groundtruth-newcase.md` (structure, link mix,
  evidence, notes), ran `go test ./internal/eval` (dataset load + full package) — green — and committed
  them one per commit, plus the transfer sub-brief file that was untracked.
- Spawned four blind sub-agents (transfer sub-brief) for the G12 transfer environments. Uncertain for
  the returning Claude agent: whether more than four transfers are wanted (brief says ≥4; I stopped at
  four to fit the window), and the new cases were reviewed by me at structural + notes level, not
  re-researched quote-by-quote against upstream (their sub-agent reports claimed validation-blindness
  and the loader checks line ranges; a full re-verification pass may be worthwhile later).
