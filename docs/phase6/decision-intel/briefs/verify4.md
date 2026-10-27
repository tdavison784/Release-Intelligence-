# Lane `verify4`: Milestone 4 fix, "the role cannot form" as an observed probe outcome (PD-6) (Opus)

Branch `p6di/verify4`, worktree `.claude/worktrees/p6di-verify4`. Ledger D-0027. Read `_common.md` first; that
includes `PRODUCT-DECISIONS.md` **PD-5 and PD-6**. Then `M4-IMPLEMENTATION.md` and `status/verify3.md` (your
starting point).

## The gap (from verify3's real kind run; generic description)

verify3's default probes for a role field (unset, the other value) produced:
- **outside the role:** all values behaved the same (no harm), so the probes cannot tell the values apart;
- **inside the role:** the unset and other-value probes were **undetermined**, because the dependent output was
  never issued. With the role field unset or at its non-role value, the subject cannot act in the role, so the
  consumer relationship never forms.

So no observed default was recorded, and objects that leave the role field unset stay undecidable.

## Required (PD-6)

1. **A new probe outcome, `role-cannot-form`.** In a default probe *inside the role*, the controller is healthy and
   reconciled, the subject exists and is ready, but the dependent object the role needs (the output the consumer
   verifies) is never produced, or is explicitly refused. That is recorded as **role-cannot-form**: observed, with
   the evidence (object states, conditions and events; never logs), distinct from "undetermined", which covers
   timeouts, controller failures and missing objects for other reasons.
2. **An observed role-membership default.** When the probe for value U (for example unset) is `role-cannot-form`,
   and the role-value arm formed the relationship, record on the confirmed proof: "F = U ⇒ outside role R (observed)".
   It is used only to evaluate **membership in role R** for this refinement. It never becomes a general field
   default, and the evaluator must not use it for any other condition. It carries the limitation "observed on the
   fixture's controller configuration".
3. **The guards stay:**
   - only through a confirmed proof of the same rule digest;
   - ignored when the proof is inconclusive or refuted;
   - a contradiction with a cited or schema default blocks;
   - human promotion copies it into the fact, and the review UI shows it and its limitation.
4. **Tests:**
   - role-cannot-form versus undetermined (a controller failure, a timeout, a missing object);
   - the membership default recorded only when the role arm formed the relationship;
   - used only for role membership and not for another condition on the same field;
   - plus the existing guards.

   An opt-in kind case on the real controller should show the `role-cannot-form` outcome for an unset role field.
5. **Docs** and your status file. No live model runs on development cases. Clusters are deleted.

FLEET.md applies, including generic logic only and the files you never open.
