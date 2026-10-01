package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// checkRelationships ingests each release in exhaustive mode (every member
// of a fallback group is consulted) and turns sources, artifacts and
// contents into checks. Releases are processed sequentially in the given
// order; each ingestion is internally parallel.
func (i *Ingester) checkRelationships(ctx context.Context, def *catalog.ProductDefinition, releases []domain.Version, known *VersionList) (*RelationshipReport, error) {
	if def == nil {
		return nil, errors.New("ingest: nil product definition")
	}
	rep := &RelationshipReport{Product: domain.ProductID(def.ID)}
	byID := map[domain.EvidenceID]domain.Evidence{}
	var evs domain.EvidenceSet
	for _, v := range releases {
		res, err := i.ingest(ctx, def, v, known, true)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("release %s: %w", v, err)
		}
		rep.Releases = append(rep.Releases, res.release.Version.Semver)
		for _, e := range res.release.Evidence {
			byID[e.ID] = e
		}
		for _, e := range res.checkEvidence {
			if e.ID != "" {
				byID[e.ID] = e
			}
		}
		for _, c := range res.checks {
			for _, id := range c.Evidence {
				if e, ok := byID[id]; ok {
					evs.Add(e)
				}
			}
			rep.Checks = append(rep.Checks, c)
		}
	}
	rep.Summary = summarize(rep.Checks)
	rep.Evidence = evs.List()
	return rep, nil
}

// summarize aggregates checks per subject. Each release counts once per
// subject: a release fails when any of its checks failed, passes when any
// passed, else is unverifiable (not-applicable checks are ignored).
func summarize(checks []RelationshipCheck) []RelationshipSummary {
	type key struct{ kind, subject string }
	var order []key
	perRelease := map[key]map[string]string{} // subject → release → outcome
	var releaseOrder = map[key][]string{}
	rank := map[string]int{OutcomeNotApplicable: 0, OutcomeUnverifiable: 1, OutcomePass: 2, OutcomeFail: 3}
	for _, c := range checks {
		k := key{c.SubjectKind, c.Subject}
		m, ok := perRelease[k]
		if !ok {
			m = map[string]string{}
			perRelease[k] = m
			order = append(order, k)
		}
		prev, seen := m[c.Release]
		if !seen {
			releaseOrder[k] = append(releaseOrder[k], c.Release)
		}
		if !seen || rank[c.Outcome] > rank[prev] {
			m[c.Release] = c.Outcome
		}
	}
	out := make([]RelationshipSummary, 0, len(order))
	for _, k := range order {
		s := RelationshipSummary{Subject: k.subject, SubjectKind: k.kind}
		for _, rel := range releaseOrder[k] {
			switch perRelease[k][rel] {
			case OutcomePass:
				s.Passed++
			case OutcomeFail:
				s.Failed++
			case OutcomeUnverifiable:
				s.Unverifiable++
			}
		}
		switch {
		case s.Failed > 0:
			s.Verdict = VerdictFailing
		case s.Passed >= MinValidations:
			s.Verdict = VerdictValidated
		default:
			s.Verdict = VerdictInsufficient
		}
		out = append(out, s)
	}
	return out
}
