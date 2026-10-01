# Research notes — postgresql REL_17_2 → REL_17_3

Authored blind on 2026-10-01 from the SGML release notes at the REL_17_3 tag
(the channel the definition ingests: doc/src/sgml/release-17.sgml, section
"Release 17.3"), cross-checked against the postgresql.org announcement page
listed in sources.

## Judgement calls

- E1 is the release's security item (CVE-2025-1094) and carries
  action-required: a security remediation is upgrade work by definition; the
  notes also state an operator-side consequence (applications/drivers quoting
  untrusted input via libpq "should take special care").
- E2/E3 are behavioural changes of the minor update; review-required because
  both bite only under specific setups (over-length names in connection
  strings; per-worker privilege assumptions).
- E4 informational (tzdata), E5 review-required (earthdistance matters only
  to its users, but the notes explicitly connect it to v17 upgrade failures).
- The 17.3 section carries ~50 routine bug fixes; the notExpected entry
  samples the clearly non-upgrade shapes (assertion failures, leaks) so the
  precision measurement has teeth.
- No environment fixture: a source-based PostgreSQL product has no
  values/CRD/image surface the join vocabulary can compare (the
  postgres-image artifact is built per version, and no environment input
  pins it in this case).
