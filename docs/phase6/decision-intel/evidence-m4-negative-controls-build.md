# M4 negative-control knowledge stores: build report

Built 2026-10-28. All four stores were built and all four load. No repository file was changed or added, and nothing
was committed. `git status` in the worktree is clean, HEAD `e4725b7e`.

## How they were built

- Each store started as `cp -Rp K1 Kneg-<variant>`. `diff -rq` against K1 showed an identical copy before any change.
- Every write went through RI's own code: `knowledge.NewFileStore(dir).Put` (`Validate` plus `checkFact`/`checkProof`)
  or `knowledge.NewQueue(store, now).Decide`, the write path `ri knowledge decide` uses. The CLI exposes no
  `-supersedes` flag, so I called the Go API. Gate checks used `knowledge.PromotionGate` and `knowledge.HypothesisSource`.
- I hand-edited no JSON. FileStore refuses to overwrite immutable fields. Where it did, I recorded the refusal, removed
  the one file with `os.Remove` and wrote the replacement record with `FileStore.Put`, which re-validates it and
  re-proves its basis.
- **Load check:** `app.LoadKnowledge(dir)` is the loader `ri eval -investigations -knowledge` uses. I also ran
  `ri knowledge metrics -dir <store> -product cert-manager -o json` (a FileStore load). That `ri` binary was built
  with `go build ./cmd/ri` from the worktree. All five runs, K1 included, exited 0.
- **K1 baseline:** 1254 candidates, 4123 proposals, 4057 validations, 2934 review items, 3494 decisions, 5 proofs,
  241 facts. All 241 facts were kept, with **1 warning**, and that warning was already there:
  `K1/ingress-nginx/failures/f7f7afa86088.json: invalid  record, ignored: knowledge record: schemaVersion = "", want "ri.dev/knowledge/v1alpha1"`.
  Each Kneg store gives exactly this one warning, under its own path. **No store refuses a fact, and none ignores a
  default.**
- **Synthetic human decisions** (contradicted and inapplicable):
  - reviewer `m4-negative-control`, reviewerKind `human`;
  - startedAt 2026-10-28T09:00:00Z, decidedAt 2026-10-28T09:01:00Z;
  - each reason begins "M4 negative control (synthetic)".
  - Both stores contain decision id `rd-f4ab657a81a9` (same item, reviewer and time) with **different content**.
    Never merge these two stores.
- The builder program (`negbuild/`, with a go.work over the repository) and its `ri` binary are deleted. The stores
  remain.

Tree digests below were computed by re-implementing `knowledgeTreeDigest` in Python. `ri` has not confirmed them.

| store | files | tree digest |
|---|---|---|
| K1 (unchanged) | 16109 | sha256:a028f48aa8cc8e6b87671132de48b4f321ff599f2c6b96d60c7505957fed76b4 |
| Kneg-nodefault | 16109 | sha256:64fd3498c7ca08e43bc052a5fd7c24c24e5660ff99ea2e7f3535955380237c97 |
| Kneg-expired | 16109 | sha256:b62b1f49e638950efe1b3d0e91d8b2a19f1ae601f9ec12d52bc28f0c4c5e9f59 |
| Kneg-contradicted | 16111 | sha256:8b0e86872e4f7f4c1ece112a9c8ddafdb793a574e93498fe1bb02b0a47069702 |
| Kneg-inapplicable | 16111 | sha256:cdb2e5b64b70d90e8821ec296aa38f50be53bd31d52f04087ce0453f7490fe7d |

## 1. Kneg-nodefault: built

Path: `/Users/tommydavison/.claude/jobs/3f39867a/tmp/m4/Kneg-nodefault`

**What changed:** one file, `cert-manager/1.18.0/facts/vf-4d9b756f1343.json`. Its only change is that
`fact.roleMembershipDefaults` was removed. That field held one entry: `spec.isCA`, role `ca-trust`, roleValue `true`,
outsideValue `false`, observed, proof `pf-ae4237aaecac`. The fact id is unchanged, because a default is not part of
the id.

**How:**
- `FileStore.Put` over the existing record was **refused**: `knowledge: conflicting record: fact vf-4d9b756f1343
  already exists with different content (only status, anchors, candidates, verification upgrades, consequence prose,
  consensusAction and auditedBy may change)`.
- I removed the file and wrote the fact again with `FileStore.Put`, which accepted it.

**Load:**
- 241 facts kept, 1 pre-existing warning.
- `vf-4d9b756f1343` is active, human-verified and kept, with 0 role defaults.
- `vf-9b9cfa5c9f5f` is still superseded.

**Note:** proof `pf-ae4237aaecac` is unchanged and still records the default. The loader reads defaults only from the
fact, so it does not restore it. Through the decision path, a fact without this default would only come from a
promotion whose proof recorded none.

## 2. Kneg-expired: built (the RI decision path refuses it; built with FileStore)

Path: `/Users/tommydavison/.claude/jobs/3f39867a/tmp/m4/Kneg-expired`

