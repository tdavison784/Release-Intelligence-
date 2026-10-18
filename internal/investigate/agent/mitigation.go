package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Structured mitigation hypotheses (prompt feature mitigation; PROTOCOL §8).
// record_hypothesis may carry a proposed rule in the existing fact grammar,
// the environment basis showing this environment meets its conditions, and
// the verification requested. The tool layer checks it deterministically
// before recording: the grammar (a complete, decidable assertion), the
// subject exists in the bundle and belongs to its product, the release
// bounds parse and lie within this run's transition, every basis reference
// resolves in the run, and at least one citation is upstream text. A spec
// that fails is not stored: the hypothesis is recorded as free text and
// MitigationRejected says why. A well-formed spec is still only a proposal:
// it is never wired into a pin (the agent never chooses the pin rule); it
// becomes knowledge only through export, independent verification and a
// human decision.

// mitigationSpec checks a mitigation and returns the spec, or the reason it
// is malformed.
func (s *Session) mitigationSpec(ctx context.Context, it domain.ChangeItem, in investigate.MitigationInput, cites []domain.Evidence) (*domain.MitigationSpec, string) {
	var basis []string
	for _, b := range in.EnvironmentBasis {
		if b = trimmed(b); b != "" {
			basis = append(basis, b)
		}
	}
	spec := &domain.MitigationSpec{Item: it.ID, ProposedRule: in.ProposedRule, EnvironmentBasis: uniqSorted(basis),
		RequestedVerification: trimmed(in.RequestedVerification)}
	if err := spec.Validate(); err != nil {
		return nil, clip(err.Error(), maxDetail)
	}
	sub := spec.ProposedRule.Assertion.Subject
	if string(sub.Product) != string(s.cfg.Bundle.Change.Product.ID) {
		return nil, fmt.Sprintf("subject product %q is not this investigation's product %q", sub.Product, s.cfg.Bundle.Change.Product.ID)
	}
	if why := s.subjectInBundle(ctx, *sub); why != "" {
		return nil, why
	}
	if why := s.boundsWithinRun(spec.ProposedRule.Release); why != "" {
		return nil, why
	}
	for _, ref := range spec.EnvironmentBasis {
		if why := s.basisRef(ctx, it.ID, ref); why != "" {
			return nil, fmt.Sprintf("environmentBasis %q: %s", ref, why)
		}
	}
	upstream := false
	for _, e := range cites {
		if a, ok := s.artifact(e.SourceID); ok && a.Origin == domain.OriginUpstream {
			upstream = true
		}
	}
	if !upstream {
		return nil, "no citation is upstream text: a mitigation rule must cite the upstream source that states the behaviour"
	}
	return spec, ""
}

// subjectTokens are the words that must occur together in one bundle
// artifact for a subject to exist there.
func subjectTokens(sub domain.Subject) []string {
	leaf := func(p string) string {
		segs := strings.Split(strings.ReplaceAll(p, "[]", ""), ".")
		return segs[len(segs)-1]
	}
	switch sub.Family {
	case domain.SubjectCRDField:
		return []string{sub.Kind, leaf(sub.Path)}
	case domain.SubjectHelmValue, domain.SubjectConfigKey, domain.SubjectTerraformAttribute:
		return []string{leaf(sub.Path)}
	case domain.SubjectGVK:
		return []string{sub.Kind}
	}
	return []string{sub.Name}
}

// subjectInBundle reports why the subject cannot be found in the bundle
// ("" = found): its identifying words occur together in one artifact.
func (s *Session) subjectInBundle(ctx context.Context, sub domain.Subject) string {
	toks := subjectTokens(sub)
	for _, a := range s.cfg.Bundle.Manifest.Artifacts {
		b, err := s.bytesOf(ctx, a)
		if err != nil || isBinary(b) {
			continue
		}
		text, all := string(b), true
		for _, t := range toks {
			all = all && tokenLine(text, t) > 0
		}
		if all {
			return ""
		}
	}
	return fmt.Sprintf("subject %s does not occur in the bundle (no artifact names %s)", sub.Key(), strings.Join(toks, " and "))
}

// boundsWithinRun reports why the rule's bounds are not within this run's
// FROM→TO ("" = they are): the rule must be about this transition.
func (s *Session) boundsWithinRun(r domain.ReleaseBounds) string {
	from, to, err := r.Versions()
	if err != nil {
		return err.Error()
	}
	runFrom, err1 := domain.ParseReleaseVersion(s.cfg.Bundle.Change.From.Tag)
	runTo, err2 := domain.ParseReleaseVersion(s.cfg.Bundle.Change.To.Tag)
	if err1 != nil || err2 != nil {
		return fmt.Sprintf("this run's versions %s → %s are not semver: the release bounds cannot be checked", s.cfg.Bundle.Change.From.Tag, s.cfg.Bundle.Change.To.Tag)
	}
	if from.LessThan(runFrom) || runTo.LessThan(to) {
		return fmt.Sprintf("release bounds (%s, %s] are not within this run's transition %s → %s", r.From, r.To, s.cfg.Bundle.Change.From.Tag, s.cfg.Bundle.Change.To.Tag)
	}
	return ""
}

// basisRef reports why an environment-basis reference does not resolve
// ("" = it does): a verified claim or a validation record of this run for
// the item, or a subject the resolver finds in the graph.
func (s *Session) basisRef(ctx context.Context, item, ref string) string {
	if strings.HasPrefix(ref, domain.SettingClaimIDPrefix) || strings.HasPrefix(ref, domain.ValidationRecordIDPrefix) {
		return s.verifiedFact(item, ref)
	}
	if _, _, err := s.resolveSubjects(ctx, "environmentBasis", []string{ref}); err != nil {
		return strings.TrimPrefix(err.Error(), investigate.ErrRejected.Error()+": ")
	}
	return ""
}
