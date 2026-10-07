package semantic

import (
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// edgeBuilder assembles a synthetic, valid upgrade edge.
type edgeBuilder struct {
	e domain.UpgradeEdge
}

func newEdge() *edgeBuilder {
	return &edgeBuilder{e: domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "widget-operator", Name: "Widget Operator"},
		From:          domain.Version{Tag: "v1.0.0", Semver: "1.0.0"},
		To:            domain.Version{Tag: "v1.1.0", Semver: "1.1.0"},
		GeneratedAt:   t0,
	}}
}

func (b *edgeBuilder) ev(kind domain.EvidenceKind, uri, locator, excerpt string) domain.EvidenceID {
	e := domain.NewEvidence(kind, "src", uri, locator, excerpt, "", t0)
	for _, x := range b.e.Evidence {
		if x.ID == e.ID {
			return e.ID
		}
	}
	b.e.Evidence = append(b.e.Evidence, e)
	return e.ID
}

const notesURI = "https://example.io/widget/releases/v1.1.0.md"
const guideURI = "https://example.io/widget/upgrading-1.0-1.1.md"

// note adds a note-derived change quoting excerpt from uri.
func (b *edgeBuilder) note(id, title, uri, locator string, mod ...func(*domain.Change)) *domain.Change {
	evID := b.ev(domain.EvidenceDocument, uri, locator, title)
	c := domain.Change{ID: id, Category: domain.CategoryConfiguration, Title: title, Release: "1.1.0",
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: normalize.ProducerNotes, Rule: "section:/x/", Confidence: domain.ConfidenceHigh},
		Evidence:   []domain.EvidenceID{evID}}
	for _, m := range mod {
		m(&c)
	}
	b.e.Changes = append(b.e.Changes, c)
	return &b.e.Changes[len(b.e.Changes)-1]
}

// computed adds a computed diff change.
func (b *edgeBuilder) computed(id, rule, title string, subjects ...string) {
	evID := b.ev(domain.EvidenceStructured, "https://example.io/widget/v1.1.0/values.yaml", "", "")
	b.e.Changes = append(b.e.Changes, domain.Change{ID: id, Category: domain.CategoryHelmValues, Title: title, Subjects: subjects,
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: rule, Confidence: domain.ConfidenceHigh},
		Evidence:   []domain.EvidenceID{evID}})
}

func (b *edgeBuilder) edge() *domain.UpgradeEdge { e := b.e; return &e }

// rotationEdge is a small realistic edge: one change restated three times,
// a values diff a note names, a feature-gate deprecation, a routine bump, a
// security fix, an umbrella and a computed CRD field addition.
func rotationEdge() *domain.UpgradeEdge {
	b := newEdge()
	b.note("chg-guide", "We have changed the default value of `Widget.Spec.Rotation.Policy` from `Never` to `Always`.", guideURI, "L8-L10",
		func(c *domain.Change) { c.Breaking = true; c.Category = domain.CategoryMigration })
	b.note("chg-theme", "The default value of `Widget.Spec.Rotation.Policy` is now `Always`: We have changed the default value of `Widget.Spec.Rotation.Po…", notesURI, "L20-L40")
	b.note("chg-item", "The default value of `Widget.Spec.Rotation.Policy` changed from `Never` to `Always`", notesURI, "L120")
	b.note("chg-helm", "Adds the `global.rbac.disableChallenges` helm value to disable challenges", notesURI, "L121", func(c *domain.Change) { c.Category = domain.CategoryFeature })
	b.computed("chg-values-added", upgrade.RuleValuesAdded, "New Helm value `global.rbac.disableChallenges`", "global.rbac.disableChallenges")
	b.note("chg-gate", "The `ValidateThing` feature gate has been deprecated and will be removed in v1.3", notesURI, "L130", func(c *domain.Change) { c.Category = domain.CategoryDeprecation })
	b.note("chg-bump", "Bump golang.org/x/net to v0.30.0", notesURI, "L140", func(c *domain.Change) { c.Routine = true; c.RoutineKind = "dependency" })
	b.note("chg-cve", "Bump `golang.org/x/crypto` to patch `CVE-2025-22869`", notesURI, "L141", func(c *domain.Change) { c.Category = domain.CategorySecurity })
	b.note("chg-umbrella", "API Moves", guideURI, "L50-L60", func(c *domain.Change) { c.Detail = "- ExpireAfter moved\n- Consolidation renamed\n" })
	b.e.Changes = append(b.e.Changes, domain.Change{ID: "chg-crd-added", Category: domain.CategoryCRDSchema,
		Title: "Widget v1 schema: 1 field added: `spec.signatureAlgorithm`", Subjects: []string{"spec.signatureAlgorithm"},
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: upgrade.RuleCRDFieldsAdded, Confidence: domain.ConfidenceHigh},
		Evidence:   []domain.EvidenceID{b.ev(domain.EvidenceStructured, "https://example.io/widget/v1.1.0/crds.yaml", "", "")}})
	b.e.Changes = append(b.e.Changes, domain.Change{ID: "chg-crd-multi", Category: domain.CategoryCRDSchema,
		Title: "Gadget v1 schema: 2 fields added: `spec.a`, `spec.b`", Subjects: []string{"spec.a", "spec.b"},
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: upgrade.RuleCRDFieldsAdded, Confidence: domain.ConfidenceHigh},
		Evidence:   []domain.EvidenceID{b.ev(domain.EvidenceStructured, "https://example.io/widget/v1.1.0/crds.yaml", "", "")}})
	return b.edge()
}

func candidateFor(t interface{ Fatalf(string, ...any) }, cands []domain.SemanticCandidate, changeID string) domain.SemanticCandidate {
	for _, c := range cands {
		for _, m := range c.Members {
			if m.ChangeID == changeID {
				return c
			}
		}
	}
	t.Fatalf("no candidate has member %s", changeID)
	return domain.SemanticCandidate{}
}
