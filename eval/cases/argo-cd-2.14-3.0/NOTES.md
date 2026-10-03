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

## 2026-10-03 — E1, E2 and E3 moved to undecidedImpact (product-owner decision, L7 option B)

Decided after seeing results (LOOP-DIAGNOSIS-2 §4c). The upstream escape hatch for each link lives in
a ConfigMap this fixture does not supply, so the honest answer is UNKNOWN (environment-visibility-gap).
The fixture is unchanged. See eval/CHANGELOG.md "2026-10-03 (j)".
