package impact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// Facts in these tests are hand-built from the contract's own examples
// (DESIGN.md §10), never derived from eval expectations.

// note adds a note-derived change quoting excerpt from uri.
func (b *edgeBuilder) note(uri, title, excerpt string, subjects ...string) domain.Change {
	e := domain.NewEvidence(domain.EvidenceDocument, "notes", uri, "L10", excerpt, domain.Digest([]byte(uri)), testNow)
	b.edge.Evidence = append(b.edge.Evidence, e)
	c := domain.Change{
		ID: "chg-" + domain.ShortHash(uri, title), Category: domain.CategoryConfiguration, Title: title, Release: "1.1.0",
		Subjects:   subjects,
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "notes@v1", Confidence: domain.ConfidenceHigh},
		Evidence:   []domain.EvidenceID{e.ID},
	}
	b.edge.Changes = append(b.edge.Changes, c)
	return c
}

func ptr(s string) *string { return &s }

// fact builds a valid VerifiedFact at one verification level for every aspect.
func fact(t *testing.T, a domain.SemanticAssertion, level domain.VerificationLevel, anchors ...domain.ChangeAnchor) domain.VerifiedFact {
	t.Helper()
	f := domain.VerifiedFact{
		Product: "example", Release: "1.1.0", Anchors: anchors, Candidates: []string{"sc-000000000001"},
		Assertion: a, Status: domain.FactActive, CreatedAt: testNow,
		Evidence: []domain.Evidence{domain.NewEvidence(domain.EvidenceDocument, "docs", "https://example/upgrade-guide.md", "L42", "fact evidence: "+a.Statement, domain.Digest([]byte("guide")), testNow)},
	}
	for _, x := range domain.Aspects {
		v := domain.AspectVerification{Aspect: x, Level: level}
		switch level {
		case domain.VerifiedDeterministic:
			v.Basis = []string{"val-" + string(x)}
		case domain.VerifiedConsensus:
			v.Basis = []string{"sp-a" + string(x), "sp-b" + string(x)}
			v.Consensus = domain.ConsensusCrossModel
		default:
			v.Basis = []string{"rd-" + string(x)}
		}
		f.Verification = append(f.Verification, v)
	}
	f.AutoApproved = level == domain.VerifiedDeterministic || level == domain.VerifiedConsensus
	f.ID = domain.VerifiedFactID(f.Product, f.Release, f.Assertion)
	if err := f.Validate(); err != nil {
		t.Fatalf("test fact is invalid: %v", err)
	}
	return f
}

// rotationFact is DESIGN.md §10 path 1: a default change whose consequence is
// a behaviour change (REVIEW, D11).
func rotationAssertion() domain.SemanticAssertion {
	return domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "example", Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: ptr(`"Never"`), After: ptr(`"Always"`)},
		Applicability: &domain.Applicability{
			Exposure: certScope(field("spec.privateKey.rotationPolicy", domain.StateUnset)),
			Overlap:  ptrCond(certScope(field("spec.privateKey.rotationPolicy", domain.StateSet))),
		},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired,
			Statement: "Private keys of Certificates without an explicit rotationPolicy are rotated on every renewal."},
		Statement: "The default Certificate.spec.privateKey.rotationPolicy changed from Never to Always.",
	}
}

func ptrCond(c domain.Condition) *domain.Condition { return &c }

// http01Assertion is DESIGN.md §10 path 2: a cross-product failure.
func http01Assertion() domain.SemanticAssertion {
	return domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectProductRelationship, Product: "example", Name: "ingress-nginx"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindRequirementChanged, After: ptr(">=1.12.0")},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
			{Op: domain.OpProductVersion, Name: "ingress-nginx", State: domain.StateInRange, Range: ">=1.12.0"},
			{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Issuer", Of: []domain.Condition{field("spec.acme.solvers[].http01.ingress", domain.StateSet)}},
		}}},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceWorkloadFailure, ExposedClass: domain.ImpactActionRequired,
			Statement: "HTTP01 challenges fail: ingress-nginx rejects the Exact-path Ingress the solver creates."},
		Statement: "HTTP01 solver paths are now PathType Exact; ingress-nginx >= 1.12 rejects them under strict path validation.",
	}
}

func lookupOf(e *domain.UpgradeEdge) func(domain.EvidenceID) (domain.Evidence, bool) {
	return edgeLookup(e)
}

