package domain

import (
	"testing"
	"time"
)

func TestVersionParse(t *testing.T) {
	p := VersionParser{Pattern: DefaultTagPattern("v")}
	v, err := p.Parse("v1.18.0")
	if err != nil || v.Semver != "1.18.0" || v.Line() != "1.18" {
		t.Fatalf("got %+v %v", v, err)
	}
	if _, err := p.Parse("cert-manager-1.0"); err == nil {
		t.Fatal("expected mismatch")
	}
	rc, _ := p.Parse("v1.18.0-rc.1")
	if !rc.IsPrerelease() || !rc.Less(v) {
		t.Fatal("prerelease ordering")
	}
	np := VersionParser{Pattern: DefaultTagPattern("")}
	iv, err := np.Parse("1.26.2")
	if err != nil || iv.Tag != "1.26.2" {
		t.Fatalf("istio style: %v", err)
	}
}

func TestSatisfiesTreatsPrereleaseAsItsRelease(t *testing.T) {
	p := VersionParser{Pattern: DefaultTagPattern("v")}
	tests := []struct {
		tag, constraint string
		want            bool
	}{
		{"v1.19.0-alpha.0", ">= 1.5.0", true},
		{"v1.21.0-alpha.1", ">= 1.21.0", true},
		{"v1.21.0-alpha.1", "< 1.21.0", false},
		{"v1.21.0-rc.2", "< 1.22.0", true},
		{"v1.21.0-rc.2", ">= 1.22.0", false},
		{"v1.21.0-rc.2", "", true},
		{"v1.20.3", ">= 1.21.0", false},
		{"v1.21.0", ">= 1.21.0", true},
	}
	for _, tt := range tests {
		v, err := p.Parse(tt.tag)
		if err != nil {
			t.Fatal(err)
		}
		got, err := v.Satisfies(tt.constraint)
		if err != nil || got != tt.want {
			t.Errorf("%s satisfies %q = %v, %v; want %v", tt.tag, tt.constraint, got, err, tt.want)
		}
	}
	if _, err := MustVersion("v1.0.0-rc.1", "1.0.0-rc.1").Satisfies("not a constraint"); err == nil {
		t.Error("invalid constraint must still fail")
	}
}

func TestEdgeValidate(t *testing.T) {
	ev := NewEvidence(EvidenceDocument, "notes", "https://example/notes.md", "L1-L2", "text", "", time.Now())
	e := &UpgradeEdge{
		From: MustVersion("v1.0.0", "1.0.0"), To: MustVersion("v1.1.0", "1.1.0"),
		Evidence: []Evidence{ev},
		Changes: []Change{{ID: "c1", Category: CategoryFeature, Title: "x", Evidence: []EvidenceID{ev.ID},
			Provenance: Provenance{Method: MethodDeclared, Producer: "test", Confidence: ConfidenceHigh}}},
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	e.Changes = append(e.Changes, Change{ID: "c2", Title: "ai", Evidence: []EvidenceID{"ev-missing"},
		Provenance: Provenance{Method: MethodAI, Producer: "x", Confidence: ConfidenceLow}})
	if err := e.Validate(); err == nil {
		t.Fatal("expected validation errors for AI change and missing evidence")
	}
}

func TestEvidenceIDStable(t *testing.T) {
	a := NewEvidence(EvidenceDocument, "s", "u", "l", "x", "d", time.Unix(0, 0))
	b := NewEvidence(EvidenceDocument, "s", "u", "l", "x", "d", time.Unix(100, 0))
	if a.ID != b.ID {
		t.Fatal("evidence id must not depend on retrieval time")
	}
}
