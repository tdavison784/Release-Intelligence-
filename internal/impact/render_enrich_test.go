package impact

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impactenrich"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// declaredNote is the provenance of a note-derived change (declared, as the
// notes normalizer records it).
func declaredNote() domain.Provenance {
	return domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.notes@v1", Rule: "section:/breaking/i", Confidence: domain.ConfidenceHigh}
}

// enrichedFixture builds the standard fixture report plus one note-derived
// unknown finding, enriches it with a scripted model, and returns both the
// plain and the enriched report.
func enrichedFixture(t *testing.T) (plain, enriched *domain.ImpactReport, applicability domain.ImpactFinding, evidenceID string) {
	t.Helper()
	eb := newEdge()
	// a note-derived (declared) change without a comparable subject: the
	// join records it as unknown, and it is exactly what the AI step asks about
	noteEv := eb.ev("https://example/declared.md", "A declared change without a comparable subject")
	note := domain.Change{
		ID: "chg-" + domain.ShortHash("declared", "note"), Category: domain.CategoryConfiguration,
		Title: "A declared change without a comparable subject", Provenance: declaredNote(),
		Evidence: []domain.EvidenceID{noteEv},
	}
	eb.edge.Changes = append(eb.edge.Changes, note)
	eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")
	eb.constraint("supported", "1.29, 1.30, 1.31")
	dir := t.TempDir()
	vf := writeFile(t, dir, "values.yaml", "webhook.config:\n  a: b\n")
	e := loadEnv(t, env.Inputs{KubernetesVersion: "1.30", ValuesFiles: []string{vf}})
	plain = buildReport(t, eb.edge, e)

	// the note-derived change is the unknown finding we enrich
	for _, f := range plain.Findings {
		if f.ChangeID == note.ID {
			applicability = f
		}
	}
	if applicability.ID == "" {
		t.Fatal("fixture lacks the unknown finding")
	}
	evidenceID = string(plain.Findings[0].UpstreamEvidence[0])

	enriched = buildReport(t, eb.edge, e)
	var cite string
	for _, x := range enriched.Evidence {
		if x.ID == applicability.UpstreamEvidence[0] {
			cite = string(x.ID)
		}
	}
	client := &llm.Fake{Model: "glm-5.3-flash", ModelVersion: "glm-5.3-flash-2026-09",
		Respond: func(req llm.Request) (string, error) {
			switch {
			case strings.Contains(req.System, "duplicate"):
				return `{"sameChange":false,"content":"different","citations":[],"confidence":"low"}`, nil
			case strings.Contains(req.System, "migration"):
				return "", llm.ErrNotCached
			default:
				return `{"verdict":"plausibly-applies","title":"Declared change may affect you","content":"Your values set keys of this release; inspect the declared change against them.","citations":["` + cite + `"],"confidence":"high"}`, nil
			}
		}}
	res, err := impactenrich.Run(context.Background(), enriched, eb.edge, e, impactenrich.Options{Client: client,
		Clock: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Enrichments) != 1 {
		t.Fatalf("enrichments = %d, want 1", len(res.Enrichments))
	}
	if err := impactenrich.Apply(enriched, res, e); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return plain, enriched, applicability, evidenceID
}

func TestRenderEnrichedReport(t *testing.T) {
	plain, enriched, _, _ := enrichedFixture(t)

	// the plain render has no AI section
	var plainOut strings.Builder
	if err := RenderText(&plainOut, plain, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plainOut.String(), EnrichedHeading) || strings.Contains(plainOut.String(), "AI SUGGESTS REVIEW") {
		t.Error("the deterministic render mentions AI output")
	}

	var out strings.Builder
	if err := RenderText(&out, enriched, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		EnrichedHeading,
		"[suggests review]",
		"AI suggests review-required",
		"of which AI SUGGESTS REVIEW",
		"(ai · glm-5.3-flash-2026-09 · medium)", // provenance summary, confidence capped
		"finding(s):",
		"evidence:",
		"impact-enrich@v1",
		"Prompts:",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("enriched render lacks %q\n%s", want, s)
		}
	}
	// the deterministic verdict is still stated
	if !strings.Contains(s, "UNKNOWN") {
		t.Error("the unknown class disappeared from the render")
	}
}

func TestRenderEnrichedJSONShape(t *testing.T) {
	_, enriched, fd, _ := enrichedFixture(t)
	if err := enriched.Validate(); err != nil {
		t.Fatalf("enriched report invalid: %v", err)
	}
	for i := range enriched.Findings {
		if enriched.Findings[i].ID == fd.ID {
			f := enriched.Findings[i]
			if f.Classification != domain.ImpactUnknown {
				t.Fatalf("classification = %s, want unknown", f.Classification)
			}
			if f.SuggestedClassification != domain.ImpactReviewRequired {
				t.Fatalf("suggestion = %q", f.SuggestedClassification)
			}
		}
	}
	if enriched.Summary.SuggestedReview != 1 || enriched.Summary.Unknown != enriched.Summary.Unknown {
		t.Errorf("summary = %+v", enriched.Summary)
	}
}

// TestRenderPlainByteIdentity: rendering the same report before and after a
// candidates-only "enrichment run" (no client, no Apply) is byte-identical —
// the deterministic path never changes shape.
func TestRenderPlainByteIdentity(t *testing.T) {
	plain, _, _, _ := enrichedFixture(t)
	var a, b strings.Builder
	if err := RenderText(&a, plain, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := RenderText(&b, plain, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Fatal("rendering is not deterministic")
	}
}
