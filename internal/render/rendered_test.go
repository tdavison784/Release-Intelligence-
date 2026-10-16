package render

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestRenderedPath(t *testing.T) {
	cases := map[string]Change{
		"spec.template.spec.containers[name=c].args[=--flag]": {Class: ContainerArgAdded, Path: "spec.template.spec.containers[name=c].args", Name: "--flag"},
		"rules[=apps/deployments:get]":                        {Class: RBACPermissionRemoved, Path: "rules", Name: "deployments/get", Permission: &Permission{Group: "apps", Resource: "deployments", Verb: "get"}},
		"spec.replicas":                                       {Class: FieldChanged, Path: "spec.replicas"},
	}
	for want, c := range cases {
		if got := RenderedPath(c); got != want {
			t.Errorf("RenderedPath = %q, want %q", got, want)
		}
	}
}

func TestSelfArtifactBump(t *testing.T) {
	inst := func(coord string) *domain.ArtifactInstance {
		return &domain.ArtifactInstance{Type: domain.ArtifactContainerImage, Coordinate: coord}
	}
	edge := &domain.UpgradeEdge{Artifacts: []domain.ArtifactChange{
		{Type: domain.ArtifactContainerImage, From: inst("quay.io/x/controller:v1.17.2"), To: inst("quay.io/x/controller:v1.18.0")},
		{Type: domain.ArtifactContainerImage, From: inst("quay.io/x/solver:v1.17.2"), To: inst("quay.io/x/solver:v1.18.0")},
	}}
	q := func(s string) *string { v := `"` + s + `"`; return &v }
	cases := []struct {
		name string
		c    Change
		want bool
	}{
		{"image bump", Change{Class: ImageChanged, Before: q("quay.io/x/controller:v1.17.2"), After: q("quay.io/x/controller:v1.18.0")}, true},
		{"image arg bump", Change{Class: ContainerArgChanged, Name: "--solver-image", Before: q("--solver-image=quay.io/x/solver:v1.17.2"), After: q("--solver-image=quay.io/x/solver:v1.18.0")}, true},
		{"other tag", Change{Class: ImageChanged, Before: q("quay.io/x/controller:v1.17.2"), After: q("quay.io/x/controller:v1.18.1")}, false},
		{"mirror", Change{Class: ImageChanged, Before: q("mirror.io/x/controller:v1.17.2"), After: q("mirror.io/x/controller:v1.18.0")}, false},
		{"repository moved", Change{Class: ImageChanged, Before: q("quay.io/x/old:v1.17.2"), After: q("quay.io/x/controller:v1.18.0")}, false},
		{"not the product's", Change{Class: ImageChanged, Before: q("nginx:1.25"), After: q("nginx:1.27")}, false},
		{"plain arg", Change{Class: ContainerArgChanged, Name: "--v", Before: q("--v=1"), After: q("--v=2")}, false},
		{"field", Change{Class: FieldChanged, Before: q("quay.io/x/controller:v1.17.2"), After: q("quay.io/x/controller:v1.18.0")}, false},
	}
	for _, c := range cases {
		if _, got := SelfArtifactBump(c.c, edge); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
