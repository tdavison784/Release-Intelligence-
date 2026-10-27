# Lane join5: status (Milestone 4 fix; ledger D-0028)

Branch `p6di/join5` (from `p6-decision-intel` @ fac5f22c). Brief: `briefs/join5.md`. Two defects: D1 (the knowledge
join evaluates source files, not rendered objects) and D2 (observed role-membership defaults not applied in the pin
scope). Design, tests, regression and known limits: `M4-IMPLEMENTATION.md`, lane `join5`.

## Done

1. **D2 root cause found and fixed** (65472529). `app.LoadKnowledge` (the reader `ri investigate`, `ri impact`,
   `ri eval` use) never loaded `proof` records, so `impact.VerifyFacts` ignored every observed (PD-5) and
   role-membership (PD-6) default ("proof … does not resolve"); the pin scope and the join get their facts from that
   loader. Fix: load proofs. Also the investigation CLI printed none of the loader's warnings (now printed; an
   unreadable `-knowledge` is an error), and an ignored default is reported as "fact default ignored".
2. **D1: effective object view for the impact join** (85653d7f, 858d0acb): `env.Effective` (rendered objects of the
   render targets over the files they consumed; evidence mapped to the producing file and line, the overlay patch
   first, via `render.KustomizeLayers`; unrendered declarations and render gaps decide nothing), built in
   `app.ImpactRun`, read by the knowledge join and the schema-attribute joins (`impact.Input.Objects`).
3. **D1: validators** (5573aa86): `SchemaCompat` reads the effective view (`schema-compat@v3`, `schema:unrendered`);
   pins on rendered objects cite the producing file and line.
4. **Tests**: D2 through the real loader; D1 for overlay sets / overlay removes or overrides (fail-open direction) /
   no render (unknown) / render gaps / the knowledge and attribute joins / the pin scope and schema check / a real
   `kubectl kustomize` render. Membership in the pin scope, no default without a confirmed proof, contradiction
   blocks: verify4's `TestPinScopeRoleMembershipDefault` plus the new loader tests.
5. **Regression** (details in M4-IMPLEMENTATION.md): `m4rescore` 1004/1004 identical; join on the 8 investigation
   cases with K0: 827/828 changes identical; the one difference is on `shop-nl` (not a development case):
   `impact:knowledge-exposed` → `impact:knowledge-overlap` (see Open questions). 241/241 checked-in facts verify with
   and without proofs loaded; the one stored proof validates.
6. **Docs**: `docs/IMPACT.md` (objects as deployed; the effective view in the resource store), `docs/INVESTIGATE.md`
   (schema check view, pin origin, producer version), `M4-IMPLEMENTATION.md` (lane section).

## In progress (exact next step)

- Nothing: lane done; awaiting the commander's merge and part B with K1.

## Decisions

- **Chart objects are in the view** when their render is complete (the validators' view includes them; the brief asks
  for the same objects). Consequence: the shop-nl difference above. One place to change if the commander disagrees:
  `app.renderedObjectsOf`.
- **A render gap never costs an exposed finding**: under a gap only a would-be false becomes unknown (rule 1 of the
  evaluator; `m4rescore` counts a lost affected as looser). A true resting on absence is not negated under a gap.
- **Unrendered declarations are unknown, not dropped**: a file consumed by an unavailable render stays in the view so
  `resource` scopes over its kind are unknown rather than "no object" (false).
- **Without `-render`**, kustomize targets are detected (not rendered) and their files are unrendered; chart objects
  stay outside the view as before (no new gaps for `ri impact`/`ri eval` without renders). No deterministic eval case
  (`eval/cases`) has a kustomization, so the frozen deterministic gates are untouched.
- **`crd:fields-removed` stays on the file view**: it reads every document including patch documents, which can only
  over-report; rebuilding its GVK-usage coverage on rendered objects is out of scope.
- **`schema-compat` producer bumped to v3** (it judges different objects); override inspection keeps v3 (only the
  hit's text gains the origin).

## Files touched outside ownership

- `internal/investigate/verify/probe_test.go` (test helper keeps the store directory).
- `internal/render/kustomize.go` (walk shared by `KustomizeInputs`, new `KustomizeLayers`; inputs unchanged).
- `internal/investigate/validate/*` (schema view, pin origin, `kustomizeLayers`), `cmd/ri/investigate.go`.

## Contract changes (additive; CONTRACT-CHANGE(join5))

- `env.Effective`, `env.RenderedObjects`, `env.EffectiveProducer`, `Resource.Render`, `Resource.Unrendered`,
  `Environment.EffectiveView`, `Environment.RenderGaps` (both `omitempty`).
- `impact.Input.Objects`; `render.KustomizeLayers`.
- No `internal/domain` change (no schema regeneration needed).

## Integrity note

- While locating K0 I ran a broad `grep` over `docs/phase5/learning-loop/*.md` that matched three lines of
  `EVAL-REPORT.md` (a file FLEET.md says never to open); the file itself was not opened. The lines name the Phase 5
  K0 recipe and describe that K0's only pin rules require the field on every Certificate. All code was written before
  this and is generic; nothing was tuned on it.

## Open questions

- For the commander: accept the shop-nl transition (a fact's exposure decided by the rendered chart's controller args
  instead of absence in files that never contain the controller), or keep chart objects out of the join's view?
- Follow-up candidate: the relations graph shows version-independent overlay objects as their base declares them.

## Test status

- `go build ./... && go vet ./... && go test ./...` green; `m4rescore` green.
