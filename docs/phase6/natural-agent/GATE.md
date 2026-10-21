# Phase 0 decision gate (X-0006)

**Question:** natural Claude Code vs the RI agent, on P-US, P-EU and D-EU.

**Inputs:**
- **Scores and per-run quotes:** `SCORES.md`. The product owner audited them on 2026-10-20 and confirmed both boundary
  judgements as hedged.
- **Arm D:** the product owner's review of C-whole r1 per case, in `runs/<case>/C-whole-r1/product-owner-review.md`.
- **Protocol:** `PROTOCOL.md`, with amendment A1.

## Recommendation

**Proceed to the architecture plan.** "Meaningful advantage over RI" holds, narrowly. The plan has to be shaped by
what the advantage actually came from:

- **Not from per-item scoping.** That hypothesis was not supported for mechanism correctness.
- **From three other things:**
  - reading the consumers;
  - producing an upgrade-level recommendation;
  - for one item, D-EU B3, requiring a test instead of guessing.

The one item no arm gets reliably right, D-EU B3, is an evidence gap. Mission §11 ("if both agents struggle") applies
to it, not a redesign.

## The six questions

### 1. Does natural Claude Code distinguish the affected and unaffected environments?

**Yes, 2 of 3 C-whole repeats** (P-US B1 and P-EU B1 both E in r1 and r3).

- Both repeats name the CA's effective `rotationPolicy` as the deciding difference. The P-US overlay pins `Never`;
  P-EU's has no pin.
- r2 fails on P-US. Its headline calls the change "breaking … latent", but its own analysis clears the environment.
- C-item: 3 of 3. Arm B: P-US H, P-EU CU.

### 2. Does it explain the breaking mechanism and the mitigation?

**For cert-manager, yes.**

- Every C run followed the chain Certificate → Secret → mounted `ca.crt` → nginx / curl trust.
- For P-EU, the C runs explained the failure: a root key rotates on renewal, and pods still hold the old `ca.crt`.
- They gave the mitigation the product owner wants: "add `privateKey.rotationPolicy: Never` to `mesh-root-ca` …
  in Git" (P-EU C-whole r1).
- Arm B on P-EU had the mechanism (R1) but missed the mitigation: "I recorded no mitigation."

**For D-EU B3, no arm explains it from evidence.** The workspace and RI's bundle contain only "`strict-validate-path-type`
gets enabled by default". 5 of 6 C runs and arm B took the accepted character set from memory and got it wrong
("." allowed).

### 3. Does it trace relevant consumers more effectively?

**Yes. This is the largest difference, and it is retrieval, not model.**

- Arm B made 21–23 tool calls across about 18 change items. It never opened the payments Deployments or `ledger.conf`,
  although both were in its bundle: "I did not trace what consumes the two certificates' Secrets."
- Every C run opened them. C-item did too, and it was scoped to a single change like a B item. So per-change reading
  depth decides this, not whole-upgrade scope.

### 4. Does it avoid the Phase 5 false-safe?

**Narrowly. 2 of 3 C-whole repeats are not FS on D-EU B3** (r1 FS, r2 H, r3 H).

- The two non-FS runs avoided it by making a server-side dry-run a precondition, not by knowing the rule.
- r2 is the closer one. The product owner's reading of r2, and of P-US r2, is the same: "just not enough proper
  wording around them". The headline ("Probably not") was stronger than the evidence it rested on.
- False-safes in total: C-whole 1, C-item 1, B 1. Root cause in every case: missing evidence plus incorrect
  semantic interpretation from model knowledge.

### 5. What did it do differently from the RI agent?

1. **It read the consumers.** See question 3.
2. **It produced upgrade-level recommendations that RI's per-item output has no place for.**
   - cert-manager: 5 of 6 C-whole runs said to target the latest 1.18.x patch (1.18.6) rather than 1.18.0. The
     product owner: "that's what we want".
   - D-EU: C-whole r1 rejected the target outright ("1.12.0 is a security regression from 1.11.5") and recommended
     "at least 1.12.1 (chart 4.12.1)". The product owner rated this the main D-EU miss for RI.
   - No C-item run names a target version. Neither do the arm-B records held in the repository: RI's output is
     per item and has no field for one. Here, unlike mechanism correctness, whole-upgrade
     scope is what made the difference. That bears out part of the product owner's per-item suspicion.
3. **It turned uncertainty into a required test** (dry-run, staging renewal rehearsal) instead of a verdict.

In the opposite direction, it is also more willing to state model knowledge as fact (D-EU B3), so free-form
reasoning needs the same evidence discipline as RI's rules.

### 6. Would those differences justify an architectural change?

**Yes, for three of them. The smallest changes that reproduce the advantage:**

| Change | Source | Notes |
|---|---|---|
| Consumer-tracing retrieval: the agent must open the workloads that mount or read an affected object before concluding | Q3 | the cert-manager gap; independent of session scope |
| A whole-upgrade synthesis step after the per-item findings: a named target version (latest patch on the line; a lower patch when the target regresses security), plus change sequencing | Q5.2 | the product owner prefers small isolated changes: pin today's behaviour (`Never`) and test the new default (`Always`) separately in dev. Not one combined change |
| Evidence: ingest GitHub's global advisory database (X-0009), and supply behavioural semantics (validator source or docs) for "default enabled" changes | Q2, Q4, D-EU | RI's `advisory:affects-target` rule exists, but reads repository advisories only (0 for ingress-nginx) |
| Verdict wording: the headline carries its confidence and any precondition ("unverified; dry-run required") | Q4 | product-owner audit |
| Output: keep the natural write-up and fix its shape to a change ticket: summary/verdict · impact (affected / not affected and why, with evidence) · implementation (named target version, isolated change sequence) · test plan (missing information and tests) · backout | product-owner review, 2026-10-20 | "well done … layout and the overall message … great to serving into a change management system and process for a change ticket". Every C-whole run already has the verdict, affected/not-affected, tests and evidence. Only 4 of 9 have a recommendation or sequence section, and 7 of 9 mention rollback. |

**Not justified by Phase 0: replacing per-item sessions with one whole-upgrade session.**
- On mechanism correctness, C-item is equal to or better than C-whole: 3 of 18 pairs differ, all in C-item's favour.
- Keep per-item depth, and add the upgrade-level synthesis on top.
- Retain deterministic validation and the existing policy boundaries (mission §11).

## Arm D

The product owner resumed all three C-whole r1 sessions and sent no turns, so there are no interventions to classify
and no guided scores. The review was given in the commander session and is recorded verbatim per run. Its content
appears under questions 2, 4, 5 and 6. Arm D therefore offers no evidence that human guidance would close a gap, and
mission §11's "human-guided runs outperform" branch does not apply.

## Caveats

- n = 3 per condition on 3 cases. These are diagnostic thresholds, not statistics.
- "Avoids the false-safe" and "meaningful advantage" both rest on D-EU C-whole r2, now confirmed as H.
- The commander knows the labels and wrote the protocol and rubric (PROTOCOL §11).
- The upgrade-level findings (target version, sequencing) are outside RUBRIC. They come from the product owner's
  review, and no arm is scored on them.
