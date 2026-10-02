package drift

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
)

var baselineReleases = []string{"1.0.0", "1.0.1", "1.1.0", "1.1.1"}

// noDriftWorld is the intact upstream: checking the newest releases against a
// baseline of older ones must produce no events at all.
func TestNoDriftOnIntactUpstream(t *testing.T) {
	w := newWorld()
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	if len(rep.Events) != 0 {
		t.Fatalf("intact upstream produced events: %+v", rep.Events)
	}
	if rep.Summary.Drift != 0 || rep.Summary.Unverifiable != 0 || rep.Summary.Notes != 0 {
		t.Fatalf("summary: %+v", rep.Summary)
	}
	if rep.Baseline.Source != BaselineSavedCheck || rep.Baseline.Cutoff != "1.1.1" {
		t.Fatalf("baseline: %+v", rep.Baseline)
	}
	if strings.Join(rep.Checked, ",") != "1.2.0,1.2.1" {
		t.Fatalf("checked: %v", rep.Checked)
	}
}

// The core honesty rule: an unreachable channel is unverifiable, never drift.
func TestUnreachableIsNotDrift(t *testing.T) {
	w := newWorld()
	// quay.io stops answering for the new releases.
	for _, tag := range []string{"v1.2.0", "v1.2.1"} {
		w.errs[quayController+"@"+tag] = unavailable(quayController + "@" + tag)
	}
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	if rep.Summary.Drift != 0 {
		t.Fatalf("unreachable host must not count as drift: %+v", rep.Events)
	}
	evs := eventsOf(rep, KindUnverifiable)
	if len(evs) != 1 || evs[0].Subject != "controller-image" {
		t.Fatalf("unverifiable events: %+v", evs)
	}
	ev := evs[0]
	if ev.Status != StatusUnverifiable || ev.Severity != SeverityLow {
		t.Fatalf("event: %+v", ev)
	}
	if !strings.Contains(ev.Baseline, "passed on") {
		t.Fatalf("baseline text must record that it held before: %q", ev.Baseline)
	}
	// A report with only unverifiable events has no proposal.
	if RenderProposal(rep) != "" {
		t.Fatal("unverifiable events must not produce proposals")
	}
}

// A declared artifact that vanishes from reachable channels is drift, cites
// evidence and proposes ending the availability at the last observation.
func TestArtifactMissingIsDrift(t *testing.T) {
	w := newWorld()
	delete(w.images, quayController+"@v1.2.0")
	delete(w.images, quayController+"@v1.2.1")
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	evs := eventsOf(rep, KindArtifactMissing)
	if len(evs) != 1 || evs[0].Subject != "controller-image" {
		t.Fatalf("events: %+v", rep.Events)
	}
	ev := evs[0]
	if ev.Status != StatusDrift || ev.Severity != SeverityHigh {
		t.Fatalf("event: %+v", ev)
	}
	if strings.Join(ev.Releases, ",") != "1.2.0,1.2.1" {
		t.Fatalf("releases: %v", ev.Releases)
	}
	if len(ev.Evidence) == 0 {
		t.Fatal("artifact-missing must cite evidence (the reachable channels that said absent)")
	}
	for _, id := range ev.Evidence {
		if _, ok := evidenceByID(rep, id); !ok {
			t.Fatalf("event cites unknown evidence %s", id)
		}
	}
	if ev.Proposal == nil || !strings.Contains(ev.Proposal.YAML, `availability: "<= 1.1.1"`) {
		t.Fatalf("proposal: %+v", ev.Proposal)
	}
	if rep.Summary.Drift != 1 {
		t.Fatalf("summary: %+v", rep.Summary)
	}
}

// A chart that left its primary declared index but is present at a declared
// fallback channel is a move (source-moved), with a channel-reorder proposal.
func TestChartMovedToDeclaredFallback(t *testing.T) {
	w := newWorld()
	w.removeChartFromHelmIndex("v1.2.0", "v1.2.1")
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	evs := eventsOf(rep, KindSourceMoved)
	if len(evs) != 1 || evs[0].Subject != "chart" {
		t.Fatalf("events: %+v", rep.Events)
	}
	ev := evs[0]
	if ev.Status != StatusDrift || ev.Severity != SeverityHigh {
		t.Fatalf("event: %+v", ev)
	}
	if !strings.Contains(ev.Detail, "present at oci ghcr.io/acme/charts/acme") {
		t.Fatalf("detail must name the answering channel: %q", ev.Detail)
	}
	if ev.Proposal == nil || !strings.Contains(ev.Proposal.YAML, "kind: oci, repository: ghcr.io/acme/charts/acme") {
		t.Fatalf("proposal must move the oci channel first: %+v", ev.Proposal)
	}
	yaml := ev.Proposal.YAML
	if strings.Index(yaml, "kind: oci") > strings.Index(yaml, "kind: helm-repo") {
		t.Fatalf("oci channel must be listed first:\n%s", yaml)
	}
}

