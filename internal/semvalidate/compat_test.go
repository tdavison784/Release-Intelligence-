package semvalidate

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func boundary(name string) *domain.Subject {
	return &domain.Subject{Family: domain.SubjectCompatibilityBoundary, Product: "demo", Name: name}
}

func TestCompatValidator(t *testing.T) {
	from := newRel("v1.0.0").compat("kubernetes", "supported", "1.29 → 1.33").compat("kafka", "supported", "3.8, 3.9")
	to := newRel("v1.1.0").compat("kubernetes", "supported", "1.30 → 1.34").compat("kafka", "supported", "3.9, 4.0").compat("openshift", "minimum", "4.16")
	O, R, I := domain.OutcomeConfirmed, domain.OutcomeRefuted, domain.OutcomeInconclusive
	cases := []struct {
		name            string
		platform        string
		chg             *domain.ChangeSpec
		subject, change domain.ValidationOutcome
	}{
		{"range moved", "kubernetes", change(domain.ChangeKindRequirementChanged, "", ">=1.30.0-0, <1.35.0-0"), O, O},
		{"equivalent spelling", "kubernetes", change(domain.ChangeKindRequirementChanged, ">=1.29.0-0, <1.34.0-0", "1.30 - 1.34"), O, O},
		{"operand set moved", "kafka", change(domain.ChangeKindRequirementChanged, "", "3.9 || 4.0"), O, O},
		{"too wide: open-ended where the table is closed", "kubernetes", change(domain.ChangeKindRequirementChanged, "", ">=1.30"), O, R},
		{"wrong lower bound", "kubernetes", change(domain.ChangeKindRequirementChanged, "", ">=1.31.0-0, <1.35.0-0"), O, R},
		{"wrong before", "kubernetes", change(domain.ChangeKindRequirementChanged, ">=1.20", "1.30 - 1.34"), O, R},
		{"the requirement did not move", "openshift", change(domain.ChangeKindRequirementChanged, "", ">=4.16"), O, I},
		{"platform without rows", "nomad", change(domain.ChangeKindRequirementChanged, "", ">=1.0"), I, I},
		{"other kinds are not decidable", "kubernetes", change(domain.ChangeKindBehaviorChanged, "", ""), O, I},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, compatValidator{}, input(from, to, assertion(boundary(tc.platform), tc.chg)))
			if got[domain.AspectSubject] != tc.subject || got[domain.AspectChange] != tc.change {
				t.Errorf("subject/change = %s/%s, want %s/%s", got[domain.AspectSubject], got[domain.AspectChange], tc.subject, tc.change)
			}
		})
	}
}

func TestCompatIdenticalRowsRefuteARequirementChange(t *testing.T) {
	from := newRel("v1.0.0").compat("kubernetes", "supported", "1.29 → 1.33")
	to := newRel("v1.1.0").compat("kubernetes", "supported", "1.29 - 1.33")
	got := run(t, compatValidator{}, input(from, to, assertion(boundary("kubernetes"), change(domain.ChangeKindRequirementChanged, "", "1.29 - 1.33"))))
	if got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("unchanged range: %v", got)
	}
}

// minimum/maximum columns combine into one requirement; both spellings compare equal.
func TestCompatMinimumMaximumRows(t *testing.T) {
	from := newRel("v1.0.0").compat("kubernetes", "minimum", "1.28").compat("kubernetes", "maximum", "1.31")
	to := newRel("v1.1.0").compat("kubernetes", "minimum", "1.29").compat("kubernetes", "maximum", "1.33")
	got := run(t, compatValidator{}, input(from, to, assertion(boundary("kubernetes"), change(domain.ChangeKindRequirementChanged, "", "1.29 - 1.33"))))
	if got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("min/max rows: %v", got)
	}
}

func TestProductRelationshipUsesTheSameRows(t *testing.T) {
	from := newRel("v1.0.0").compat("ingress-nginx", "minimum", "1.8")
	to := newRel("v1.1.0").compat("ingress-nginx", "minimum", "1.10")
	s := &domain.Subject{Family: domain.SubjectProductRelationship, Product: "demo", Name: "ingress-nginx"}
	got := run(t, compatValidator{}, input(from, to, assertion(s, change(domain.ChangeKindRequirementChanged, "", ">=1.10"))))
	if got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("product relationship: %v", got)
	}
}

func TestImageValidator(t *testing.T) {
	from := newRel("v1.0.0").images("manifest", "quay.io/acme/ctl:v1.0.0", "docker.io/library/busybox:1.35", "quay.io/acme/old:1")
	to := newRel("v1.1.0").images("manifest", "quay.io/acme/ctl:v1.1.0", "busybox:1.36", "quay.io/acme/new:1")
	img := func(n string) *domain.Subject {
		return &domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: n}
	}
	O, R, I := domain.OutcomeConfirmed, domain.OutcomeRefuted, domain.OutcomeInconclusive
	for _, tc := range []struct {
		name, repo      string
		chg             *domain.ChangeSpec
		subject, change domain.ValidationOutcome
	}{
		{"removed", "quay.io/acme/old", change(domain.ChangeKindRemoved, "", ""), O, O},
		{"removed but still referenced", "quay.io/acme/ctl", change(domain.ChangeKindRemoved, "", ""), O, R},
		{"added", "quay.io/acme/new", change(domain.ChangeKindAdded, "", ""), O, O},
		{"tag changed", "quay.io/acme/ctl", change(domain.ChangeKindValueChanged, `"v1.0.0"`, `"v1.1.0"`), O, O},
		{"tag changed, registry spelling differs", "busybox", change(domain.ChangeKindValueChanged, `"1.35"`, `"1.36"`), O, O},
		{"wrong after tag", "quay.io/acme/ctl", change(domain.ChangeKindValueChanged, `"v1.0.0"`, `"v9"`), O, R},
		{"not referenced anywhere is not a refutation", "quay.io/acme/ghost", change(domain.ChangeKindRemoved, "", ""), I, R},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, imageValidator{}, input(from, to, assertion(img(tc.repo), tc.chg)))
			if got[domain.AspectSubject] != tc.subject || got[domain.AspectChange] != tc.change {
				t.Errorf("subject/change = %s/%s, want %s/%s", got[domain.AspectSubject], got[domain.AspectChange], tc.subject, tc.change)
			}
		})
	}
	// no image snapshot at all
	if got := run(t, imageValidator{}, input(newRel("v1.0.0"), newRel("v1.1.0"), assertion(img("x"), change(domain.ChangeKindRemoved, "", "")))); got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("no snapshots: %v", got)
	}
}
