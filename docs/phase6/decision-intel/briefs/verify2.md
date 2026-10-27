# Lane `verify2`: Milestone 4 fix, non-discriminating exercise arms and verified scope refinement (Opus)

Branch `p6di/verify2`, worktree `.claude/worktrees/p6di-verify2`. Ledger D-0025. Read `_common.md` first, then
`PLAN-M4.md` and `M4-IMPLEMENTATION.md`, and the M4 lanes' status files (`status/applicability.md`,
`status/semverify.md`, `status/formalize.md`).

## Two problems found by the commander on a development run (generic description)

### 1. Non-discriminating exercise arms

**The rule:** "when field F is unset, the target version's new default D applies (new behaviour); F set explicitly to
the previous value P keeps the previous behaviour".

**The arms the exercise synthesised:**
- **F = P:** previous behaviour at both versions, as predicted. Pass.
- **F omitted:** the behaviour changed between FROM and TO, as predicted. Pass, and this is the real evidence of the
  default change.
- **F = D explicitly** (the synthesised negation of the condition): it predicted "new behaviour". But an explicit D
  behaves the same at FROM and TO, so no change was observed, and the **whole rule became inconclusive**.

**The fix:** an arm whose condition yields the same behaviour at both versions by construction (an explicit value
equal to the new default, or anything that does not contrast FROM with TO) is classified **non-discriminating**. It is
recorded and excluded from the verdict. A default-change rule is decided by its omitted-field arm plus its condition
arm. A rule whose only contrast arm is non-discriminating stays inconclusive. Document the classification and test
it, including a rule where an explicit value *does* discriminate.

### 2. A scope refinement that only verification can establish

**What was missing.** The investigation traced consumers that read key material produced by a certificate-like
subject. In one environment a workload's config reads the issuer CA's certificate file from a mounted Secret. Whether
a behaviour change actually harms those consumers depends on how they use the key material, for example verifying
peers against the CA rather than pinning a leaf. That is consumer-software behaviour that no upstream fetch of the
product establishes, and the agent stated it only as inferred. So formalization correctly produced only the broad
behaviour rule, which cannot clear an environment whose CA is pinned while its leaves are not.

**The fix: scope refinement as a candidate, established by observation.**
- **(a) Formalization proposes a scope-refinement candidate:** "the harmful consequence occurs only for subjects in
  consumer role R". R is a role declared in the product's exercises catalog (`consumers:` and role conditions, for
  example a CA-trust consumer whose role is a field condition on the subject), and only when the investigation's trace
  shows consumers reading key material that role R produces.
  - The candidate cites the trace (the `reads-file` edges) and the catalog role.
  - It is marked `ProposedVerification: consumer exercise (role R)`.
  - It is **never** a confirmed rule by itself: the non-invention floor still holds.
- **(b) The verifier runs the declared consumer exercise for role R** (as `verify.exercise-consumer@v1` already does):
  the consumer verifies at FROM and at TO, in the arms with and without the role condition and with and without the
  mitigating setting.
  - The refinement is **confirmed only by an observed pass** in which the role-absent arm shows no consumer harm and
    the role-present, unmitigated arm shows harm. Otherwise it is refuted or inconclusive.
  - The omitted-field boundary (applicability lane) applies to the role condition too. With no evidenced default the
    refinement stays undecidable for objects that leave the role field unset.
- **(c) The resulting rule** (behaviour rule + confirmed refinement) goes through the existing promotion gate
  unchanged: a human only, a confirmed proof of the same rule digest, version bounds.
- **No product logic in Go.** Roles, consumers and their conditions come only from the catalog. Prompts stay generic.

## Required

1. Both fixes, with tests:
   - the non-discriminating arm;
   - a refinement candidate proposed only with trace evidence;
   - no candidate without it;
   - refinement confirmed, refuted, and inconclusive on fixtures (kind-based tests skip without kind or docker);
   - an unevidenced role default that stays undecidable;
   - a promotion gate that still requires a human.
2. **Replay and history:**
   - stored proofs and formalization records stay readable;
   - existing proofs are not re-judged retroactively;
   - prompt `formalize/v1` has been run once (by the commander on a development case), so it is frozen: use
     `formalize/v2`.
3. **No live model runs on development cases;** the commander re-runs the demo. Kind clusters are named
   `ri-verify2-*` and deleted.
4. **Docs:** `M4-IMPLEMENTATION.md`, `docs/INVESTIGATE.md` (verification), and your status file.

FLEET.md applies, including generic logic only and the files you never open.