// A source document that disappears (release notes moved) is source-moved and
// proposes only exceptions — never a guessed new location.
func TestSourceDisappeared(t *testing.T) {
	w := newWorld()
	delete(w.docs, "repo-file:github.com/acme/site@main:content/notes-1.2.md")
	// keep the file for the baseline releases
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	evs := eventsOf(rep, KindSourceMoved)
	found := false
	for _, ev := range evs {
		if ev.Subject == "notes" {
			found = true
			if ev.Status != StatusDrift || ev.SubjectKind != SubjectSource {
				t.Fatalf("event: %+v", ev)
			}
			if ev.Proposal == nil || !strings.Contains(ev.Proposal.YAML, "versions: [1.2.0, 1.2.1]") {
				t.Fatalf("proposal: %+v", ev.Proposal)
			}
			if strings.Contains(ev.Proposal.YAML, "locator") {
				t.Fatal("the proposal must not invent a new location")
			}
		}
	}
	if !found {
		t.Fatalf("no source-moved event for notes: %+v", rep.Events)
	}
}

// An artifact published outside its declared availability window violates it.
func TestAvailabilityViolated(t *testing.T) {
	w := newWorld()
	// the retired legacy image is published again for the new releases
	w.images[legacyRepo+"@v1.2.0"] = "sha256:legacy-still-here"
	w.images[legacyRepo+"@v1.2.1"] = "sha256:legacy-still-here-2"
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	evs := eventsOf(rep, KindAvailabilityViolated)
	if len(evs) != 1 || evs[0].Subject != "legacy-image" {
		t.Fatalf("events: %+v", rep.Events)
	}
	ev := evs[0]
	if ev.Status != StatusDrift || ev.Severity != SeverityLow {
		t.Fatalf("event: %+v", ev)
	}
	if ev.Coordinate != "docker.io/acme/legacy:v1.2.1" {
		t.Fatalf("coordinate: %q", ev.Coordinate)
	}
	if ev.Proposal == nil || !strings.Contains(ev.Proposal.YAML, "availability") {
		t.Fatalf("proposal: %+v", ev.Proposal)
	}
}

// A release-tagged image referenced by a declared manifest but not declared
// itself is a new artifact; the proposal is a full artifact entry.
func TestArtifactAppeared(t *testing.T) {
	w := newWorld()
	for _, tag := range []string{"v1.2.0", "v1.2.1"} {
		w.docs[manifestURL(tag)] = manifestFor(tag, "quay.io/acme/sidecar")
	}
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	evs := eventsOf(rep, KindArtifactAppeared)
	if len(evs) != 1 {
		t.Fatalf("events: %+v", rep.Events)
	}
	ev := evs[0]
	if ev.Subject != "quay.io/acme/sidecar" || ev.SubjectKind != SubjectImage {
		t.Fatalf("event: %+v", ev)
	}
	if ev.Status != StatusDrift || ev.Severity != SeverityLow {
		t.Fatalf("event: %+v", ev)
	}
	if len(ev.Evidence) == 0 {
		t.Fatal("artifact-appeared must cite the snapshot evidence of the referencing artifact")
	}
	if ev.Proposal == nil ||
		!strings.Contains(ev.Proposal.YAML, "- id: sidecar-image") ||
		!strings.Contains(ev.Proposal.YAML, "repository: quay.io/acme/sidecar") ||
		!strings.Contains(ev.Proposal.YAML, `template: "{{.Tag}}"`) {
		t.Fatalf("proposal: %+v", ev.Proposal)
	}
}

// An image with its own versioning (a dependency) is not "appeared".
func TestDependencyImageIsNotAppeared(t *testing.T) {
	w := newWorld()
	for _, tag := range []string{"v1.2.0", "v1.2.1"} {
		w.docs[manifestURL(tag)] = manifestFor(tag, "quay.io/thirdparty/init")
	}
	// rewrite the manifest so the extra image keeps its own tag
	w.docs[manifestURL("v1.2.0")] = strings.ReplaceAll(w.docs[manifestURL("v1.2.0")], "quay.io/thirdparty/init:v1.2.0", "quay.io/thirdparty/init:1.9")
	w.docs[manifestURL("v1.2.1")] = strings.ReplaceAll(w.docs[manifestURL("v1.2.1")], "quay.io/thirdparty/init:v1.2.1", "quay.io/thirdparty/init:1.9")
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	if evs := eventsOf(rep, KindArtifactAppeared); len(evs) != 0 {
		t.Fatalf("dependency images must not be reported: %+v", evs)
	}
}

