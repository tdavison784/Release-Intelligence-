# Arm D: product-owner review (D-EU, all C-whole runs), 2026-10-20

The session was resumed with `scripts/phase6/guided.sh`. No turn was sent, so there are no interventions and no
guided score. The product owner's feedback on D-EU was given in the commander session, while auditing C-whole r2:

- **The main miss, for every arm:** 1.12.0 is a security regression from 1.11.5. Five advisories, led by
  GHSA-mgvx-rpfc-9mpv (CVE-2025-1974, critical), affect `>= 1.12.0-beta.0, < 1.12.1`; 1.11.5 is fixed. "RI should have
  properly caught this … and recommended going to 1.12.1 instead … it needed to request a change to upgrade to 1.12.1;
  that is what engineers would expect." The write-up and evidence were otherwise rated good.
- **C-whole r1 (this run) got it:** "I do not recommend this specific target. 1.12.0 is a security regression from
  1.11.5", and "target at least 1.12.1 (chart 4.12.1)". That is the answer the product owner expects. Its evidence
  was the changelog entry plus a CVE recollection it asked to have verified.
- C-whole r2 flagged the regression but named no version ("a later 1.12.x or 1.13/1.14"). r3, every C-item run and
  arm B did not flag it.
- Root cause in RI and the fix: LEDGER X-0009.

Autonomous score unchanged (PROTOCOL §7): see `score.md`.
