package domain

// CONTRACT-CHANGE(render): the renderability table (RENDER-MISSION R12) lives
// in the domain as data so every lane can classify a change without
// importing internal/render (which imports internal/knowledge for the
// Validator port): the semantic lane when it builds candidates, the knowledge
// lane's auto-approval policy (AutoApproveRenderVerifiable keys on it), the
// dashboard. internal/render delegates to it.

// renderabilityTable says, per subject family, which change kinds a render
// can show. The renderer is never universal truth: a runtime-only change is
// not wrong because no rendered object shows it.
//
//   - render-verifiable: the effect is a rendered object, field, permission,
//     image, argument, env var, port, label, annotation or API version;
//   - partially-render-verifiable: the effect is visible only through
//     configuration that may or may not be templated (defaults, feature
//     activation, cross-resource relationships);
//   - not-render-verifiable: runtime controller behaviour, protocol
//     semantics, migrations, external services, performance, internal
//     algorithms — every family absent from the table, and every
//     non-structural change kind.
var renderabilityTable = map[SubjectFamily]Renderability{
	SubjectRBACPermission: RenderVerifiable,
	SubjectImage:          RenderVerifiable,
	SubjectCLIFlag:        RenderVerifiable,
	SubjectEnvVar:         RenderVerifiable,
	SubjectGVK:            RenderVerifiable,
	SubjectFeatureGate:    RenderPartiallyVerifiable,
	SubjectHelmValue:      RenderPartiallyVerifiable,
	SubjectCRDField:       RenderPartiallyVerifiable,
	SubjectConfigKey:      RenderPartiallyVerifiable,
}

// structuralChange reports whether a change kind has a rendered shape:
// something appears, disappears, moves or takes another value. Behaviour
// changes, deprecations, tightened validation, requirement moves and
// migrations do not.
func structuralChange(k ChangeKind) bool {
	switch k {
	case ChangeKindAdded, ChangeKindRemoved, ChangeKindRenamed, ChangeKindValueChanged, ChangeKindDefaultChanged:
		return true
	}
	return false
}

// RenderabilityOf classifies a (family, change kind) pair.
func RenderabilityOf(f SubjectFamily, k ChangeKind) Renderability {
	if r, ok := renderabilityTable[f]; ok && structuralChange(k) {
		return r
	}
	return RenderNotVerifiable
}

// AssessRenderability classifies an assertion ("" when it states no subject
// or no change).
func AssessRenderability(a SemanticAssertion) Renderability {
	if a.Subject == nil || a.Change == nil {
		return ""
	}
	return RenderabilityOf(a.Subject.Family, a.Change.Type)
}

// EffectiveRenderability is a candidate's renderability for routing: the
// candidate's own assessment when set, otherwise that of the assertion a
// render-based validation confirmed both subject and change of
// (confirmed-by-render). A candidate no render confirmed stays "" (not
// assessed) — renderability is never inferred from a model claim alone.
func EffectiveRenderability(c SemanticCandidate, vs []ValidationResult) Renderability {
	if c.Renderability != "" {
		return c.Renderability
	}
	for _, v := range vs {
		if v.CandidateID == c.ID && v.RenderRelation == RenderConfirmed && v.Confirms(AspectSubject) && v.Confirms(AspectChange) {
			return AssessRenderability(v.Assertion)
		}
	}
	return ""
}
