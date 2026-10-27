# Lane `advisories3` status (D-0012, Milestone 2 fix)

Branch `p6di/advisories3` from `p6-decision-intel` @ e6eeda35. Brief: `briefs/advisories3.md`. **State: done**, ready
for the commander to merge.

## The defect and the fix

An advisory can state one range across lines but list per-line patched versions (`>= 1.2.0, <= 1.4.2` with
`patched: 1.3.6, 1.4.3`), so a patched version lies inside its own range. RI followed the range (correct) but said
nothing about it.

Now (`internal/upgrade/advisories.go`, `security.go`):

1. **Detect.** Every assessed advisory's patched versions are checked against its constraint with the same check as
   every other version (`contradicting`). An explicit version list that names a patched version is caught the same
   way. A prerelease patched version matches only a range that names a prerelease.
2. **Report.** A contradiction becomes:
   - an entry in `EdgeSecurity.inconsistencies` (`AdvisoryInconsistency`: id, aliases, URL, range, full patched list,
     contradicting versions, source(s), `changeId`, statement, evidence);
   - a `security.gaps` entry: "advisory-consistency: GHSA-x lists 1.3.6 as patched but its range >= 1.2.0, <= 1.4.2
     includes it; check the advisory (<url>). The stated range decides …";
   - an edge warning with the same statement.

   It is reported even when the advisory affects neither endpoint, because it can still decide a candidate. Its
   evidence is then added to the edge, so it can be cited. The impact report carries it (and cites the evidence), the
   text reports render an "Inconsistent" list, and the dossier summary has `inconsistent`.
3. **Candidates stay strict.** `candidates()` was refactored into per-kind questions answered under a *reading*. The
   strict reading (stated ranges) chooses the candidate exactly as before. When any advisory is inconsistent, each
   question is answered again under the alternative reading: a contradicting patched version fixes its own line from
   that version on, with From and To read the same way. A different answer becomes `VersionCandidate.alternatives`
   (`CandidateAlternative`: version or none, advisories, reason, evidence), and the reason is appended to the
   statement: "If GHSA-x's patched list is right, 1.3.6 also qualifies; it is not chosen because the stated range
   includes it." When the alternative is stricter (From is itself a contradicting patched version), the reason says
   that the strict candidate would introduce the advisory.
4. **Tests** (`internal/upgrade/security_consistency_test.go`, plus additions to the impact and dossier tests):
   - contradiction: gap, warning, inconsistency and alternatives; candidates identical to the same advisory without the
     contradicting patched version;
   - consistent advisories, including a prerelease lower bound with a stable fix, a patched prerelease below a stable
     bound, and a free-text patched entry: no finding;
   - explicit version list (through `github.ConvertVersionRange`) naming a patched version: contradiction;
   - a prerelease patched version inside a prerelease-bounded range: contradiction;
   - a contradiction away from both endpoints: reported, no alternative where the answer does not change;
   - From is the contradicting patched version: stricter alternatives, plus a `current-line` alternative;
   - render, impact report citation and render, dossier lines.

## Decisions (with why)

- **"Inside the range" means as RI evaluates the range.** Otherwise RI would flag as inconsistent a reading it uses
  itself. With Masterminds, `< 1.1.2` does not include `1.1.2-rc.1`, so that case is not flagged.
- **Alternative reading = a patched version fixes its line from that version on.** GitHub's patched version is the
  first fix of its line. Sparing only the exact version would treat 1.3.7 as still affected.
- **The alternative also re-reads From and To**, so the alternative answer is the true answer under that reading. The
  set of candidate kinds is still decided by the strict classification.
- **Alternatives are not candidates.** They get no chart lookup (`CandidateVersions` is unchanged), and `removes` and
  `leaves` stay strict.
- **Status unchanged by an inconsistency.** The source was read. The finding is a gap (allowed with `complete`).
- **No policy or classification change.** Edge warnings are only copied into reports and rendered, never used by
  policy. The section still adds no impact finding.

## Files touched

- `internal/domain/edgesecurity.go`: `Inconsistencies`, `AdvisoryInconsistency`, `VersionCandidate.Alternatives`,
  `CandidateAlternative`, validation.
- `internal/domain/schemagen/meta.go` (3 descriptions). `schemas/upgrade-edge.schema.json` and
  `schemas/impact-report.schema.json` were regenerated.
- `internal/upgrade/advisories.go`, `security.go`, `security_render.go`.
- Outside ownership (additive): `internal/impact/build.go` (cites the new evidence), `internal/investigate/agent/dossier.go`
  (`Inconsistent`, alternatives on candidate lines).
- Tests: `internal/upgrade/security_consistency_test.go` (new), `internal/impact/advisory_security_test.go`,
  `internal/investigate/agent/dossier_security_test.go`.
- Docs: `docs/SECURITY_ADVISORIES.md` (new section "Advisories that contradict themselves", fields), `docs/IMPACT.md`
  (one sentence).

## Contract changes (additive)

`EdgeSecurity.inconsistencies`, `VersionCandidate.alternatives`, `DossierSecurity.inconsistent`. Candidate statements
gain an appended sentence when an alternative exists. The schema version is unchanged (additive optional fields).

## Regression comparison

No live runs (brief). Existing candidate tests and the e2e goldens are unchanged and pass: no inconsistency means no
new output. The stored runs carry no edge security section, so rescoring cannot exercise this code (see
`status/advisories.md`).

## Open questions

None.

## Test status

`go build ./... && go vet ./... && go test ./...`: green (46 packages ok, 0 FAIL). The schemas are regenerated.

## Cleanup

No clusters or servers were started. There is no `bin/` and no scratch output in the worktree.