func buildWith(t *testing.T, e *domain.UpgradeEdge, en *env.Environment, facts []domain.VerifiedFact, min domain.VerificationLevel) *domain.ImpactReport {
	t.Helper()
	r, err := Build(Input{Edge: e, Env: en, Now: testNow, Facts: facts, MinVerification: min})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("report does not validate: %v", err)
	}
	return r
}

func findingsOf(r *domain.ImpactReport, changeID string) []domain.ImpactFinding {
	var out []domain.ImpactFinding
	for _, f := range r.Findings {
		if f.ChangeID == changeID {
			out = append(out, f)
		}
	}
	return out
}

func TestClassifyKnowledgeLadder(t *testing.T) {
	tr, fa, un := ConditionResult{Value: True}, ConditionResult{Value: False}, ConditionResult{Value: Unknown, Reason: domain.UnknownCrossProductContextGap}
	none := ConditionResult{}
	action := http01Assertion()
	cases := []struct {
		level    domain.VerificationLevel
		x, o     ConditionResult
		class    domain.ImpactClass
		conf     domain.Confidence
		reason   domain.UnknownReason
		describe string
	}{
		{domain.VerifiedHuman, tr, none, domain.ImpactActionRequired, domain.ConfidenceHigh, "", "trusted + exposed → the fact's class"},
		{domain.VerifiedDeterministic, tr, none, domain.ImpactActionRequired, domain.ConfidenceHigh, "", "deterministic is trusted"},
		{domain.VerifiedProxy, tr, none, domain.ImpactReviewRequired, domain.ConfidenceMedium, "", "proxy capped at review"},
		{domain.VerifiedConsensus, tr, none, domain.ImpactReviewRequired, domain.ConfidenceMedium, "", "consensus treated like proxy"},
		{domain.VerifiedHuman, fa, tr, domain.ImpactInformational, domain.ConfidenceHigh, "", "shielded overlap"},
		{domain.VerifiedProxy, fa, tr, domain.ImpactInformational, domain.ConfidenceMedium, "", "proxy overlap"},
		{domain.VerifiedHuman, fa, none, domain.ImpactNotAffected, domain.ConfidenceHigh, "", "trusted clear"},
		{domain.VerifiedHuman, fa, un, domain.ImpactNotAffected, domain.ConfidenceHigh, "", "trusted clear, overlap unknown"},
		{domain.VerifiedProxy, fa, none, domain.ImpactUnknown, domain.ConfidenceMedium, domain.UnknownReleaseKnowledgeGap, "a proxy never clears"},
		{domain.VerifiedConsensus, fa, fa, domain.ImpactUnknown, domain.ConfidenceMedium, domain.UnknownReleaseKnowledgeGap, "consensus never clears"},
		{domain.VerifiedHuman, un, none, domain.ImpactUnknown, domain.ConfidenceHigh, domain.UnknownCrossProductContextGap, "leaf reason"},
		{domain.VerifiedProxy, un, none, domain.ImpactUnknown, domain.ConfidenceMedium, domain.UnknownCrossProductContextGap, "leaf reason (proxy)"},
	}
	for _, tc := range cases {
		f := fact(t, action, tc.level)
		class, conf, reason := ClassifyKnowledge(f, tc.x, tc.o)
		if class != tc.class || conf != tc.conf || reason != tc.reason {
			t.Errorf("%s: got %s/%s/%q, want %s/%s/%q", tc.describe, class, conf, reason, tc.class, tc.conf, tc.reason)
		}
	}
}

// rotationEdge: two prose restatements (upgrade guide + release notes) and the
// computed schema-default diff of the same field, plus an umbrella note.
func rotationEdge() (*edgeBuilder, domain.Change, domain.Change, domain.Change, domain.Change) {
	eb := newEdge()
	guide := eb.note("https://example/upgrade-guide.md", "Certificate rotationPolicy now defaults to Always",
		"The default value of spec.privateKey.rotationPolicy is now Always.")
	notes := eb.note("https://example/release-notes.md", "Certificate rotationPolicy now defaults to Always",
		"BREAKING: rotationPolicy defaults to Always.")
	computedC := domain.Change{
		ID: "chg-computed-default", Category: domain.CategoryCRDSchema,
		Title:      "Certificate v1 schema: default changed: `spec.privateKey.rotationPolicy`",
		Detail:     "Schema defaults changed in the cert-manager.io/v1 schema of certificates.cert-manager.io (old → new; defaults apply to objects that leave the field unset):\nspec.privateKey.rotationPolicy: \"Never\" → \"Always\"",
		Subjects:   []string{"spec.privateKey.rotationPolicy"},
		Provenance: computed(upgrade.RuleCRDDefaultChanged),
		Evidence:   []domain.EvidenceID{eb.ev("https://example/crds.yaml", "crd schema")},
	}
	eb.edge.Changes = append(eb.edge.Changes, computedC)
	umbrella := eb.note("https://example/release-notes.md", "Highlights",
		"- rotationPolicy defaults to Always\n- other things")
	umbrella.Detail = "- rotationPolicy defaults to Always\n- something else entirely"
	eb.edge.Changes[len(eb.edge.Changes)-1] = umbrella
	return eb, guide, notes, computedC, umbrella
}

