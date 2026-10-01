# Research notes — terraform-provider-aws v5.99.1 → v6.0.0

Authored blind on 2026-10-01 from the CHANGELOG.md at the v6.0.0 tag (the
channel the definition ingests; its BREAKING CHANGES section lists every
removal/default/validation change of the major) and the published v6 upgrade
guide (registry.terraform.io, listed in sources as the upstream narrative for
the same facts).

## Judgement calls

- The v6.0.0 BREAKING CHANGES section is long (~60 entries); the case picks
  one entry per SHAPE — behaviour/output change (E1), attribute removal with
  a migration target (E2, E4, E5), default change (E3), EOL removals (E6),
  data-source deprecations (E7), validation tightening (E8), provider-level
  deprecation with a removal horizon (E9) — so the precision measurement
  covers the changelog's variety rather than duplicating one resource.
- E1 is critical/action-required: a provider that suddenly renders
  `user_data` as cleartext leaks secrets into state and logs — the upstream
  entry's "Base64 encoded content should use user_data_base64 instead" is
  direct operator work.
- E3 review-required: the new default is a security improvement; only
  clusters relying on public accessibility act (by pinning the attribute).
- No environment fixture: a Terraform provider has no Helm-values/CRD/image
  surface the join vocabulary compares (.tf files are only mined for image
  references, which this provider does not ship).
