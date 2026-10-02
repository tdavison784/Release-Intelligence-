package catalog

import (
	"strings"
	"testing"
)

func TestReferencedArtifacts(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`{{.ArtifactVersionOf "prometheus-operator-image"}}`, []string{"prometheus-operator-image"}},
		{`v{{.ArtifactVersionOf  "op"}}`, []string{"op"}}, // extra spaces
		{`{{.ArtifactVersionOf "a"}}-{{.ArtifactVersionOf "b"}}/{{.ArtifactVersionOf "a"}}`, []string{"a", "b"}},
		{`{{.ArtifactVersionOf ""}}`, []string{""}}, // malformed kept for validation
		{`{{.ArtifactVersion}}`, nil},               // the plain artifact-channel variable is not a reference
		{`{{.Tag}}`, nil},
		{`{{.ArtifactVersionOf ` + "`op`}}", nil}, // backquoted argument is not recognised
	}
	for _, c := range cases {
		got := ReferencedArtifacts(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: got %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestSourceArtifactRefs(t *testing.T) {
	src := Source{
		Locator: Locator{Kind: LocatorRepoFile, Repository: "example.org/acme/charts",
			Ref: `{{.ArtifactVersionOf "chart"}}`, Path: "docs"},
		Extract: &Extract{Type: ExtractMarkdownSection,
			Heading:  `^operator {{.ArtifactVersionOf "operator-image"}}$`,
			KeyMatch: `{{.ArtifactVersionOf "chart"}}`, // not rendered for this type, but scanned anyway
		},
	}
	// TagPattern is not a rendered field; a reference there is invisible to
	// ingest and must not be reported
	src.Locator.TagPattern = `{{.ArtifactVersionOf "hidden"}}`
	got := SourceArtifactRefs(src)
	want := []string{"chart", "operator-image"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// pinnedDef is a minimal aggregating-product definition: an operator image
// pinned via field and a source following the pin.
func pinnedDef(sourceRef string) *ProductDefinition {
	src := `  - {id: tags, roles: [versions], locator: {kind: git-tags, repository: example.org/acme/charts}}
  - id: operator-notes
    roles: [release-notes]
    locator:
      kind: github-releases
      repository: acme/operator
      ref: '` + sourceRef + `'
artifacts:
  - id: operator-image
    type: container-image
    name: quay.io/acme/prometheus-operator
    version:
      strategy: field
      field: appVersion
      from: {kind: repo-file, repository: example.org/acme/charts, path: Chart.yaml}
    channels:
      - {kind: oci, repository: quay.io/acme/prometheus-operator}
`
	d, err := Parse([]byte(`apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: stack
name: stack
versioning: {scheme: semver, tagPrefix: stack-}
sources:
` + src))
	if err != nil {
		panic(err)
	}
	return d
}

func TestValidateSourceArtifactRefs(t *testing.T) {
	rep := Validate(pinnedDef(`{{.ArtifactVersionOf "operator-image"}}`))
	if !rep.OK() {
		t.Fatalf("valid follow rejected: %v", rep.Issues)
	}

	for _, c := range []struct {
		ref  string
		want string
	}{
		{`{{.ArtifactVersionOf "no-such-artifact"}}`, "follows unknown artifact"},
		{`{{.ArtifactVersionOf ""}}`, "needs a double-quoted artifact id"},
	} {
		rep := Validate(pinnedDef(c.ref))
		if rep.OK() {
			t.Fatalf("%q accepted", c.ref)
		}
		if !strings.Contains(rep.Errors()[0].Message, c.want) {
			t.Fatalf("%q: got %v", c.ref, rep.Issues)
		}
		if p := rep.Errors()[0].Path; p != "sources[1].locator.ref" {
			t.Fatalf("%q: error path %q", c.ref, p)
		}
	}
}

// A source cannot follow an artifact whose version is not derivable from the
// release alone (lookup resolves through channel indexes, independent has no
// relation at all): statically rejected so the source can never exist.
func TestValidateSourceArtifactRefsStrategy(t *testing.T) {
	for _, strategy := range []string{"lookup", "independent"} {
		d := pinnedDef(`{{.ArtifactVersionOf "operator-image"}}`)
		switch strategy {
		case "lookup":
			d.Artifacts[0].Version = VersionRelation{Strategy: VersionLookup, Field: "appVersion", Match: "{{.Tag}}"}
		case "independent":
			d.Artifacts[0].Version = VersionRelation{Strategy: VersionIndependent}
		}
		rep := Validate(d)
		if rep.OK() {
			t.Fatalf("following a %s artifact accepted", strategy)
		}
		if !strings.Contains(rep.Errors()[0].Message, "which a source cannot follow") {
			t.Fatalf("%s: got %v", strategy, rep.Issues)
		}
	}
}

// Artifact templates cannot follow other artifacts' versions (circular with
// the resolution order, self-reference included): the construct is reserved
// to source locators and extracts.
func TestValidateArtifactContextsRejectArtifactRefs(t *testing.T) {
	mutate := map[string]func(d *ProductDefinition){
		"version template (self-reference)": func(d *ProductDefinition) {
			a := &d.Artifacts[0]
			a.Version = VersionRelation{Strategy: VersionTemplate, Template: `{{.ArtifactVersionOf "operator-image"}}`}
		},
		"channel ref": func(d *ProductDefinition) {
			d.Artifacts[0].Channels[0].Repository = `quay.io/acme/{{.ArtifactVersionOf "operator-image"}}`
		},
		"from locator ref": func(d *ProductDefinition) {
			d.Artifacts[0].Version.From.Path = `charts/{{.ArtifactVersionOf "operator-image"}}/Chart.yaml`
		},
		"references pattern ref": func(d *ProductDefinition) {
			d.Artifacts[0].References = []ArtifactReference{{Artifact: "operator-image", Pattern: `x:{{.ArtifactVersionOf "operator-image"}}`}}
		},
		"contents locator ref": func(d *ProductDefinition) {
			d.Artifacts[0].Contents = []Content{{Kind: ContentImageRefs,
				Locator: &Locator{Kind: LocatorRepoFile, Repository: "example.org/acme/charts", Path: `docs/{{.ArtifactVersionOf "operator-image"}}.yaml`}}}
		},
	}
	for name, fn := range mutate {
		d := pinnedDef(`{{.ArtifactVersionOf "operator-image"}}`)
		fn(d)
		rep := Validate(d)
		if rep.OK() {
			t.Fatalf("%s: accepted", name)
		}
		found := false
		for _, i := range rep.Errors() {
			if strings.Contains(i.Message, "source locators and extracts only") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: no rule error in %v", name, rep.Issues)
		}
	}
}

// The render context hands the resolved versions to source templates and
// fails loudly for ids that did not resolve; composition with template
// functions goes through an assignment (a two-result method call cannot be
// nested as an argument).
func TestRenderContextArtifactVersionOf(t *testing.T) {
	rc := RenderContext{Tag: "stack-2.1.0"}
	rc = rc.WithArtifactVersions(map[string]string{"operator-image": "v0.94.1"})
	if got, err := Render(`{{.ArtifactVersionOf "operator-image"}}`, rc); err != nil || got != "v0.94.1" {
		t.Fatalf("follow: %q %v", got, err)
	}
	if _, err := Render(`{{.ArtifactVersionOf "other"}}`, rc); err == nil {
		t.Fatal("unknown id rendered")
	}
	// without versions attached (artifact contexts), any reference fails
	if _, err := Render(`{{.ArtifactVersionOf "operator-image"}}`, rc.WithArtifactVersions(nil)); err == nil {
		t.Fatal("reference without resolved versions rendered")
	}
	composed := `{{$v := .ArtifactVersionOf "operator-image"}}operator {{regexQuote $v}}`
	if got, err := Render(composed, rc); err != nil || got != `operator v0\.94\.1` {
		t.Fatalf("composed: %q %v", got, err)
	}
	if rc.ArtifactVersion != "" {
		t.Fatal("WithArtifactVersions must not touch the plain ArtifactVersion field")
	}
}

// The default context has no versions attached, so the sample contexts used
// by validation (source vs artifact) behave as the construct promises.
func TestSampleContexts(t *testing.T) {
	d := pinnedDef(`{{.ArtifactVersionOf "operator-image"}}`)
	base := sampleContext(d)
	if base.ArtifactVersions != nil {
		t.Fatal("sampleContext must not carry artifact versions")
	}
	if _, err := base.ArtifactVersionOf("operator-image"); err == nil {
		t.Fatal("base context resolved a reference")
	}
	src := sourceSampleContext(base)
	if v, err := src.ArtifactVersionOf("operator-image"); err != nil || v != SampleArtifactVersion {
		t.Fatalf("sourceSampleContext: %q %v", v, err)
	}
	if _, err := src.ArtifactVersionOf(""); err == nil {
		t.Fatal("sourceSampleContext must not answer an empty id")
	}
}