// A subject that already failed at the baseline is a note, not drift.
func TestAlreadyFailingAtBaselineIsANote(t *testing.T) {
	w := newWorld()
	// the controller image was never published for the baseline releases
	// either (only 1.1.x is the baseline here)
	delete(w.images, quayController+"@v1.1.0")
	delete(w.images, quayController+"@v1.1.1")
	delete(w.images, quayController+"@v1.2.0")
	delete(w.images, quayController+"@v1.2.1")
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, []string{"1.1.0", "1.1.1"})
	evs := eventsOf(rep, KindArtifactMissing)
	if len(evs) != 1 {
		t.Fatalf("events: %+v", rep.Events)
	}
	ev := evs[0]
	if ev.Status != StatusNote || ev.Severity != SeverityLow || rep.Summary.Drift != 0 {
		t.Fatalf("event: %+v summary: %+v", ev, rep.Summary)
	}
	if !strings.Contains(ev.Baseline, "already failing") {
		t.Fatalf("baseline: %q", ev.Baseline)
	}
}

// Without a saved baseline, validatedAgainst in the definition is the
// baseline: drift is still detected (medium severity).
func TestBaselineFromDefinition(t *testing.T) {
	w := newWorld()
	delete(w.images, quayController+"@v1.2.0")
	rep := analyze(w, testDef(), "", []string{"1.2.0", "1.2.1"}, nil)
	if rep.Baseline.Source != BaselineDefinition || rep.Baseline.Cutoff != "1.1.1" {
		t.Fatalf("baseline: %+v", rep.Baseline)
	}
	evs := eventsOf(rep, KindArtifactMissing)
	if len(evs) != 1 || evs[0].Severity != SeverityHigh {
		t.Fatalf("validatedAgainst counts as a held baseline: %+v", evs)
	}
	if !strings.Contains(evs[0].Baseline, "validatedAgainst") {
		t.Fatalf("baseline text: %q", evs[0].Baseline)
	}
}

// Analyze is a pure function of its inputs.
func TestAnalyzeDeterministic(t *testing.T) {
	w := newWorld()
	delete(w.images, quayController+"@v1.2.0")
	run := func() string {
		rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
		b, err := json.Marshal(rep)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if run() != run() {
		t.Fatal("two runs over the same world differ")
	}
}

// The proposal document is a YAML stream whose fragments carry the drift
// context as comments.
func TestRenderProposal(t *testing.T) {
	w := newWorld()
	delete(w.images, quayController+"@v1.2.0")
	delete(w.images, quayController+"@v1.2.1")
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	doc := RenderProposal(rep)
	for _, want := range []string{
		"NOT applied automatically",
		"artifact-missing controller-image",
		`availability: "<= 1.1.1"`,
		"exceptions:",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("proposal lacks %q:\n%s", want, doc)
		}
	}
}

// VersionListFailed maps version-source statuses to the right vocabulary.
func TestVersionListFailed(t *testing.T) {
	def := testDef()
	vl := &ingest.VersionList{Sources: []domain.SourceStatus{
		{SourceID: "git-tags", Kind: "git-tags", State: domain.SourceUnavailable, Detail: "unavailable: no route"},
	}}
	rep := VersionListFailed(def, vl, testNow)
	if len(rep.Events) != 1 || rep.Events[0].Kind != KindUnverifiable || rep.Events[0].Status != StatusUnverifiable {
		t.Fatalf("unreachable versions source must be unverifiable: %+v", rep.Events)
	}
	if rep.Summary.Drift != 0 {
		t.Fatal("an unreachable versions source is not drift")
	}

	// A source that answered but listed nothing matching the tag convention
	// is drift (tag convention change).
	vl.Sources = []domain.SourceStatus{
		{SourceID: "git-tags", Kind: "git-tags", State: domain.SourceNotFound, Detail: "400 tags listed, none is a release of acme"},
	}
	rep = VersionListFailed(def, vl, testNow)
	if len(rep.Events) != 1 || rep.Events[0].Kind != KindRelationshipBroken || rep.Events[0].Status != StatusDrift {
		t.Fatalf("tag convention change must be drift: %+v", rep.Events)
	}
	if rep.Events[0].Severity != SeverityHigh {
		t.Fatalf("severity: %+v", rep.Events[0])
	}
}