func TestKnowledgeRotationPolicyIsReviewOnEveryRestatement(t *testing.T) {
	eb, guide, notes, computedC, umbrella := rotationEdge()
	f := fact(t, rotationAssertion(), domain.VerifiedHuman, domain.NewChangeAnchor(guide, lookupOf(eb.edge)))

	attached := AttachedChanges(f, eb.edge)
	ids := map[string]bool{}
	for _, c := range attached {
		ids[c.ID] = true
	}
	for _, want := range []domain.Change{guide, notes, computedC} {
		if !ids[want.ID] {
			t.Errorf("fact does not attach to restatement %q", want.Title)
		}
	}
	if ids[umbrella.ID] {
		t.Error("fact attached to an umbrella change")
	}

	en := condEnv(t, "", nil)
	r := buildWith(t, eb.edge, en, []domain.VerifiedFact{f}, "")
	// the computed default diff is joined deterministically (review for the
	// Certificate that leaves the field unset): the knowledge finding is not
	// stronger, so it is not added next to it
	if fs := findingsOf(r, computedC.ID); len(fs) == 0 || fs[0].Rule != RuleCRDDefaultApplies || fs[0].Classification != domain.ImpactReviewRequired {
		t.Errorf("computed default diff: %+v", fs)
	}
	for _, f := range findingsOf(r, computedC.ID) {
		if f.Knowledge != nil {
			t.Errorf("knowledge finding added next to an equally strong deterministic one: %+v", f)
		}
	}
	for _, c := range []domain.Change{guide, notes} {
		fs := findingsOf(r, c.ID)
		if len(fs) != 1 {
			t.Fatalf("%q: %d findings, want exactly the knowledge finding: %+v", c.Title, len(fs), fs)
		}
		k := fs[0]
		if k.Rule != RuleKnowledgeExposed || k.Classification != domain.ImpactReviewRequired || k.Knowledge == nil || k.Knowledge.Fact != f.ID {
			t.Errorf("%q: %s/%s, want impact:knowledge-exposed review-required", c.Title, k.Rule, k.Classification)
		}
		if len(k.Matches) == 0 || len(k.EnvironmentEvidence) == 0 {
			t.Errorf("%q: exposed finding without the environment chain", c.Title)
		}
		// certificate "a" leaves rotationPolicy unset; "b" pins it
		joined := ""
		for _, m := range k.Matches {
			joined += m.Subject + "\n"
		}
		if !strings.Contains(joined, "ns/a") || strings.Contains(joined, "ns/b leaves") {
			t.Errorf("%q: matches %q, want Certificate a (unset) only", c.Title, joined)
		}
	}
	// the umbrella keeps its own unknown record, with a reason
	if fs := findingsOf(r, umbrella.ID); len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown || fs[0].UnknownReason != domain.UnknownReleaseKnowledgeGap {
		t.Errorf("umbrella findings = %+v", fs)
	}
	// the fact's own upstream evidence is copied into the pool
	found := false
	for _, e := range r.Evidence {
		found = found || e.ID == f.Evidence[0].ID
	}
	if !found {
		t.Error("fact evidence is not in the report's evidence pool")
	}
}

