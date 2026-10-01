# Research notes: cert-manager v1.15.4 → v1.16.0

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Sources read

- Upgrading v1.15 to v1.16 guide:
  https://github.com/cert-manager/website/blob/master/content/docs/releases/upgrading/upgrading-1.15-1.16.md
  Lists exactly three breaking changes: Helm schema validation, and two Venafi
  Issuer renewal failures (duration vs policy; TPP username-password → OAuth).
- Release 1.16 notes (theme sections):
  https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.16.md
  Same three items plus OperatorHub discontinuation, Route53 DNS01 cleanup.

## Judgement calls

- E2 and E3 are two sides of the Venafi change (authentication method vs
  duration handling). The upstream guide lists them as two separate numbered
  breaking changes, so the dataset keeps them separate; a tool that merges
  them into one change still gets both right if both claims appear.
- OperatorHub (E4) is rated `important` rather than `critical`: it breaks the
  upgrade path only for OLM users, and v1.16.5 remained available as a bridge.
- Route53 (E5) is a relaxation, not a break: `minor`, no action required, but
  an operator reading an Issuer with a pinned region should know the field is
  now ignored when AWS_REGION is set.
- notExpected: the GitHub release body aggregates conventional-commit PR
  titles; test/build dependency bumps and docs-only PRs are noise there.

## Real-world impact corroboration

The release-notes document itself links the Venafi change to reconfiguration
of TPP servers ("you may need to reconfigure your TPP server to enable OAuth
authentication"), which is why E2 is critical/action-required.