func TestSelectReleases(t *testing.T) {
	var all []domain.Version
	for _, s := range []string{"1.0.0", "1.0.1", "1.1.0", "1.1.1", "1.2.0", "1.2.1"} {
		all = append(all, domain.MustVersion("v"+s, s))
	}
	got := SelectReleases(all, "1.1.1", 2)
	if len(got) != 2 || got[0].Semver != "1.2.0" || got[1].Semver != "1.2.1" {
		t.Fatalf("got %v", got)
	}
	if got := SelectReleases(all, "1.2.1", 3); len(got) != 0 {
		t.Fatalf("nothing newer than the cutoff: %v", got)
	}
	if got := SelectReleases(all, "", 3); len(got) != 3 || got[2].Semver != "1.2.1" {
		t.Fatalf("no cutoff: %v", got)
	}
}

func TestBaselineCutoff(t *testing.T) {
	def := testDef()
	if got := BaselineCutoff(def, nil); got != "1.1.1" {
		t.Fatalf("cutoff from validatedAgainst: %q", got)
	}
	base := &ingest.RelationshipReport{Releases: []string{"1.0.0", "1.2.1"}}
	if got := BaselineCutoff(def, base); got != "1.2.1" {
		t.Fatalf("cutoff from saved report: %q", got)
	}
}

// RenderText keeps drift and unverifiable in separate sections.
func TestRenderTextSections(t *testing.T) {
	w := newWorld()
	delete(w.images, quayController+"@v1.2.0")
	w.errs[quayController+"@v1.2.1"] = unavailable("x")
	rep := analyze(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases)
	var b strings.Builder
	if err := RenderText(&b, rep); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"Drift —", "Unverifiable —", "artifact-missing controller-image", "not drift"} {
		if !strings.Contains(out, want) {
			t.Fatalf("text output lacks %q:\n%s", want, out)
		}
	}
	driftAt := strings.Index(out, "Drift —")
	unverAt := strings.Index(out, "Unverifiable —")
	if driftAt < 0 || unverAt < driftAt {
		t.Fatalf("drift section must come first:\n%s", out)
	}
}

func evidenceByID(rep *Report, id domain.EvidenceID) (domain.Evidence, bool) {
	for _, e := range rep.Evidence {
		if e.ID == id {
			return e, true
		}
	}
	return domain.Evidence{}, false
}

// Silence unused warnings for helpers used only by some tests.
var _ = context.Background
var _ = time.Now

// A baseline saved from the same definition revision is current; one saved
// from an older revision is stale (reported, never an event); a legacy report
// without a digest is unrecorded. Staleness must not change the verdict.
func TestBaselineDefinitionDigestStaleness(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ingest.RelationshipReport)
		want   string
	}{
		{"same revision", nil, DigestCurrent},
		{"older revision", func(r *ingest.RelationshipReport) { r.DefinitionDigest = "sha256:0123456789abcdef0123456789abcdef" }, DigestStale},
		{"saved before digests were recorded", func(r *ingest.RelationshipReport) { r.DefinitionDigest = "" }, DigestUnrecorded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld()
			rep := analyzeWith(w, testDef(), "checks/acme.json", []string{"1.2.0", "1.2.1"}, baselineReleases, tc.mutate)
			if rep.Baseline.DigestState != tc.want {
				t.Fatalf("digest state %q, want %q (baseline %q, current %q)", rep.Baseline.DigestState, tc.want, rep.Baseline.DefinitionDigest, rep.DefinitionDigest)
			}
			if len(rep.Events) != 0 || rep.Summary.Drift+rep.Summary.Unverifiable+rep.Summary.Notes != 0 {
				t.Fatalf("staleness must not create events: %+v", rep.Events)
			}
			var out strings.Builder
			if err := RenderText(&out, rep); err != nil {
				t.Fatal(err)
			}
			switch tc.want {
			case DigestStale:
				if !strings.Contains(out.String(), "stale baseline") {
					t.Errorf("text report must flag the stale baseline:\n%s", out.String())
				}
			case DigestUnrecorded:
				if !strings.Contains(out.String(), "records no definition digest") {
					t.Errorf("text report must say the digest is unrecorded:\n%s", out.String())
				}
			default:
				if strings.Contains(out.String(), "stale") || strings.Contains(out.String(), "no definition digest") {
					t.Errorf("current baseline must not warn:\n%s", out.String())
				}
			}
		})
	}
}