**What changed:**
- **Proof `pf-ae4237aaecac`** (`cert-manager/1.18.0/proofs/`): `versionBounds` went from `(v1.17.2, v1.18.0]` to
  `(v1.16.0, v1.17.0]`. Nothing else changed. The id stays `pf-ae4237aaecac`, because `ProofRecordID` does not hash
  the bounds.
- **The fact moved to release 1.17.0.** I removed `cert-manager/1.18.0/facts/vf-4d9b756f1343.json` and added
  `cert-manager/1.17.0/facts/vf-8b678b57d8d6.json`:
  - `release` 1.18.0 → 1.17.0;
  - all 3 `anchors[].release` 1.18.0 → 1.17.0, because `Validate` requires anchors to carry the fact's release;
  - id re-derived by `VerifiedFactID` as `vf-8b678b57d8d6`.
  - The assertion, verification basis, evidence, `supersedes [vf-9b9cfa5c9f5f]` and the role default are identical
    to K1's fact.

**Refusals recorded:**
- `knowledge.PromotionGate` on item `ri-3110ace7e891` with the re-bounded proof: **gate closed**, `the item's release
  1.18.0 lies outside the proof's bounds (v1.16.0, v1.17.0]`. Through the decision path RI will not promote this
  rule with these bounds.
- `FileStore.Put` of the re-bounded proof was **refused**: `knowledge: conflicting record: proof pf-ae4237aaecac
  already exists with different content (only nothing: the kind is immutable may change)`. I removed the file and
  re-put it, which was accepted.
