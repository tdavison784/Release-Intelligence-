package render

import "github.com/tdavison784/release-intelligence/internal/domain"

// Renderability (RENDER-MISSION R12) is data: per subject family, which
// change kinds a render can show. The renderer is never universal truth —
// a runtime-only change is not wrong because no rendered object shows it.
//
//   - render-verifiable: the effect is a rendered object, field, permission,
//     image, argument, env var, port, label, annotation or API version;
//   - partially-render-verifiable: the effect is visible only through
//     configuration that may or may not be templated (defaults, feature
//     activation, cross-resource relationships);
//   - not-render-verifiable: runtime controller behaviour, protocol semantics,
//     migrations, external services, performance, internal algorithms.
var renderabilityTable = map[domain.SubjectFamily]map[domain.ChangeKind]domain.Renderability{
	domain.SubjectRBACPermission: structural(domain.RenderVerifiable),
	domain.SubjectImage:          structural(domain.RenderVerifiable),
	domain.SubjectCLIFlag:        structural(domain.RenderVerifiable),
	domain.SubjectEnvVar:         structural(domain.RenderVerifiable),
	domain.SubjectGVK:            structural(domain.RenderVerifiable),
	domain.SubjectFeatureGate:    structural(domain.RenderPartiallyVerifiable),
	domain.SubjectHelmValue:      structural(domain.RenderPartiallyVerifiable),
	domain.SubjectCRDField:       structural(domain.RenderPartiallyVerifiable),
	domain.SubjectConfigKey:      structural(domain.RenderPartiallyVerifiable),
}

// structural maps the structural change kinds (added, removed, renamed,
// value/default changed) to r; every other kind (behaviour changes,
// deprecations, validation, requirements, migrations) has no rendered
// shape and is not render-verifiable.
func structural(r domain.Renderability) map[domain.ChangeKind]domain.Renderability {
	return map[domain.ChangeKind]domain.Renderability{
		domain.ChangeKindAdded: r, domain.ChangeKindRemoved: r, domain.ChangeKindRenamed: r,
		domain.ChangeKindValueChanged: r, domain.ChangeKindDefaultChanged: r,
	}
}

// RenderabilityOf classifies a (family, change kind) pair. Families absent
// from the table (protocol behaviour, API endpoints, migrations, product
// relationships, compatibility boundaries, Terraform) are not render-verifiable.
func RenderabilityOf(f domain.SubjectFamily, k domain.ChangeKind) domain.Renderability {
	if r, ok := renderabilityTable[f][k]; ok {
		return r
	}
	return domain.RenderNotVerifiable
}

// AssessRenderability classifies an assertion ("" when it states no subject
// or change).
func AssessRenderability(a domain.SemanticAssertion) domain.Renderability {
	if a.Subject == nil || a.Change == nil {
		return ""
	}
	return RenderabilityOf(a.Subject.Family, a.Change.Type)
}

// RenderedClasses lists the rendered change classes eligible for automatic
// approval under the default policy (R10): structural, render-verifiable
// effects.
var RenderedClasses = map[ChangeClass]bool{
	ResourceAdded: true, ResourceRemoved: true, FieldAdded: true, FieldRemoved: true, FieldChanged: true,
	ImageChanged: true, RBACPermissionAdded: true, RBACPermissionRemoved: true,
	ContainerArgAdded: true, ContainerArgRemoved: true, ContainerArgChanged: true,
	EnvVarAdded: true, EnvVarRemoved: true, EnvVarChanged: true,
	ServicePortAdded: true, ServicePortRemoved: true, ServicePortChanged: true,
}
