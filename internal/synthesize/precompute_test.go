package synthesize

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func precompute(t *testing.T, in Input) *Precomputed {
	t.Helper()
	p, err := Precompute(in)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func byKind(p *Precomputed) map[domain.JudgementFindingKind][]domain.JudgementFinding {
	m := map[domain.JudgementFindingKind][]domain.JudgementFinding{}
	for _, f := range p.Findings {
		m[f.Kind] = append(m[f.Kind], f)
	}
	return m
}

// Each hard-finding and disagreement kind is produced, with an id derived
// from its content, the same on every computation.
func TestPrecomputeFindingsHaveStableIDs(t *testing.T) {
	f := newFixture()
	f.Report.Security.Candidates[0].Leaves = []domain.AdvisoryRef{{ID: "GHSA-p", Evidence: []domain.EvidenceID{"ev-adv-i"}}}
	p := precompute(t, f.input())
	got := byKind(p)
	// an incomplete advisory picture is its own finding (and then no option
	// is safe, so none is reported still affected)
	inc := precompute(t, newFixtureWith(func(f *fixture) {
		f.Report.Security.Status = domain.SecurityIncomplete
		f.Report.Security.Gaps = []string{"security source not read (vendor)"}
	}).input())
	got[domain.JFSecurityIncomplete] = byKind(inc)[domain.JFSecurityIncomplete]
	for _, k := range domain.JudgementFindingKinds {
		if len(got[k]) == 0 {
			t.Errorf("no %s finding", k)
		}
	}
	for _, x := range p.Findings {
		if x.ID != domain.JudgementFindingID(x.Kind, x.ChangeItem, x.Subject) || x.Hard != x.Kind.Hard() {
			t.Errorf("finding %+v: id or hard flag not derived", x)
		}
	}
	want := map[domain.JudgementFindingKind]string{
		domain.JFSecurityRegression: "GHSA-r", domain.JFConfirmedBreakage: "inf-1", domain.JFFailedValidation: "vr-fail",
		domain.JFUntracedConsumer: "apps/v1/Deployment/ns/web", domain.JFAdvisoryInconsistency: "GHSA-i", domain.JFCandidateAffected: "1.1.2",
	}
	for k, subject := range want {
		if got[k][0].Subject != subject {
			t.Errorf("%s subject = %q, want %q", k, got[k][0].Subject, subject)
		}
	}
	if r := got[domain.JFSecurityRegression][0]; r.ChangeItem != it1 || !strings.Contains(r.Statement, "CVE-2026-1") || len(r.Evidence) != 1 {
		t.Errorf("regression: %+v", r)
	}
	if n := len(got[domain.JFBehaviourUnestablished]); n != 2 {
		t.Errorf("behaviour findings = %d, want the not-checked behaviour and the unnamed unavailable request", n)
	}
	// both directions of disagreement: classifier potentially-affected vs
	// agent not-affected, classifier not-affected vs agent affected
	if n := len(got[domain.JFAgentDisagreement]); n != 2 {
		t.Errorf("disagreements = %d, want 2", n)
	}
	again := precompute(t, newFixtureWith(func(f *fixture) {
		f.Report.Security.Candidates[0].Leaves = []domain.AdvisoryRef{{ID: "GHSA-p", Evidence: []domain.EvidenceID{"ev-adv-i"}}}
	}).input())
	if p.Digest() != again.Digest() || !reflect.DeepEqual(p.Findings, again.Findings) {
		t.Error("pre-computation is not deterministic")
	}
}

func newFixtureWith(mod func(*fixture)) fixture {
	f := newFixture()
	mod(&f)
	return f
}

// An agent verdict equal to the classification, or a difference between
// the two undecided classes, is not a disagreement.
func TestDisagreement(t *testing.T) {
	cases := []struct {
		agent, cls domain.FindingClass
		want       bool
	}{
		{domain.FindingNotAffected, domain.FindingPotentiallyAffected, true},
		{domain.FindingNotAffected, domain.FindingAffected, true},
		{domain.FindingAffected, domain.FindingPotentiallyAffected, true},
		{domain.FindingPotentiallyAffected, domain.FindingInsufficientEvidence, false},
		{domain.FindingAffected, domain.FindingAffected, false},
		{"", domain.FindingAffected, false},
	}
	for _, c := range cases {
		if got := disagree(c.agent, c.cls); got != c.want {
			t.Errorf("disagree(%s, %s) = %v", c.agent, c.cls, got)
		}
	}
}

func option(p *Precomputed, role string) *domain.TargetOption {
	for i := range p.Target.Options {
		if p.Target.Options[i].Role == role {
			return &p.Target.Options[i]
		}
	}
	return nil
}

// PD-3: the latest patch on the chosen line is recommended when it removes
// every regression and adds no assessed advisory; the minimum is always shown;
// otherwise the minimum is recommended.
func TestTargetLatestPatchVersusMinimum(t *testing.T) {
	t.Run("latest qualifies", func(t *testing.T) {
		p := precompute(t, newFixture().input())
		tg := p.Target
		if tg.Minimum == nil || tg.Minimum.Semver != "1.1.2" || tg.Recommended == nil || tg.Recommended.Semver != "1.1.3" {
			t.Fatalf("minimum %v recommended %v, want 1.1.2 and 1.1.3 (the prerelease and the next line are not candidates)", tg.Minimum, tg.Recommended)
		}
		lp := option(p, domain.TargetLatestPatch)
		if lp == nil || !lp.Safe || len(lp.Charts) != 1 || lp.Charts[0].Version != "1.1.3" || !strings.Contains(lp.Statement, "2 assessed advisories") {
			t.Errorf("latest-patch option: %+v", lp)
		}
		if !reflect.DeepEqual(lp.Unassessed, []string{"GHSA-u"}) {
			t.Errorf("unassessed = %v, want the advisory without a range", lp.Unassessed)
		}
		if prop := option(p, domain.TargetProposed); prop == nil || prop.Safe || !reflect.DeepEqual(prop.Introduces, []string{"GHSA-r"}) {
			t.Errorf("proposed option: %+v", prop)
		}
		if m := option(p, domain.TargetMinimum); m == nil || len(m.Charts) != 1 || len(m.Evidence) == 0 {
			t.Errorf("minimum option: %+v", m)
		}
		// the chart evidence of the latest patch is not in the run: the
		// judgement carries it
		found := false
		for _, e := range p.external {
			found = found || e.ID == "ev-chart3"
		}
		if !found {
			t.Error("ev-chart3 is not carried as the judgement's evidence")
		}
	})
	t.Run("latest adds an advisory", func(t *testing.T) {
		f := newFixtureWith(func(f *fixture) {
			f.Releases.Advisories = append(f.Releases.Advisories, domain.Advisory{ID: "GHSA-x", Vulnerable: ">=1.1.3, <1.1.4", Evidence: []domain.EvidenceID{"ev-adv-i"}})
		})
		p := precompute(t, f.input())
		if p.Target.Recommended.Semver != "1.1.2" || option(p, domain.TargetLatestPatch) != nil || !strings.Contains(p.Target.Rule, "GHSA-x") {
			t.Errorf("recommended %v rule %q, want the minimum because 1.1.3 introduces GHSA-x", p.Target.Recommended, p.Target.Rule)
		}
	})
	t.Run("no release list", func(t *testing.T) {
		f := newFixtureWith(func(f *fixture) { f.Releases = nil })
		p := precompute(t, f.input())
		if p.Target.Recommended.Semver != "1.1.2" || !strings.Contains(p.Target.Rule, "not computed") {
			t.Errorf("recommended %v rule %q", p.Target.Recommended, p.Target.Rule)
		}
	})
	t.Run("release facts disagree with the edge", func(t *testing.T) {
		f := newFixtureWith(func(f *fixture) { f.Releases.Advisories[0].Vulnerable = ">=1.0.0, <1.1.2" })
		p := precompute(t, f.input())
		if p.Target.Recommended.Semver != "1.1.2" || !strings.Contains(p.Target.Rule, "do not reproduce") {
			t.Errorf("recommended %v rule %q", p.Target.Recommended, p.Target.Rule)
		}
	})
	t.Run("no regression", func(t *testing.T) {
		f := newFixtureWith(func(f *fixture) {
			f.Report.Security.Regressions, f.Report.Security.Candidates = nil, nil
			f.Releases.Advisories = f.Releases.Advisories[1:]
		})
		p := precompute(t, f.input())
		if p.Target.Minimum.Semver != "1.1.0" || option(p, domain.TargetProposed) == nil || !option(p, domain.TargetProposed).Safe {
			t.Errorf("minimum %v, want the safe proposed target", p.Target.Minimum)
		}
		if p.Target.Recommended.Semver != "1.1.3" {
			t.Errorf("recommended %v, want the latest safe patch of the target's line", p.Target.Recommended)
		}
	})
	t.Run("no safe release", func(t *testing.T) {
		f := newFixtureWith(func(f *fixture) {
			f.Report.Security.Candidates = []domain.VersionCandidate{{Kind: domain.CandidateTargetLine, Line: "1.1", Statement: "none"}}
		})
		p := precompute(t, f.input())
		if p.Target.Minimum != nil || p.Target.Recommended != nil || len(byKind(p)[domain.JFCandidateAffected]) != 1 {
			t.Errorf("target %+v; want no minimum and a no-safe-release finding", p.Target)
		}
	})
	t.Run("incomplete advisory assessment", func(t *testing.T) {
		f := newFixtureWith(func(f *fixture) {
			f.Report.Security.Status, f.Report.Security.Gaps = domain.SecurityIncomplete, []string{"a source was not read"}
		})
		p := precompute(t, f.input())
		for _, o := range p.Target.Options {
			if o.Safe {
				t.Errorf("option %+v is safe while a security source is unread", o)
			}
		}
		if p.Target.Minimum != nil || p.Target.Recommended != nil || !strings.Contains(p.Target.Rule, "incomplete") {
			t.Errorf("target %+v", p.Target)
		}
	})
	t.Run("no advisory source read", func(t *testing.T) {
		f := newFixtureWith(func(f *fixture) {
			f.Report.Security = &domain.EdgeSecurity{SchemaVersion: domain.EdgeSecuritySchemaVersion, Status: domain.SecurityUnavailable, Gaps: []string{"none read"}}
		})
		p := precompute(t, f.input())
		if prop := option(p, domain.TargetProposed); prop.Safe || p.Target.Minimum != nil {
			t.Errorf("proposed %+v: an unread advisory picture never makes a target safe", prop)
		}
	})
}

// Locations come from deterministic facts only: a passing pin, verified
// setting claims and the line stating the deployed version.
func TestLocations(t *testing.T) {
	p := precompute(t, newFixture().input())
	kinds := map[string]domain.ChangeLocation{}
	for _, l := range p.Locations {
		kinds[l.Kind] = l
		if l.ID != domain.ChangeLocationID(l.Kind, l.File, l.Key, l.ChangeItem) {
			t.Errorf("location %+v: id not derived", l)
		}
	}
	want := map[string]domain.ChangeLocation{
		domain.LocationPinnedSetting:   {File: "values/prod.yaml", Line: 7, Key: "feature.mode", Value: `"legacy"`, ChangeItem: it2, Source: "vr-pin"},
		domain.LocationDeployedVersion: {File: "apps/acme.yaml", Line: 12, Key: "$.spec.source.targetRevision", Value: "v1.0.0", Source: "impact-report:environment.products"},
		domain.LocationSettingSet:      {File: "values/prod.yaml", Line: 9, Key: "replicas", Value: "2", ChangeItem: it2, Source: "clm-set"},
		domain.LocationSettingUnset:    {File: "env/values/prod.yaml", Key: "feature.flag", ChangeItem: it2, Source: "clm-unset"},
	}
	for k, w := range want {
		g, ok := kinds[k]
		if !ok {
			t.Errorf("no %s location", k)
			continue
		}
		if g.File != w.File || g.Line != w.Line || g.Key != w.Key || g.Value != w.Value || g.ChangeItem != w.ChangeItem || g.Source != w.Source {
			t.Errorf("%s = %+v, want %+v", k, g, w)
		}
		if k != domain.LocationSettingUnset && len(g.Evidence) == 0 {
			t.Errorf("%s cites no evidence", k)
		}
	}
	if b := p.Backout; b.Location != kinds[domain.LocationDeployedVersion].ID || !strings.Contains(b.Statement, "apps/acme.yaml line 12") || !b.Version.Equal(v("1.0.0")) {
		t.Errorf("backout = %+v", b)
	}
	// A failed pin, another product's version line or a version label is no location.
	f := newFixtureWith(func(f *fixture) {
		f.Inv.Validations[1].Outcome = domain.ValidationFail
		f.Report.Environment.Products[0].Source = "detected-helm"
	})
	for _, l := range precompute(t, f.input()).Locations {
		if l.Kind == domain.LocationPinnedSetting || l.Kind == domain.LocationDeployedVersion {
			t.Errorf("unexpected location %+v", l)
		}
	}
}

// Items that change the same rendered object are flagged; a classified item
// left unresolved is a deterministic residual risk; the per-item digest
// carries the assessment's headline, configuration, preconditions and tests.
func TestInteractionsRisksAndDigests(t *testing.T) {
	p := precompute(t, newFixture().input())
	if len(p.Interactions) != 1 || p.Interactions[0].Shared != "apps/v1/Deployment/ns/acme" || !reflect.DeepEqual(p.Interactions[0].Items, []string{it1, it2}) {
		t.Errorf("interactions = %+v", p.Interactions)
	}
	if len(p.ResidualRisks) != 1 || p.ResidualRisks[0].ChangeItem != it2 || p.ResidualRisks[0].Source != domain.RiskDeterministic {
		t.Errorf("risks = %+v", p.ResidualRisks)
	}
	var d ItemDigest
	for _, x := range p.Items {
		if x.ChangeItem == it2 {
			d = x
		}
	}
	if d.Headline == "" || len(d.Effective) != 1 || len(d.Preconditions) != 1 || len(d.Tests) != 2 || d.AgentVerdict != domain.FindingNotAffected {
		t.Errorf("digest = %+v", d)
	}
	var transition bool
	for _, c := range p.Tests {
		transition = transition || c.Source == "target:1.1.3"
	}
	if !transition {
		t.Error("no transition test for the latest-patch option")
	}
	// The model's input is generic data: it names no case, and the
	// investigation's own records are referenced by id.
	raw, _ := json.Marshal(p)
	if !json.Valid(raw) || len(p.Evidence) == 0 {
		t.Error("input is not citable JSON")
	}
}

func TestPrecomputeNeedsAClassifiedInvestigation(t *testing.T) {
	f := newFixture()
	f.Inv.Decision = nil
	if _, err := Precompute(f.input()); err == nil {
		t.Error("an unclassified investigation was accepted")
	}
}
