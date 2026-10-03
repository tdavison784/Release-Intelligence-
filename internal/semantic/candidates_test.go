package semantic

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

func TestCandidatesClusterRestatements(t *testing.T) {
	edge := rotationEdge()
	if err := edge.Validate(); err != nil {
		t.Fatalf("fixture edge invalid: %v", err)
	}
	rep := BuildCandidates(edge, t0)
	for _, c := range rep.Candidates {
		if err := c.Validate(); err != nil {
			t.Errorf("candidate %s invalid: %v", c.ID, err)
		}
	}

	rot := candidateFor(t, rep.Candidates, "chg-guide")
	if got := memberIDs(rot); !reflect.DeepEqual(got, []string{"chg-guide", "chg-theme", "chg-item"}) {
		t.Errorf("rotation cluster members = %v", got)
	}
	if rot.Grouping != GroupTitleJaccard {
		t.Errorf("rotation grouping = %q", rot.Grouping)
	}
	if len(rot.Anchors()) != 3 {
		t.Errorf("every prose member carries an anchor, got %d", len(rot.Anchors()))
	}
	if rot.Release != "1.1.0" || rot.Producer != CandidateProducer {
		t.Errorf("release/producer = %q/%q", rot.Release, rot.Producer)
	}

	helm := candidateFor(t, rep.Candidates, "chg-helm")
	if got := memberIDs(helm); !reflect.DeepEqual(got, []string{"chg-helm", "chg-values-added"}) {
		t.Errorf("subject-named cluster = %v", got)
	}
	if helm.Grouping != GroupSubjectNamed {
		t.Errorf("helm grouping = %q", helm.Grouping)
	}
	if !strings.Contains(helm.Text, "computed artifact diff (values:added)") {
		t.Errorf("the candidate must carry the computed member's statement, text = %q", helm.Text)
	}

	crd := candidateFor(t, rep.Candidates, "chg-crd-added")
	if crd.Grouping != GroupSingle || !crd.Members[0].Computed || crd.Release != "" {
		t.Errorf("computed no-join-rule change = %+v", crd)
	}

	skipped := map[string]string{}
	for _, s := range rep.Skipped {
		skipped[s.ChangeID] = s.Reason
	}
	want := map[string]string{"chg-bump": SkipRoutine, "chg-cve": SkipSecurityFix, "chg-umbrella": SkipUmbrella, "chg-crd-multi": SkipMultiSubject}
	if !reflect.DeepEqual(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
	if len(rep.Candidates) != 4 {
		t.Errorf("want 4 candidates (rotation, helm, gate, crd), got %d", len(rep.Candidates))
	}
}

func TestCandidatesDeterministicAndOrderIndependentIDs(t *testing.T) {
	a := Candidates(rotationEdge(), t0)
	b := Candidates(rotationEdge(), t0)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("candidates differ between runs")
	}
	// reversing the edge's change order changes nothing about identity
	e := rotationEdge()
	for i, j := 0, len(e.Changes)-1; i < j; i, j = i+1, j-1 {
		e.Changes[i], e.Changes[j] = e.Changes[j], e.Changes[i]
	}
	ids := func(cs []domain.SemanticCandidate) map[string]bool {
		m := map[string]bool{}
		for _, c := range cs {
			m[c.ID] = true
		}
		return m
	}
	if !reflect.DeepEqual(ids(a), ids(Candidates(e, t0))) {
		t.Error("candidate ids depend on change order")
	}
}

func TestCandidatesNeverCrossSubjectsOrReleases(t *testing.T) {
	b := newEdge()
	b.note("chg-a", "The default value of `spec.alpha` changed from `1` to `2`", notesURI, "L1")
	b.note("chg-b", "The default value of `spec.beta` changed from `1` to `2`", notesURI, "L2")
	b.note("chg-c", "The default value of `spec.alpha` changed from `1` to `2`", notesURI, "L3", func(c *domain.Change) { c.Release = "1.0.5" })
	cands := Candidates(b.edge(), t0)
	if len(cands) != 3 {
		t.Fatalf("distinct subjects / releases must stay separate, got %d candidates", len(cands))
	}
	for _, c := range cands {
		if len(c.Members) != 1 {
			t.Errorf("unexpected cluster %v", memberIDs(c))
		}
	}
}

func TestSameStatementAcrossChanges(t *testing.T) {
	b := newEdge()
	// two changes quoting the same evidence statement (e.g. two headings
	// classifying one bullet) are one cluster even with unrelated titles
	evID := b.ev(domain.EvidenceDocument, notesURI, "L9", "Removed the deprecated flag `--old-mode`.")
	for _, id := range []string{"chg-1", "chg-2"} {
		c := b.note(id, "x "+id, notesURI, "L"+id)
		c.Evidence = []domain.EvidenceID{evID}
	}
	cands := Candidates(b.edge(), t0)
	if len(cands) != 1 || cands[0].Grouping != GroupSameStatement {
		t.Fatalf("want one same-statement cluster, got %+v", cands)
	}
}