- The decision path cannot mint a 1.17.0 fact from candidate `sc-0564d2273368`, which is release 1.18.0
  (`buildFact` takes the candidate's release). So I wrote the fact only with `FileStore.Put`, which accepted it.

**Load:**
- 241 facts kept, 1 pre-existing warning.
- `vf-8b678b57d8d6` (release 1.17.0) is active, human-verified and kept. Its role default is stored and still
  **usable** (1/1), because `RoleMembershipEstablished` does not look at proof bounds.
- `vf-9b9cfa5c9f5f` is still superseded.

**Expected effect:** 1.17.0 is not in (1.17.2, 1.18.0]. So `impact.factInEdge` and the validator's
`outsideTransition` should leave this fact out of the v1.17.2 → v1.18.0 upgrade.

**Inconsistencies the loader does not catch:**
- Decision `rd-49bea9629f0a.resultingFact` still names `vf-4d9b756f1343`, which is no longer in the store.
- The candidate, the review item and `proof.release` all still say 1.18.0.

## 3. Kneg-contradicted: built (the gate refuses it; the queue accepts it through an ordinary item)

Path: `/Users/tommydavison/.claude/jobs/3f39867a/tmp/m4/Kneg-contradicted`

**What changed:** K1 plus two new files. No existing file changed.

**`cert-manager/1.18.0/facts/vf-45b922ec1cc6.json`**, the second fact: active, human-verified, release 1.18.0.
- Subject and change are the same as `vf-4d9b756f1343`: cert-manager.io Certificate
  `spec.privateKey.rotationPolicy`, default-changed `"Never"` → `"Always"`.
- `exposure` is Certificate with `spec.privateKey.rotationPolicy` **unset**, with no `spec.isCA` condition, so it
  covers every Certificate that leaves the field unset. `overlap` is the field **set**.
- `consequence`: `workload-failure` / `action-required`, severity `high`.
  - Statement: "Every Certificate that leaves spec.privateKey.rotationPolicy unset gets a new private key at each
    reissuance, which breaks every consumer that trusts or pins the previous key."
  - Remediation: "Set spec.privateKey.rotationPolicy: Never explicitly on every Certificate before upgrading."
- Verification:
  - subject: deterministic (`val-7525eb69cfe5`, `val-9c6530d9fab0`)
  - change: human (`rd-5c50eff7c94b`)
  - applicability: human (`rd-f4ab657a81a9`)
  - consequence: human (`rd-f4ab657a81a9`)
- No role default. `supersedes [vf-9b9cfa5c9f5f]` (see below). **No relation to `vf-4d9b756f1343`**: no supersedes or
  coexists in either direction. `vf-4d9b756f1343` is unchanged, active, with a usable default.

**`cert-manager/1.18.0/decisions/rd-f4ab657a81a9.json`**: a human `correct` on review item `ri-125b8856cb8b`.
- Labels: corrected, wrong-applicability, wrong-consequence.
- `resultingFact` is `vf-45b922ec1cc6`. No supersedes or coexists.

**Refusal recorded (route A):** a human `correct` through `Queue.Decide` on the hypothesis item `ri-3110ace7e891` was
**refused**: `knowledge: decision rd-519554a55a1b on ri-3110ace7e891 refused: promotion gate: the corrected rule
differs from the verified one (digest 07dbbf1225d7): a changed rule needs its own proof — re-export it as a new
hypothesis item and verify it`.

**Built by (route B):** the same correction on `ri-125b8856cb8b`, the ordinary fact-review item behind
`vf-9b9cfa5c9f5f`, through `Queue.Decide`. `HypothesisSource` says that item is not hypothesis-sourced. `gateDecision`
applies the proof requirement and the "name every active fact on the subject" rule **only to hypothesis-sourced
items**. So the queue minted a second active human fact that contradicts `vf-4d9b756f1343` and does not name it.
This is a gap the M4 gate may want to know about.

**Side effects:**
- Because the item is the fact-review item of `vf-9b9cfa5c9f5f`, a correction automatically records
  `supersedes [vf-9b9cfa5c9f5f]`. That fact was already superseded, so its file is unchanged.
- `ri knowledge metrics` `Facts.AutoApprovalAgreement` drops from 1 to 0.667, because the correction counts as
  disagreement with the auto-approval of `vf-9b9cfa5c9f5f`.

**Load:**
- 242 facts kept, 1 pre-existing warning.
- `vf-45b922ec1cc6` and `vf-4d9b756f1343` are both active, human-verified and kept.

**Expected effect:** the validator's contradiction check (`ruleShape`, comparing change and applicability) should see
two active facts on the same subject with different applicability.

## 4. Kneg-inapplicable: built (the gate refuses it; the queue accepts it through an ordinary item)

Path: `/Users/tommydavison/.claude/jobs/3f39867a/tmp/m4/Kneg-inapplicable`

**What changed:** two new files and one changed file.

**New `cert-manager/1.18.0/facts/vf-84fd6c24fa39.json`**: K1's rule with the role condition flipped. It is active,
human-verified, release 1.18.0.
- `applicability.exposure.of[1]`: `spec.isCA equals ["true"]` → `["false"]`.
- `applicability.overlap.of[0].of[1].of[0]` (the "shielded" branch): `spec.isCA equals ["false"]` → `["true"]`.
  - I flipped the overlap too so the rule stays coherent: shielded means explicitly `Never`, or a CA certificate.
  - With only the exposure flipped, a non-CA Certificate with the field unset would be both exposed and shielded.
  - If you want only the exposure flipped, this is the one deviation from the brief.
- Everything else matches K1's fact: subject, change, consequence (behavior-change / review-required, same prose),
  statement and evidence.
- Verification: applicability is human (`rd-f4ab657a81a9`). Subject, change and consequence have the same basis as
  K1.
- `supersedes [vf-9b9cfa5c9f5f, vf-4d9b756f1343]`.
- **No role-membership default.** RI carries a default only on a human promotion through an open gate on a
  hypothesis item. K1's default was also observed for rule digest `d4a8d573d0c1`, not this rule (`4cd581aeb38b`), so
  it would be ignored here.

**New `cert-manager/1.18.0/decisions/rd-f4ab657a81a9.json`**: a human `correct` on `ri-125b8856cb8b`.
- Labels: corrected, wrong-applicability.
- `supersedes [vf-4d9b756f1343]`, `resultingFact` `vf-84fd6c24fa39`.

**Changed `cert-manager/1.18.0/facts/vf-4d9b756f1343.json`**: `status` active → `superseded`, through
`Queue.Decide`'s supersede. Nothing else changed.

**Refusal recorded (route A):** the same correction on hypothesis item `ri-3110ace7e891` through `Queue.Decide` was
**refused**: `promotion gate: the corrected rule differs from the verified one (digest 4cd581aeb38b): a changed rule
needs its own proof — re-export it as a new hypothesis item and verify it`.

**Built by (route B):** ordinary item `ri-125b8856cb8b`, not hypothesis-sourced, so there is no proof requirement.
The same `AutoApprovalAgreement` side effect applies (1 → 0.667).

**Load:**
- 242 facts kept, 1 pre-existing warning.
- `vf-84fd6c24fa39` is active, human-verified and kept, with 0 role defaults.
- `vf-4d9b756f1343` is superseded. It is still in the loaded set, but `factInEdge` and the contradiction check use
  only active facts.

## Summary

| store | built | load | new warnings | active rotationPolicy fact(s) |
|---|---|---|---|---|
| Kneg-nodefault | yes (FileStore; in-place edit refused) | ok, 241 kept | none | vf-4d9b756f1343, no default |
| Kneg-expired | yes (FileStore; gate refuses, decision path cannot mint 1.17.0) | ok, 241 kept | none | vf-8b678b57d8d6 @1.17.0, default usable |
| Kneg-contradicted | yes (Queue.Decide on ordinary item; gate refuses hypothesis route) | ok, 242 kept | none | vf-4d9b756f1343 and vf-45b922ec1cc6 |
| Kneg-inapplicable | yes (Queue.Decide on ordinary item; gate refuses hypothesis route) | ok, 242 kept | none | vf-84fd6c24fa39 (isCA false), no default |

Findings outside the brief:
1. The promotion gate's proof and coexistence rules apply only to hypothesis-sourced items. A human correction on an
   ordinary fact-review item of the same candidate mints a contradicting or rule-changing fact without a proof and
   without naming the active fact.
2. Proof `versionBounds` is not part of the proof's content id. The loader never checks bounds: not for facts, and
   not for role-membership defaults.
3. The loader does not check that a decision's `resultingFact` exists, or that a fact's release matches its
   candidate's or proof's release.
