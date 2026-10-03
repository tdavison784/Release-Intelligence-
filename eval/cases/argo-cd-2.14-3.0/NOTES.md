# Research notes — argo-cd 2.14.5 → 3.0.0

Authored blind on 2026-10-01 from docs/operator-manual/upgrading/2.14-3.0.md
at v3.0.0 (the guide the definition's upgrade-guide source reads, file
2.14-3.0.md — the predecessor minor of 3.0 is 2.14) and
tested-kubernetes-versions.md at v3.0.0.

## Sources read

- The 2.14 → 3.0 upgrade guide, "Breaking Changes" section top to bottom:
  fine-grained RBAC (E1), logs RBAC default (E2), default resource.exclusions
  (E3), legacy controller metrics removal (E4), Dex sub-claim switch (E5),
  legacy repo config removal (E6), applyNestedSelectors ignored (E7), Helm
  3.17.1 bundling (E8), annotation-based tracking default (E9). The guide
  also documents the end of 2.x minor releases (support-policy prose, not an
  operator action — left out).
- tested-kubernetes-versions.md: 3.0 tests v1.29-v1.32 — grounds F1 for the
  fixture's 1.29 cluster.

## Judgement calls

- E3/E4/E5/E6/E7/E8/E9 are review-required, not action-required: each bites
  only under conditions the guide itself frames as conditional ("Users who
  have … are unaffected"). E1/E2 are unconditional for anyone using the
  affected RBAC surface, hence action-required.
- The environment fixture deliberately pairs cert-manager with Argo CD so E3
  (default exclusions naming CertificateRequest) has a concrete, checkable
  relevance claim — a cross-product interaction the join cannot see (no rule
  reads Argo CD's exclusion list), so the E3 link is an expected honest miss.

## Semantic labels and corrections (2026-10-01, groundtruth lane)

Authored blind from the upstream sources above (no pipeline output read); corrections are listed with before/after values in eval/CHANGELOG.md.

- All nine items carry `semantics`. RBAC items use the `rbac-permission` family (`applications/update`, `applications/delete`, `logs/get`) with `permission-lost`; the removed argocd-cm keys use `config-key` (component `argocd-cm`).
- E1 exposure: a policy.csv line granting `applications, update|delete` AND no line granting `update/*|delete/*` (a line-level approximation of "per role" — the condition language has no per-role correlation), AND the documented escape hatch `server.rbac.disableApplicationFineGrainedRBACInheritance: "false"` absent. E2 follows the guide's Detection section verbatim (no `policy.default` or a custom one, no `logs, get` grant, enforce flag not "true").
- E3: the new default exclusions ship in the install manifest's argocd-cm (manifests/base/config/argocd-cm.yaml at v3.0.0), not in code, so the exposure is "cert-manager / Cilium / Kyverno present" AND argocd-cm does not override `resource.exclusions`. The fixture supplies no argocd-cm, so a deterministic evaluator can only decide the override clause from a complete-manifests declaration; the relevance (review) is unchanged.
- E8: bundled Helm 3.16.3 → 3.17.1 from hack/tool-versions.sh at both tags.

## Fixture completion: argocd-cm (2026-10-02, groundtruth-6; LOOP-DIAGNOSIS-2 §8.3/L7)

The E1/E2/E3 exposures each end in a `not(argocd-cm sets <override>)` guard,
but the fixture supplied no argocd-cm at all, so under absence-is-not-knowledge
all three links were honestly UNKNOWN against their affected labels (the
2026-10-01 judgement call anticipated this: "a deterministic evaluator can
only decide the override clause from a complete-manifests declaration").
Completed the fixture instead of relabelling to undecidedImpact: a declarative
Argo CD install always ships argocd-cm, and the v2.14.5 install manifest's
argocd-cm is present with **no data keys** (every key optional upstream) —
which is exactly the environment the story describes (no RBAC overrides, no
resource.exclusions override; the base case's E3 note documents the default
exclusions shipping in the *v3.0.0* manifest's argocd-cm, not the customer's).
Labels, whys and matchers unchanged; E1/E2 evidence now also cites
manifests/argocd-cm.yaml. Engine-verified: E1/E2/E3 decide true as labelled.
See eval/CHANGELOG.md "2026-10-02 (g)".
