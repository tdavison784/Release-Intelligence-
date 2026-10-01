# Research notes: Vault v1.21.4 → v2.0.0

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Sources read

- CHANGELOG.md section 2.0.0 (April 14, 2026), including its BREAKING
  CHANGES, SECURITY and CHANGES blocks:
  https://github.com/hashicorp/vault/blob/v2.0.0/CHANGELOG.md

## Judgement calls

- The 2.0.0 changelog labels exactly one item "BREAKING" (docker→moby SDK
  move, E5), but the CHANGES block contains operator-facing behaviour
  changes that matter far more in practice (E1, E2, E3, E4). E1
  (authenticated rekey/generate-root) is rated critical: break-glass
  automation that hits those endpoints unauthenticated fails at the worst
  possible moment after the upgrade.
- Security fixes (the long SECURITY block) are deliberately NOT expected
  items: they are reasons to upgrade, not upgrade work; matching them would
  pad recall.
- notExpected: plugin version bumps and UI improvements are churn, not
  upgrade intelligence. The CHANGELOG has dozens of each.
- Enterprise-only items (rotation manager, license reporting) were skipped —
  this dataset tracks the open-source server an operator runs in Kubernetes.
