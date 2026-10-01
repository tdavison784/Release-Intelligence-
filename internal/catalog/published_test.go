package catalog

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// publishedDef is a definition whose chart contents come from the published
// package (channel fallback, no content locators) with a compareWith guard.
func publishedDef() *ProductDefinition {
	tgz := Locator{Kind: LocatorChartTGZ, URL: "https://github.com/acme/charts/releases/download/{{.Tag}}/acme-{{.ArtifactVersion}}.tgz"}
	return &ProductDefinition{
		APIVersion: APIVersion, Kind: Kind, ID: "acme", Name: "Acme",
		Versioning: Versioning{Scheme: domain.SchemeSemver, TagPrefix: "v"},
		Sources: []Source{{ID: "tags", Roles: []domain.SourceRole{domain.RoleVersions},
			Locator: Locator{Kind: LocatorGitTags, Repository: "github.com/acme/charts"}}},
		Artifacts: []Artifact{{
			ID: "chart", Type: domain.ArtifactHelmChart, Name: "acme",
			Version:  VersionRelation{Strategy: VersionTemplate, Template: "{{.Version}}"},
			Channels: []Locator{tgz},
			Contents: []Content{
				{Kind: ContentHelmValues,
					CompareWith: &Locator{Kind: LocatorRepoFile, Repository: "github.com/acme/charts", Ref: "{{.Tag}}", Path: "charts/acme/values.yaml"}},
			},
		}},
	}
}

func TestValidateChartTGZChannel(t *testing.T) {
	rep := Validate(publishedDef())
	if !rep.OK() {
		t.Fatalf("chart-tgz channel definition must validate: %v", rep.Issues)
	}
}

func TestValidateChartTGZRequiresURL(t *testing.T) {
	d := publishedDef()
	d.Artifacts[0].Channels[0] = Locator{Kind: LocatorChartTGZ}
	rep := Validate(d)
	if !hasErrorAt(rep, "artifacts[0].channels[0].url") {
		t.Fatalf("chart-tgz without url must fail: %v", rep.Issues)
	}
}

func TestValidateChartTGZWrongType(t *testing.T) {
	d := publishedDef()
	d.Artifacts[0].Type = domain.ArtifactContainerImage
	rep := Validate(d)
	if !hasErrorAt(rep, "artifacts[0].channels[0].kind") {
		t.Fatalf("chart-tgz must not be a channel of a container image: %v", rep.Issues)
	}
}

func TestValidateContentPackageChannelFallback(t *testing.T) {
	// contents without a locator are servable by a packaged-chart channel
	for _, kind := range []string{LocatorHelmRepo, LocatorOCI} {
		d := publishedDef()
		d.Artifacts[0].Channels = []Locator{{Kind: kind, URL: "https://charts.acme.example", Chart: "acme", Repository: "ghcr.io/acme/charts/acme"}}
		rep := Validate(d)
		if hasErrorAt(rep, "artifacts[0].contents[0].locator") {
			t.Errorf("%s channel should serve contents without a locator: %v", kind, rep.Issues)
		}
	}
	// a content of an artifact with no servable channel needs its own locator
	d := publishedDef()
	d.Artifacts[0].Channels = []Locator{{Kind: LocatorGitHubReleases, Repository: "acme/charts"}}
	rep := Validate(d)
	if !hasErrorAt(rep, "artifacts[0].contents[0].locator") {
		t.Fatalf("unservable channel must require a content locator: %v", rep.Issues)
	}
}

func TestValidateCompareWith(t *testing.T) {
	// comparing the content with itself is a warning, not an error
	d := publishedDef()
	self := Locator{Kind: LocatorChartTGZ, URL: "https://github.com/acme/charts/releases/download/{{.Tag}}/acme-{{.ArtifactVersion}}.tgz"}
	d.Artifacts[0].Contents[0].CompareWith = &self
	rep := Validate(d)
	if !rep.OK() {
		t.Fatalf("self comparison should only warn: %v", rep.Issues)
	}
	found := false
	for _, i := range rep.Issues {
		if i.Severity == SeverityWarning && i.Path == "artifacts[0].contents[0].compareWith" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a self-comparison warning: %v", rep.Issues)
	}

	// an invalid locator inside compareWith is an error
	bad := Locator{Kind: LocatorRepoFile, Path: "values.yaml"}
	d.Artifacts[0].Contents[0].CompareWith = &bad
	rep = Validate(d)
	if !hasErrorAt(rep, "artifacts[0].contents[0].compareWith.repository") {
		t.Fatalf("compareWith locator must be validated: %v", rep.Issues)
	}
}

func hasErrorAt(rep ValidationReport, path string) bool {
	for _, i := range rep.Errors() {
		if i.Path == path || strings.HasPrefix(i.Path, path+".") {
			return true
		}
	}
	return false
}
