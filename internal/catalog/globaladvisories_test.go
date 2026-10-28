package catalog

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func globalAdvisoryDef(loc Locator) *ProductDefinition {
	d := publishedDef()
	d.Sources = append(d.Sources, Source{ID: "global-advisories", Roles: []domain.SourceRole{domain.RoleSecurity}, Locator: loc})
	return d
}

func TestValidateGlobalAdvisoriesLocator(t *testing.T) {
	ok := Locator{Kind: LocatorGitHubGlobalAdvisories, Ecosystem: "go", Package: "example.com/acme"}
	if rep := Validate(globalAdvisoryDef(ok)); !rep.OK() {
		t.Fatalf("valid global-advisories source: %v", rep.Issues)
	}
	for name, tc := range map[string]struct {
		loc Locator
		at  string
	}{
		"missing package":   {Locator{Kind: LocatorGitHubGlobalAdvisories, Ecosystem: "go"}, "sources[1].locator.package"},
		"missing ecosystem": {Locator{Kind: LocatorGitHubGlobalAdvisories, Package: "example.com/acme"}, "sources[1].locator.ecosystem"},
		"unknown ecosystem": {Locator{Kind: LocatorGitHubGlobalAdvisories, Ecosystem: "golang", Package: "example.com/acme"}, "sources[1].locator.ecosystem"},
		"templated package": {Locator{Kind: LocatorGitHubGlobalAdvisories, Ecosystem: "go", Package: "example.com/{{.Tag}}"}, "sources[1].locator.package"},
		"package on another kind": {Locator{Kind: LocatorGitHubAdvisories, Repository: "acme/acme", Ecosystem: "go", Package: "example.com/acme"},
			"sources[1].locator.ecosystem"},
	} {
		if rep := Validate(globalAdvisoryDef(tc.loc)); !hasErrorAt(rep, tc.at) {
			t.Errorf("%s: want an error at %s, got %v", name, tc.at, rep.Issues)
		}
	}
}