func TestKnowledgeOverlapAndClear(t *testing.T) {
	eb, guide, _, _, _ := rotationEdge()
	f := fact(t, rotationAssertion(), domain.VerifiedHuman, domain.NewChangeAnchor(guide, lookupOf(eb.edge)))
	dir := t.TempDir()
	// every Certificate pins the policy → overlap: informational
	pinned := loadEnv(t, env.Inputs{Manifests: []string{writeFile(t, dir, "m/c.yaml", "apiVersion: cert-manager.io/v1\nkind: Certificate\nmetadata: {name: p}\nspec:\n  privateKey: {rotationPolicy: Always}\n")}})
	r := buildWith(t, eb.edge, pinned, []domain.VerifiedFact{f}, "")
	if fs := findingsOf(r, guide.ID); len(fs) != 1 || fs[0].Rule != RuleKnowledgeOverlap || fs[0].Classification != domain.ImpactInformational {
		t.Errorf("pinned: %+v", fs)
	}
	// no Certificate at all, manifests healthy → not affected with checks
	none := loadEnv(t, env.Inputs{Manifests: []string{writeFile(t, dir, "n/cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: x}\ndata: {a: b}\n")}})
	r = buildWith(t, eb.edge, none, []domain.VerifiedFact{f}, "")
	fs := findingsOf(r, guide.ID)
	if len(fs) != 1 || fs[0].Rule != RuleKnowledgeClear || fs[0].Classification != domain.ImpactNotAffected || len(fs[0].Checks) == 0 {
		t.Errorf("clear: %+v", fs)
	}
	// no manifests at all → unknown with the leaf's reason
	bare := loadEnv(t, env.Inputs{KubernetesVersion: "1.30"})
	r = buildWith(t, eb.edge, bare, []domain.VerifiedFact{f}, "")
	fs = findingsOf(r, guide.ID)
	if len(fs) != 1 || fs[0].Rule != RuleKnowledgeUndecided || fs[0].UnknownReason != domain.UnknownEnvironmentVisibilityGap {
		t.Errorf("no manifests: %+v", fs)
	}
}

func http01Edge() (*edgeBuilder, domain.Change) {
	eb := newEdge()
	c := eb.note("https://example/upgrade-guide.md", "HTTP01 solver paths are now PathType Exact",
		"HTTP01 challenge paths now use PathType Exact; ingress-nginx >= 1.12 with strict path validation rejects them.")
	return eb, c
}

func TestKnowledgeActionPathEndToEnd(t *testing.T) {
	eb, c := http01Edge()
	anchor := domain.NewChangeAnchor(c, lookupOf(eb.edge))
	human := fact(t, http01Assertion(), domain.VerifiedHuman, anchor)

	en := condEnv(t, "- product: ingress-nginx\n  version: v1.12.1\n", nil)
	r := buildWith(t, eb.edge, en, []domain.VerifiedFact{human}, "")
	fs := findingsOf(r, c.ID)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactActionRequired || fs[0].Provenance.Confidence != domain.ConfidenceHigh {
		t.Fatalf("trusted cross-product fact: %+v", fs)
	}
	kinds := map[domain.ImpactMatchKind]bool{}
	for _, m := range fs[0].Matches {
		kinds[m.Kind] = true
	}
	if !kinds[domain.MatchProduct] || !kinds[domain.MatchAPIVersion] {
		t.Errorf("ACTION must cite both leaves (inventory entry and the Issuer): %+v", fs[0].Matches)
	}

	// the same fact verified only by a proxy: REVIEW at most, medium
	proxy := fact(t, http01Assertion(), domain.VerifiedProxy, anchor)
	r = buildWith(t, eb.edge, en, []domain.VerifiedFact{proxy}, domain.VerifiedProxy)
	if fs := findingsOf(r, c.ID); len(fs) != 1 || fs[0].Classification != domain.ImpactReviewRequired || fs[0].Provenance.Confidence != domain.ConfidenceMedium {
		t.Errorf("proxy fact: %+v", fs)
	}
	// a proxy fact is not used at the default (human) minimum
	r = buildWith(t, eb.edge, en, []domain.VerifiedFact{proxy}, "")
	if fs := findingsOf(r, c.ID); len(fs) != 1 || fs[0].Knowledge != nil {
		t.Errorf("proxy fact used below its level: %+v", fs)
	}

	// guards: each missing leaf turns the verdict into UNKNOWN with its reason
	guards := []struct {
		name      string
		inventory string
		reason    domain.UnknownReason
	}{
		{"no inventory", "", domain.UnknownCrossProductContextGap},
		{"product not listed, inventory not declared complete", "- product: argo-cd\n  version: 2.14.1\n", domain.UnknownCrossProductContextGap},
	}
	for _, g := range guards {
		en := condEnv(t, g.inventory, nil)
		r := buildWith(t, eb.edge, en, []domain.VerifiedFact{human}, "")
		fs := findingsOf(r, c.ID)
		if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown || fs[0].UnknownReason != g.reason {
			t.Errorf("%s: %+v", g.name, fs)
		}
	}
	// declared complete without the product → checked and clear
	en = condEnv(t, "complete: true\nproducts:\n  - product: argo-cd\n    version: 2.14.1\n", nil)
	r = buildWith(t, eb.edge, en, []domain.VerifiedFact{human}, "")
	if fs := findingsOf(r, c.ID); len(fs) != 1 || fs[0].Classification != domain.ImpactNotAffected {
		t.Errorf("complete inventory without ingress-nginx: %+v", fs)
	}
}

