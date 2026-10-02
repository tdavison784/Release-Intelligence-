package knowledge

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestRenderEvidenceOfPicksTheMostDecisive(t *testing.T) {
	rel := func(id string, r domain.RenderRelation, cand string) domain.ValidationResult {
		return domain.ValidationResult{ID: id, CandidateID: cand, RenderRelation: r,
			Checks: []domain.AspectCheck{{Aspect: domain.AspectChange, Detail: id}}}
	}
	vs := []domain.ValidationResult{
		rel("val-a", domain.RenderNotVisible, "sc-1"),
		rel("val-b", domain.RenderConfirmed, "sc-1"),
		rel("val-c", domain.RenderContradicted, "sc-2"), // another candidate
		{ID: "val-d", CandidateID: "sc-1"},              // not render-based
	}
	re := RenderEvidenceOf("sc-1", vs)
	if re == nil || re.Validation != "val-b" || re.Relation != domain.RenderConfirmed || re.Explanation != "val-b" {
		t.Fatalf("picked %+v", re)
	}
	vs = append(vs, rel("val-e", domain.RenderContradicted, "sc-1"))
	if re := RenderEvidenceOf("sc-1", vs); re.Validation != "val-e" {
		t.Errorf("a contradiction must win: %+v", re)
	}
	if RenderEvidenceOf("sc-9", vs) != nil {
		t.Error("no render validation ⇒ nil")
	}
}
