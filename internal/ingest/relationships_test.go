package ingest

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestCheckRelationships(t *testing.T) {
	w, def, p := newWorld(), testDef(), &fakeParser{}
	def.Artifacts[2].Version.Select = "all" // several chart instances per release count once
	ing := newTestIngester(w, p)
	vl := mustVersions(t, ing, def)
	var releases []domain.Version
	for _, s := range []string{"1.1.0", "1.1.1", "1.2.0", "1.2.1"} {
		releases = append(releases, version(t, vl, s))
	}
	rep, err := ing.CheckRelationships(context.Background(), def, releases, vl)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rep.Releases, []string{"1.1.0", "1.1.1", "1.2.0", "1.2.1"}) || rep.Product != "acme" {
		t.Fatalf("report header: %+v %v", rep.Product, rep.Releases)
	}

	type verdict struct {
		kind                string
		passed, failed, unv int
		verdict             string
	}
	want := map[string]verdict{
		// 1.1.1 has no section in the website notes, but its fallback
		// alternative (gh-notes) answered: covered, not failed
		"site-notes": {SubjectSource, 3, 0, 0, VerdictValidated},
		// exhaustive mode validates the fallback even when it is not needed
		"gh-notes": {SubjectSource, 4, 0, 0, VerdictValidated},
		// patch releases are not applicable
		"upgrade-guide":  {SubjectSource, 2, 0, 0, VerdictInsufficient},
		"support-matrix": {SubjectSource, 4, 0, 0, VerdictValidated},
		// no git-log adapter in this build
		"commit-log":       {SubjectSource, 0, 0, 4, VerdictInsufficient},
		"legacy-changelog": {SubjectSource, 0, 0, 0, VerdictInsufficient},
		"controller-image": {SubjectArtifact, 4, 0, 0, VerdictValidated},
		"install-manifest": {SubjectArtifact, 4, 0, 0, VerdictValidated},
		// v1.2.1 ships in no chart
		"chart": {SubjectArtifact, 3, 1, 0, VerdictFailing},
		"crds":  {SubjectArtifact, 4, 0, 0, VerdictValidated},
		"cli":   {SubjectArtifact, 0, 0, 4, VerdictInsufficient},
		// skipped for 1.2.1 (no chart version) → not applicable
		"chart/helm-values":           {SubjectContent, 3, 0, 0, VerdictValidated},
		"chart/chart-metadata":        {SubjectContent, 3, 0, 0, VerdictValidated},
		"crds/crds":                   {SubjectContent, 4, 0, 0, VerdictValidated},
		"install-manifest/image-refs": {SubjectContent, 4, 0, 0, VerdictValidated},
	}
	got := map[string]RelationshipSummary{}
	for _, s := range rep.Summary {
		got[s.Subject] = s
	}
	for subject, w := range want {
		s, ok := got[subject]
		if !ok {
			t.Errorf("no summary for %s", subject)
			continue
		}
		if s.SubjectKind != w.kind || s.Passed != w.passed || s.Failed != w.failed || s.Unverifiable != w.unv || s.Verdict != w.verdict {
			t.Errorf("%s: got %+v, want %+v", subject, s, w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("summaries: %+v", rep.Summary)
	}

	covered := false
	for _, c := range rep.Checks {
		if c.Subject == "site-notes" && c.Release == "1.1.1" {
			covered = c.Outcome == OutcomeCovered && strings.Contains(c.Detail, "covered by gh-notes")
		}
	}
	if !covered {
		t.Errorf("site-notes@1.1.1 should be covered by gh-notes")
	}

	// Checks carry release, channel, detail and resolvable evidence.
	evs := map[domain.EvidenceID]bool{}
	for _, e := range rep.Evidence {
		evs[e.ID] = true
	}
	var sawGhPass, sawChartFail bool
	for _, c := range rep.Checks {
		if c.Release == "" || c.Outcome == "" || c.SubjectKind == "" {
			t.Errorf("incomplete check: %+v", c)
		}
		for _, id := range c.Evidence {
			if !evs[id] {
				t.Errorf("check %s/%s references unknown evidence %s", c.Subject, c.Release, id)
			}
		}
		if c.Subject == "gh-notes" && c.Release == "1.2.0" && c.Outcome == OutcomePass {
			sawGhPass = len(c.Evidence) > 0 && c.Channel == catalog.LocatorGitHubReleases
		}
		if c.Subject == "chart" && c.Release == "1.2.1" {
			sawChartFail = c.Outcome == OutcomeFail && strings.Contains(c.Detail, "no chart version ships appVersion v1.2.1")
		}
	}
	if !sawGhPass || !sawChartFail {
		t.Fatalf("checks: gh-notes pass with evidence=%v chart fail=%v", sawGhPass, sawChartFail)
	}

	// The plain ingestion still stops at the first ok member.
	rel, err := ing.IngestRelease(context.Background(), def, version(t, vl, "1.2.0"), vl)
	if err != nil {
		t.Fatal(err)
	}
	if s := statusOf(t, rel, "gh-notes"); s.State != domain.SourceSkipped {
		t.Fatalf("IngestRelease must not be exhaustive: %+v", s)
	}
}

func TestSummarizeVerdicts(t *testing.T) {
	c := func(subject, release, outcome string) RelationshipCheck {
		return RelationshipCheck{Subject: subject, SubjectKind: SubjectArtifact, Release: release, Outcome: outcome}
	}
	checks := []RelationshipCheck{
		c("a", "1", OutcomePass), c("a", "2", OutcomePass), c("a", "3", OutcomePass),
		c("b", "1", OutcomePass), c("b", "2", OutcomePass), c("b", "2", OutcomePass), // same release twice
		c("c", "1", OutcomePass), c("c", "1", OutcomeFail), c("c", "2", OutcomePass), c("c", "3", OutcomePass), c("c", "4", OutcomePass),
		c("d", "1", OutcomeUnverifiable), c("d", "1", OutcomePass), c("d", "2", OutcomeNotApplicable),
	}
	got := summarize(checks)
	want := []RelationshipSummary{
		{Subject: "a", SubjectKind: SubjectArtifact, Passed: 3, Verdict: VerdictValidated},
		{Subject: "b", SubjectKind: SubjectArtifact, Passed: 2, Verdict: VerdictInsufficient},
		{Subject: "c", SubjectKind: SubjectArtifact, Passed: 3, Failed: 1, Verdict: VerdictFailing},
		{Subject: "d", SubjectKind: SubjectArtifact, Passed: 1, Verdict: VerdictInsufficient},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestGroupSources(t *testing.T) {
	src := func(id string, prio int, roles ...domain.SourceRole) catalog.Source {
		return catalog.Source{ID: id, Priority: prio, Roles: roles}
	}
	srcs := []catalog.Source{
		src("change-notes", 1, domain.RoleReleaseNotes),
		src("compat", 0, domain.RoleCompatibility),
		src("yaml-notes", 0, domain.RoleReleaseNotes),
		src("patch-master", 0, domain.RoleReleaseNotes),
		src("upgrade", 0, domain.RoleUpgradeGuide),
		src("patch-branch", 1, domain.RoleReleaseNotes),
	}
	// Explicit, role-independent groups (Istio: structured YAML vs
	// change-notes; patch notes on master vs the release branch).
	groups := map[string]string{"change-notes": "minor-notes", "yaml-notes": "minor-notes", "upgrade": "minor-notes",
		"patch-master": "patch-notes", "patch-branch": "patch-notes"}
	got := groupSources(srcs, func(s catalog.Source) string { return groups[s.ID] })
	var desc []string
	for _, g := range got {
		var ids []string
		for _, m := range g.members {
			ids = append(ids, m.ID)
		}
		desc = append(desc, g.name+":"+strings.Join(ids, ","))
	}
	wantDesc := []string{"minor-notes:yaml-notes,upgrade,change-notes", ":compat", "patch-notes:patch-master,patch-branch"}
	if !reflect.DeepEqual(desc, wantDesc) {
		t.Fatalf("groups = %v, want %v", desc, wantDesc)
	}
}

func TestFallbackGroupOfDefinition(t *testing.T) {
	def := testDef()
	byID := map[string]string{}
	for _, s := range def.Sources {
		byID[s.ID] = fallbackGroup(s)
	}
	if byID["site-notes"] == "" || byID["site-notes"] != byID["gh-notes"] || byID["upgrade-guide"] != "" {
		t.Fatalf("fallback groups: %v", byID)
	}
}

func TestFindReference(t *testing.T) {
	cases := []struct {
		content, pattern string
		line             int
	}{
		{"a\n  image: \"quay.io/x/ctl:v1.2.0\"\n", "quay.io/x/ctl:v1.2.0", 2},
		{"image: quay.io/x/ctl:v1.2.0-rc.7\nimage: quay.io/x/ctl:v1.2.0\n", "quay.io/x/ctl:v1.2.0", 2},
		{"image: quay.io/x/ctl:v1.2.0@sha256:abc\n", "quay.io/x/ctl:v1.2.0", 1},
		{"- --solver-image=quay.io/x/solver:v1.2.0\n", "quay.io/x/solver:v1.2.0", 1},
		{"image: mirror.example/quay.io/x/ctl:v1.2.0\n", "quay.io/x/ctl:v1.2.0", 0},
		{"image: quay.io/x/ctl:v1.2.01\n", "quay.io/x/ctl:v1.2.0", 0},
		{"Use quay.io/x/ctl:v1.2.0.\n", "quay.io/x/ctl:v1.2.0", 1},
		{"image: quay.io/x/ctl:v1.2.0.1\n", "quay.io/x/ctl:v1.2.0", 0},
		{`tag: "v1.2.0"`, `"v1.2.0"`, 1},
	}
	for _, tc := range cases {
		n, _, ok := findReference([]byte(tc.content), tc.pattern)
		if (tc.line == 0) == ok || n != tc.line {
			t.Errorf("findReference(%q, %q) = %d,%v; want line %d", tc.content, tc.pattern, n, ok, tc.line)
		}
	}
}

func TestAdvisories(t *testing.T) {
	w, def := newWorld(), testDef()
	ing := newTestIngester(w, &fakeParser{})
	advs, evs, sts, err := ing.Advisories(context.Background(), def)
	if err != nil {
		t.Fatal(err)
	}
	if got := stateList(sts); !reflect.DeepEqual(got, []string{"advisories=ok", "bulletins=skipped"}) {
		t.Fatalf("statuses: %v", got)
	}
	if !strings.Contains(sts[1].Detail, "no advisory adapter") {
		t.Fatalf("bulletins detail: %q", sts[1].Detail)
	}
	if len(advs) != 2 {
		t.Fatalf("advisories: %+v", advs)
	}
	have := map[domain.EvidenceID]bool{}
	for _, e := range evs {
		have[e.ID] = true
	}
	for _, a := range advs {
		if a.SourceID != "advisories" {
			t.Errorf("%s: source id %q", a.ID, a.SourceID)
		}
		if len(a.Evidence) == 0 {
			t.Errorf("%s without evidence", a.ID)
		}
		for _, id := range a.Evidence {
			if !have[id] {
				t.Errorf("%s references unknown evidence %s", a.ID, id)
			}
		}
	}

	w.down[catalog.LocatorGitHubAdvisories] = true
	advs, _, sts, err = ing.Advisories(context.Background(), def)
	if err != nil || len(advs) != 0 || sts[0].State != domain.SourceUnavailable {
		t.Fatalf("unavailable advisories: %v %+v %+v", err, advs, sts)
	}
}
