package render

import (
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