func TestKnowledgeNeverDowngradesADeterministicFinding(t *testing.T) {
	eb := newEdge()
	removed := eb.change(upgrade.RuleValuesRemoved, "Helm value tls.secretsBackend removed", "tls.secretsBackend")
	// a trusted fact that (wrongly) says no environment is exposed
	a := domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectHelmValue, Product: "example", Path: "tls.secretsBackend"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindRemoved},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "nonexistent.key", State: domain.StateSet}},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceSettingIgnored, ExposedClass: domain.ImpactActionRequired, Statement: "The key stops taking effect."},
		Statement:     "tls.secretsBackend was removed.",
	}
	f := fact(t, a, domain.VerifiedHuman)
	if got := AttachedChanges(f, eb.edge); len(got) != 1 || got[0].ID != removed.ID {
		t.Fatalf("subject restatement: attached %+v", got)
	}
	en := condEnv(t, "", nil)
	r := buildWith(t, eb.edge, en, []domain.VerifiedFact{f}, "")
	fs := findingsOf(r, removed.ID)
	if len(fs) != 1 || fs[0].Rule != RuleValuesRemoved || fs[0].Classification != domain.ImpactActionRequired {
		t.Errorf("deterministic ACTION must stand alone (a knowledge not-affected never hides it): %+v", fs)
	}
}

func TestKnowledgeProxyNeverClearsAndSupersedesTheUnknown(t *testing.T) {
	eb, c := http01Edge()
	proxy := fact(t, http01Assertion(), domain.VerifiedProxy, domain.NewChangeAnchor(c, lookupOf(eb.edge)))
	en := condEnv(t, "- product: ingress-nginx\n  version: v1.11.0\n", nil) // exposure false
	r := buildWith(t, eb.edge, en, []domain.VerifiedFact{proxy}, domain.VerifiedProxy)
	fs := findingsOf(r, c.ID)
	if len(fs) != 1 {
		t.Fatalf("want exactly one finding (the knowledge record supersedes not-joined), got %+v", fs)
	}
	if fs[0].Classification != domain.ImpactUnknown || fs[0].UnknownReason != domain.UnknownReleaseKnowledgeGap || fs[0].Knowledge == nil {
		t.Errorf("proxy + exposure false: %+v", fs[0])
	}
}

func TestAttachmentGuards(t *testing.T) {
	eb := newEdge()
	// a computed values diff reaching outside the fact's subtree is an umbrella for it
	wide := eb.change(upgrade.RuleValuesSectionRemoved, "Helm values section tls.* removed", "tls.secretsBackend", "tls.other")
	narrow := eb.change(upgrade.RuleValuesSectionRemoved, "Helm values section tls.secretsBackend.* removed", "tls.secretsBackend.mode")
	deflt := eb.change(upgrade.RuleValuesDefaultChanged, "Default of tls.secretsBackend changed", "tls.secretsBackend")
	a := domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectHelmValue, Product: "example", Path: "tls.secretsBackend"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindRemoved},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "tls.secretsBackend", State: domain.StateSet}},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceSettingIgnored, ExposedClass: domain.ImpactActionRequired, Statement: "The key stops taking effect."},
		Statement:     "tls.secretsBackend was removed.",
	}
	f := fact(t, a, domain.VerifiedHuman)
	got := map[string]bool{}
	for _, c := range AttachedChanges(f, eb.edge) {
		got[c.ID] = true
	}
	if got[wide.ID] || !got[narrow.ID] || got[deflt.ID] {
		t.Errorf("attached: wide=%v narrow=%v default-changed=%v, want false/true/false", got[wide.ID], got[narrow.ID], got[deflt.ID])
	}
	// another product, or a release outside the edge, attaches nothing
	other := f
	other.Product = "other"
	if n := len(AttachedChanges(other, eb.edge)); n != 0 {
		t.Errorf("a fact of another product attached %d changes", n)
	}
	late := f
	late.Release = "2.0.0"
	if n := len(AttachedChanges(late, eb.edge)); n != 0 {
		t.Errorf("a fact introduced after the edge attached %d changes", n)
	}
	retracted := f
	retracted.Status = domain.FactRetracted
	if n := len(AttachedChanges(retracted, eb.edge)); n != 0 {
		t.Errorf("a retracted fact attached %d changes", n)
	}
}

