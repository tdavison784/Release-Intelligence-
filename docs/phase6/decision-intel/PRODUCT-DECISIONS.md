# Product decisions (product owner; lane-safe; binding on later milestones)

The mission (`MISSION.md`) is the product owner's text, kept verbatim. This file records the decisions the product
owner made after it, in order. Lanes read this file. Each milestone's plan must satisfy the entries that name it.

## PD-1. Engineer approval of the agent's judgement (2026-10-27): M3, M5

> "Agents should be able to give a judgment on an upgrade that can then be approved or denied. An engineer can either
> approve or deny the agents judgement/recommendation. The ownus is then on the engineer who approved."
> Confirmed: "yes add it in".

**The model.**
1. **The judgement.** RI's upgrade-level synthesis (M3) gives one committed judgement per upgrade and environment:
   Proceed, Proceed with changes, Modify target, Validation required, or Do not proceed (mission C5). Its headline
   states its scope and preconditions.
2. **The decision.** A named engineer approves or denies that judgement (M5). The decision is the human's and is
   recorded as such. The engineer who approves owns the decision.

**For accountability to be fair, approval must be informed.** The judgement shows, before any decision:
- **What is verified, inferred and unknown**, and every precondition. For example: "provided the workloads reload
  certificates".
- **Every disagreement between the agent and RI's deterministic results**, shown prominently. For example: the agent
  says proceed while a check found a security regression, an unresolved breaking mechanism, an untraced consumer or an
  unavailable behaviour.
- **The residual risks the approval accepts**, listed. Approving means "I accept these".

**Guardrails.**
1. **Override is distinct from approval.** Approving against a hard deterministic finding is an override: the
   engineer acknowledges each such finding explicitly and gives a reason. Hard findings include confirmed breakage, a
   security regression, a candidate version still affected, and a failed validation. The override is recorded
   separately from a plain approval.
2. **Approval decides one change, not knowledge.** Approving an upgrade for one environment never promotes the
   agent's reasoning into trusted knowledge, and never clears other environments. Reusable rules still go through
   knowledge review and verification (mission E6).
3. **Denial is feedback.** A denial carries a reason (for example "use 1.20.3", "pin Never first"), which is recorded
   for the learning loop.
4. **The policy floor is unchanged.** The approval is a recorded human decision. It never changes the deterministic
   classification, and never widens automatic approval eligibility (mission F1, §13). Auto-approval rules stay as they
   are.
5. **Rubber-stamping is measured.** For M6, track:
   - approvals later found to need correction;
   - approvals given while a deterministic result disagreed;
   - time to decision.

**Audit record per decision:** the approver, decision (approve, deny or override), reason, the residual risks and
hard findings acknowledged, timestamp, and the exact judgement version and evidence bundle it applied to.

**Gates.**
- **M3:** the judgement commits (scope and preconditions in the headline), and lists verified, inferred and unknown,
  preconditions, residual risks and every agent-versus-deterministic disagreement.
- **M5:**
  - approve, deny and override are recorded with the full audit record;
  - overriding a hard finding is impossible without an explicit acknowledgement;
  - approval never alters classification or knowledge;
  - a denial's reason is stored.

## PD-2. M2 passes; v13 is the default (2026-10-27): M3

> "Lets do v13 as the default for M3. I also consider M2 being met, lets continue."

Prompt `investigate-agent/v13` is the default from now on: consumer tracing, natural assessments, upstream evidence
fetched at the tags, behaviour dependencies, fetch first. M3's synthesis builds on v13 investigations.

## Earlier decisions carried forward

- **PD-0a (2026-10-20). Recommend a concrete safer target.** When the target regresses security, recommend the safer
  target ("go to 1.12.1", not "a later 1.12.x"). (M2 delivered the candidates; M3 states the recommendation.)
- **PD-0b (2026-10-20). Latest patch; small isolated changes.** Pin today's behaviour first, then upgrade, then test
  the new default separately in development. (M3 sequencing.)
- **PD-0c (2026-10-20). The write-up is a change ticket.** Summary and recommendation, impact, implementation,
  validation, backout, audit. (M5 report.)
- **PD-0d (2026-10-26/27). Scoped verdicts.** Approved; prompt v10 did not achieve them per item, so v9 stays the
  default. The committed, scoped answer belongs to M3's judgement (PD-1). If M3 cannot produce it, agent-level scoping
  returns, tested on blind cases (M6).
- **PD-0e (2026-10-27). Newer safe target accepted.** A strictly-read advisory range giving a newer safe target (1.20.3
  rather than 1.19.6) is acceptable. Self-contradicting advisories are flagged with the alternative.
