# Research notes: Argo CD v3.0.6 → v3.1.0

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Sources read

- The operator upgrade guide for exactly this transition:
  https://github.com/argoproj/argo-cd/blob/master/docs/operator-manual/upgrading/3.0-3.1.md
  Sections: symlink protection, v1 Actions API deprecation, PKCE flow moved
  to the server (with detection/remediation commands), Helm 3.18.4, Kustomize
  5.7.0, sanitized project API response, added healthchecks.

## Judgement calls

- E2 (PKCE) is the only item the guide gives a Detection and Remediation
  procedure for, and it requires identity-provider changes — critical.
- E1 is a deprecation with fallback behavior in 3.1, so actionRequired is
  false even though importance is important (migrate "as soon as possible").
- E4 is a security-driven behavior change; recorded under kind: security with
  the GHSA reference. API consumers reading project credentials break.
- The long "Added Healthchecks" list is feature content, recorded in
  notExpected: flagging individual new health checks as upgrade work would be
  noise.
- notExpected also covers conventional-commit prefixes that the git-log
  channel can drag in (docs/chore/CI/test commits).