func TestBuildWithoutApplicableFactsIsByteIdentical(t *testing.T) {
	eb, _, _, _, _ := rotationEdge()
	en := condEnv(t, "", nil)
	base := buildWith(t, eb.edge, en, nil, "")
	// a fact of another product changes nothing
	f := fact(t, rotationAssertion(), domain.VerifiedHuman)
	f.Product = "other"
	f.Assertion.Subject.Product = "other"
	f.ID = domain.VerifiedFactID(f.Product, f.Release, f.Assertion)
	with := buildWith(t, eb.edge, en, []domain.VerifiedFact{f}, "")
	a, _ := json.Marshal(base)
	b, _ := json.Marshal(with)
	if string(a) != string(b) {
		t.Error("an inapplicable fact changed the report")
	}
}

// consensusFact: every aspect agreed by separate model calls; action marks the
// PO-2 consensus-action path.
func consensusFact(t *testing.T, a domain.SemanticAssertion, action bool, anchors ...domain.ChangeAnchor) domain.VerifiedFact {
	t.Helper()
	f := fact(t, a, domain.VerifiedConsensus, anchors...)
	if action {
		f.ConsensusAction = true
		if err := f.Validate(); err != nil {
			t.Fatalf("consensus-action fact invalid: %v", err)
		}
	}
	return f
}

func TestKnowledgeConsensusActionPO2(t *testing.T) {
	eb, c := http01Edge()
	anchor := domain.NewChangeAnchor(c, lookupOf(eb.edge))
	en := condEnv(t, "- product: ingress-nginx\n  version: v1.12.1\n", nil)

	act := consensusFact(t, http01Assertion(), true, anchor)
	r := buildWith(t, eb.edge, en, []domain.VerifiedFact{act}, domain.VerifiedConsensus)
	f := onlyFinding(t, r, c.ID)
	if f.Classification != domain.ImpactActionRequired || f.Knowledge.Verification != domain.VerifiedConsensus ||
		!f.Knowledge.ConsensusAction || f.Knowledge.Consensus != domain.ConsensusCrossModel || f.Knowledge.ActionLabel() != "model consensus" {
		t.Errorf("consensus-action: %s %+v", f.Classification, f.Knowledge)
	}
	if !strings.Contains(f.Title, "model consensus") {
		t.Errorf("a consensus ACTION must be labelled as such: %q", f.Title)
	}
	// consensus facts are not used at the human (gate) level
	if f := onlyFinding(t, buildWith(t, eb.edge, en, []domain.VerifiedFact{act}, ""), c.ID); f.Knowledge != nil {
		t.Errorf("consensus fact used at the human level: %+v", f)
	}
	// consensus without the action marker: review at most
	plain := consensusFact(t, http01Assertion(), false, anchor)
	if f := onlyFinding(t, buildWith(t, eb.edge, en, []domain.VerifiedFact{plain}, domain.VerifiedConsensus), c.ID); f.Classification != domain.ImpactReviewRequired || f.Provenance.Confidence == domain.ConfidenceHigh {
		t.Errorf("plain consensus: %s/%s", f.Classification, f.Provenance.Confidence)
	}
	// consensus never clears, even with the action marker
	low := condEnv(t, "- product: ingress-nginx\n  version: v1.11.0\n", nil)
	if f := onlyFinding(t, buildWith(t, eb.edge, low, []domain.VerifiedFact{act}, domain.VerifiedConsensus), c.ID); f.Classification != domain.ImpactUnknown || f.UnknownReason != domain.UnknownReleaseKnowledgeGap {
		t.Errorf("consensus with exposure false: %s/%s", f.Classification, f.UnknownReason)
	}
	// a leaf the environment cannot decide stays unknown with its reason
	if f := onlyFinding(t, buildWith(t, eb.edge, condEnv(t, "", nil), []domain.VerifiedFact{act}, domain.VerifiedConsensus), c.ID); f.Classification != domain.ImpactUnknown || f.UnknownReason != domain.UnknownCrossProductContextGap {
		t.Errorf("consensus-action without inventory: %s/%s", f.Classification, f.UnknownReason)
	}
}
