# Research notes: ingress-nginx controller-v1.11.5 → controller-v1.12.0

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Sources read

- The controller-v1.12.0 GitHub release body (the ⚠️-marked entries):
  https://github.com/kubernetes/ingress-nginx/releases/tag/controller-v1.12.0
- The referenced PRs: https://github.com/kubernetes/ingress-nginx/pull/11819
  (security defaults) and /pull/11821 (Lua plugin removal).

## Judgement calls

- The release body marks exactly four entries with ⚠️: security defaults
  (#11819), metrics off by default (#12153), s390x drop (#12137), and the
  Lua plugin removal (#11821). All four are expected items.
- strict-validate-path-type (inside E1) is the flip side of cert-manager
  1.18's PathType: Exact change — the two dataset cases corroborate each
  other (cert-manager's release notes point at this same default change).
- E2 rated critical: Prometheus scrape going silently dark is a production
  incident for most operators.
- notExpected: the release body is a conventional-commit firehose
  (Images:/CI:/Go:/Docs: prefixes); those are chore noise, not upgrade work.
