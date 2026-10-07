package render

import "github.com/tdavison784/release-intelligence/internal/domain"

// RenderabilityOf classifies a (family, change kind) pair (RENDER-MISSION
// R12); the table is data in the domain (domain.RenderabilityOf) so lanes
// that cannot import this package classify identically.
func RenderabilityOf(f domain.SubjectFamily, k domain.ChangeKind) domain.Renderability {
	return domain.RenderabilityOf(f, k)
}

// AssessRenderability classifies an assertion ("" when it states no subject
// or change).
func AssessRenderability(a domain.SemanticAssertion) domain.Renderability {
	return domain.AssessRenderability(a)
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