func TestSubjectNamedAmbiguityJoinsNothing(t *testing.T) {
	b := newEdge()
	// a note naming one key, two computed diffs of that key in different
	// subcharts: two distinct subjects → no join
	b.note("chg-note", "Added value `seLinuxOptions` to the cni chart", notesURI, "L1")
	b.computed("chg-cni", upgrade.RuleValuesAdded, "New Helm value `seLinuxOptions` (chart cni)", "seLinuxOptions")
	b.computed("chg-zt", upgrade.RuleValuesAdded, "New Helm value `seLinuxOptions` (chart ztunnel)", "seLinuxOptions")
	cands := Candidates(b.edge(), t0)
	if len(cands) != 1 || len(cands[0].Members) != 1 {
		t.Fatalf("ambiguous subject-named join must join nothing: %+v", cands)
	}

	// a values section named key by key joins
	b = newEdge()
	b.note("chg-note", "The integration Helm options `bgp.enabled`, `bgp.announce.podCIDR` have been removed", notesURI, "L1")
	b.computed("chg-sec", upgrade.RuleValuesSectionRemoved, "Helm values section `bgp.*` removed (2 keys)", "bgp.announce.podCIDR", "bgp.enabled")
	cands = Candidates(b.edge(), t0)
	if len(cands) != 1 || len(cands[0].Members) != 2 || !cands[0].Members[1].Computed {
		t.Fatalf("section named by its keys must join: %+v", cands)
	}
}

func TestHintsAndSignature(t *testing.T) {
	hs := Hints(0, "Set `rotationPolicy: Always` on tls.secretsBackend; use --feature-gates and LEADER_ELECT with karpenter.sh/v1beta1, requires >= 1.12 (see values.yaml)")
	for _, want := range []string{"code:rotationPolicy: Always", "path:tls.secretsBackend", "flag:--feature-gates", "env:LEADER_ELECT", "apiVersion:karpenter.sh/v1beta1", "constraint:>=1.12"} {
		if !contains(hs, want) {
			t.Errorf("hints %v lack %q", hs, want)
		}
	}
	if contains(hs, "path:values.yaml") {
		t.Errorf("a file name is not a key path: %v", hs)
	}
	sig := subjectSignature("The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`: changed `Certificate.Spec.Pr")
	if !reflect.DeepEqual(sig, map[string]bool{"certificate.spec.privatekey.rotationpolicy": true}) {
		t.Errorf("signature = %v (values and a truncated span are not identifiers)", sig)
	}
}

func memberIDs(c domain.SemanticCandidate) []string {
	var out []string
	for _, m := range c.Members {
		out = append(out, m.ChangeID)
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestLeadSentenceRestatements(t *testing.T) {
	b := newEdge()
	// a label prefix, and a title that runs on into the body naming the
	// field again in another spelling: both restate the guide's item
	b.note("chg-a", "We have changed the default value of `Widget.Spec.HistoryLimit` from `nil` to `1`. > Read the notes.", guideURI, "L1")
	b.note("chg-b", "The default value of `Widget.Spec.HistoryLimit` is now `1`: The default for the `historyLimit` field is now…", notesURI, "L2")
	b.note("chg-c", "DEPRECATION: The default value of `Widget.Spec.HistoryLimit` changed to `1`", notesURI, "L3")
	cands := Candidates(b.edge(), t0)
	if len(cands) != 1 || len(cands[0].Members) != 3 {
		t.Fatalf("want one 3-member cluster, got %d candidates", len(cands))
	}
	if got := leadSentence("DEPRECATION: The `X` feature gate is deprecated"); got != "The `X` feature gate is deprecated" {
		t.Errorf("lead = %q", got)
	}
}

// L4: captured statement lines and subject-naming routine notes become
// candidates; dependency bumps do not; admitted routine notes never join an
// existing (non-routine) cluster, so existing candidate ids are stable.
func TestL4Candidates(t *testing.T) {
	b := newEdge()
	b.note("chg-prose", "The default of `widget_requests_total` changed", notesURI, "L1")
	b.note("chg-metric", "Renamed metric `widget_requests_total` to `widget_http_requests_total`", notesURI, "L2",
		func(c *domain.Change) { c.Routine = true; c.RoutineKind = upgrade.RoutineMetrics })
	b.note("chg-ci", "Update CI workflow", notesURI, "L3", func(c *domain.Change) { c.Routine = true; c.RoutineKind = upgrade.RoutineHousekeeping })
	b.note("chg-dep", "Bump `golang.org/x/net` to v0.30.0", notesURI, "L4", func(c *domain.Change) { c.Routine = true; c.RoutineKind = upgrade.RoutineDependency })
	b.computed("chg-line", upgrade.RuleLinesAdded, "Line added to install.yaml: Deprecated: Use spec.proxyRef instead.", "Deprecated: Use spec.proxyRef instead.")
	rep := BuildCandidates(b.edge(), t0)
	got := map[string]int{}
	for _, c := range rep.Candidates {
		for _, m := range c.Members {
			got[m.ChangeID] = len(c.Members)
		}
	}
	for id, n := range map[string]int{"chg-prose": 1, "chg-metric": 1, "chg-line": 1} {
		if got[id] != n {
			t.Errorf("%s: cluster size %d, want %d (%v)", id, got[id], n, got)
		}
	}
	for _, id := range []string{"chg-ci", "chg-dep"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s must stay skipped", id)
		}
	}
}
