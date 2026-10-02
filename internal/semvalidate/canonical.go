package semvalidate

import (
	"context"
	"encoding/json"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// canonicalValidator confirms the APPLICABILITY aspect when the asserted
// condition equals the canonical one for the subject family and change
// (DESIGN.md §1.3, "Canonical applicability"), AND the subject and change are
// confirmed by an artifact validator. That is why most facts need a human only
// for the consequence. It also confirms the one consequence a validator may:
// kind `none` on an `added` subject.
//
// A condition that differs from the canonical one is inconclusive, never
// refuted: a narrower or different exposure can be right, it just is not
// provable here.
type canonicalValidator struct {
	artifact []knowledge.Validator
}

func newCanonical() canonicalValidator {
	return canonicalValidator{artifact: []knowledge.Validator{valuesValidator{}, crdValidator{}, compatValidator{}, imageValidator{}}}
}

func (canonicalValidator) Name() string { return ProducerCanonical }

func (c canonicalValidator) Validate(ctx context.Context, in knowledge.ValidationInput) ([]domain.ValidationResult, error) {
	a := in.Assertion
	if a.Subject == nil || a.Change == nil || (a.Applicability == nil && a.Consequence == nil) {
		return nil, nil
	}
	// the artifact validators must prove the subject and the change
	var ev evidenceSet
	subjOK, chgOK := false, false
	var blocked []string
	for _, v := range c.artifact {
		rs, err := v.Validate(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			for _, ck := range r.Checks {
				switch {
				case ck.Outcome == domain.OutcomeConfirmed && ck.Aspect == domain.AspectSubject:
					subjOK = true
					ev.add(r.Evidence...)
				case ck.Outcome == domain.OutcomeConfirmed && ck.Aspect == domain.AspectChange:
					chgOK = true
					ev.add(r.Evidence...)
				case ck.Outcome != domain.OutcomeConfirmed && (ck.Aspect == domain.AspectSubject || ck.Aspect == domain.AspectChange):
					blocked = append(blocked, string(ck.Aspect)+" "+string(ck.Outcome))
				}
			}
		}
	}
	proven := subjOK && chgOK && len(blocked) == 0
	why := "the subject and change are not both confirmed by an artifact validator"
	if len(blocked) > 0 {
		why += " (" + blocked[0] + ")"
	}
	vs := map[domain.Aspect]verdict{}
	if a.Applicability != nil {
		rule := "canonical:" + string(a.Subject.Family) + "/" + string(a.Change.Type)
		canon := canonicalFor(a.Subject, a.Change)
		switch {
		case canon == nil:
			vs[domain.AspectApplicability] = inconclusive(rule, "no canonical exposure condition for %s %s", a.Subject.Family, a.Change.Type)
		case !proven:
			vs[domain.AspectApplicability] = inconclusive(rule, "%s", why)
		case equalApplicability(*a.Applicability, canon):
			vs[domain.AspectApplicability] = confirmed(rule, "the exposure%s is the canonical condition for a %s of a %s", overlapText(a.Applicability), a.Change.Type, a.Subject.Family)
		default:
			vs[domain.AspectApplicability] = inconclusive(rule, "the asserted condition differs from the canonical one (a narrower or different exposure needs a reviewer)")
		}
	}
	if k := a.Consequence; k != nil && k.Kind == domain.ConsequenceNone && a.Change.Type == domain.ChangeKindAdded {
		if proven {
			vs[domain.AspectConsequence] = confirmed("canonical:added/none", "a newly added subject has no consequence for existing environments")
		} else {
			vs[domain.AspectConsequence] = inconclusive("canonical:added/none", "%s", why)
		}
	}
	return build(in, ProducerCanonical, vs, ev), nil
}

func overlapText(a *domain.Applicability) string {
	if a.Overlap != nil {
		return " and overlap"
	}
	return ""
}

func equalApplicability(a domain.Applicability, canon []domain.Applicability) bool {
	got, _ := json.Marshal(a)
	for _, c := range canon {
		want, _ := json.Marshal(c)
		if string(got) == string(want) {
			return true
		}
	}
	return false
}

func leaf(op domain.ConditionOp, state domain.FieldState, path string) *domain.Condition {
	return &domain.Condition{Op: op, Path: path, State: state}
}

func resourceCond(s *domain.Subject, version bool, state domain.FieldState) *domain.Condition {
	r := &domain.Condition{Op: domain.OpResource, Group: s.Group, Kind: s.Kind,
		Of: []domain.Condition{{Op: domain.OpField, Path: s.Path, State: state}}}
	if version {
		r.Version = s.Version
	}
	return r
}

// canonicalFor returns the canonical applicability for (family, change), or
// several equivalent spellings (a resource condition may or may not carry the
// subject's version); nil when the pair has no canonical condition.
func canonicalFor(s *domain.Subject, c *domain.ChangeSpec) []domain.Applicability {
	one := func(exp domain.Condition, overlap *domain.Condition) []domain.Applicability {
		return []domain.Applicability{{Exposure: exp, Overlap: overlap}}
	}
	switch s.Family {
	case domain.SubjectHelmValue:
		switch c.Type {
		case domain.ChangeKindDefaultChanged:
			return one(*leaf(domain.OpValuesKey, domain.StateUnset, s.Path), leaf(domain.OpValuesKey, domain.StateSet, s.Path))
		case domain.ChangeKindRemoved, domain.ChangeKindRenamed:
			return one(*leaf(domain.OpValuesKey, domain.StateSet, s.Path), nil)
		case domain.ChangeKindDeprecated:
			if c.ReplacedBy != nil {
				return one(domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
					*leaf(domain.OpValuesKey, domain.StateSet, s.Path),
					*leaf(domain.OpValuesKey, domain.StateUnset, c.ReplacedBy.Path),
				}}, leaf(domain.OpValuesKey, domain.StateSet, c.ReplacedBy.Path))
			}
		}
	case domain.SubjectCRDField:
		var out []domain.Applicability
		for _, withVersion := range []bool{false, true} {
			if withVersion && s.Version == "" {
				continue
			}
			switch c.Type {
			case domain.ChangeKindDefaultChanged:
				out = append(out, domain.Applicability{Exposure: *resourceCond(s, withVersion, domain.StateUnset), Overlap: resourceCond(s, withVersion, domain.StateSet)})
			case domain.ChangeKindRemoved, domain.ChangeKindValidationTightened:
				out = append(out, domain.Applicability{Exposure: *resourceCond(s, withVersion, domain.StateSet)})
			}
		}
		return out
	case domain.SubjectGVK:
		if c.Type == domain.ChangeKindRemoved {
			return one(domain.Condition{Op: domain.OpGVKInUse, Group: s.Group, Version: s.Version, Kind: s.Kind}, nil)
		}
	case domain.SubjectImage:
		if c.Type == domain.ChangeKindRemoved || c.Type == domain.ChangeKindValueChanged {
			return one(domain.Condition{Op: domain.OpImageInUse, Name: s.Name}, nil)
		}
	case domain.SubjectCLIFlag, domain.SubjectEnvVar:
		if c.Type == domain.ChangeKindRemoved || c.Type == domain.ChangeKindRenamed {
			op := domain.OpCLIFlag
			if s.Family == domain.SubjectEnvVar {
				op = domain.OpEnvVar
			}
			return one(domain.Condition{Op: op, Name: s.Name, State: domain.StateSet}, nil)
		}
	case domain.SubjectProductRelationship:
		if c.Type == domain.ChangeKindRequirementChanged && c.After != nil {
			return one(domain.Condition{Op: domain.OpProductVersion, Name: s.Name, State: domain.StateOutOfRange, Range: *c.After}, nil)
		}
	case domain.SubjectCompatibilityBoundary:
		if c.Type == domain.ChangeKindRequirementChanged && c.After != nil {
			return one(domain.Condition{Op: domain.OpClusterVersion, Name: s.Name, State: domain.StateOutOfRange, Range: *c.After}, nil)
		}
	}
	return nil
}
