package render

import (
	"slices"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// A wildcard permission ("*" resources, "*" verbs) or a punctuation-only name
// is never "mentioned" by changelog text: markdown bullets are full of "*".
func TestCorrelateIgnoresWildcardsAndPunctuation(t *testing.T) {
	ev := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.org/n", "L1", "* Fixed a bug\n* Added - a thing", "sha256:n", fixedNow)
	edge := &domain.UpgradeEdge{Evidence: []domain.Evidence{ev}, Changes: []domain.Change{{ID: "chg-1", Title: "* Fixed a bug * in -", Evidence: []domain.EvidenceID{ev.ID}}}}
	wild := Permission{Group: "*", Resource: "*", Verb: "*"}
	d := &DiffResult{Changes: []Change{
		{Class: RBACPermissionAdded, Object: ObjectID{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole", Name: "x"}, Name: "*/*", Permission: &wild},
		{Class: ContainerArgAdded, Object: ObjectID{Group: "apps", Version: "v1", Kind: "Deployment", Name: "x"}, Name: "-"},
	}}
	if c := Correlate(d, edge); c.Documented != 0 {
		t.Fatalf("wildcards/punctuation correlated: %+v", c.Changes)
	}
	if !mentions("removed the `--enable-x` flag", "--enable-x") || mentions("a * b", "*") {
		t.Error("mentions")
	}
}

// CRD description text links only through the field it documents
// (documents-field): never through the word "description", and never to a
// change that does not name the field.
func TestCorrelateCRDDescriptionByDocumentedField(t *testing.T) {
	desc := func(field string) Change {
		return Change{Class: CRDDescriptionChanged, Object: ObjectID{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition", Name: "widgets.demo.example.org"},
			Path: "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties." + field + ".description", Name: "description"}
	}
	ev := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.org/n", "L1",
		"Reworded the description of the CRD schema for clarity; the default of Widget.spec.rotationMode is now Always.", "sha256:n", fixedNow)
	edge := &domain.UpgradeEdge{Evidence: []domain.Evidence{ev}, Changes: []domain.Change{
		{ID: "chg-docs", Title: "Reworded the CRD description", Subjects: []string{"description"}},
		{ID: "chg-rotation", Title: "Default rotation changed", Subjects: []string{"spec.rotationMode"}},
		{ID: "chg-note", Title: "Defaults changed", Evidence: []domain.EvidenceID{ev.ID}},
	}}
	d := &DiffResult{Changes: []Change{desc("rotationMode"), desc("windowSize"), desc("mode")}}
	c := Correlate(d, edge)
	got := map[string][]string{}
	for _, cc := range c.Changes {
		for _, l := range cc.Links {
			if l.Rule != "documents-field" {
				t.Errorf("%s linked by %s: only documents-field may link documentation", cc.Change.Path, l.Rule)
			}
			got[DocumentedField(cc.Change)] = append(got[DocumentedField(cc.Change)], l.ChangeID)
		}
	}
	if want := []string{"chg-note", "chg-rotation"}; !slices.Equal(got["rotationMode"], want) {
		t.Errorf("rotationMode documentation links %v, want %v (subject and evidence text)", got["rotationMode"], want)
	}
	if len(got["windowSize"]) != 0 || len(got["mode"]) != 0 {
		t.Errorf("documentation of unnamed fields linked: %v (a field of < 6 characters never counts)", got)
	}
}
