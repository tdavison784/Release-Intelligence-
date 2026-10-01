package discovery

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/ingest"
)

// nginxFixture mirrors kubernetes/ingress-nginx: a controller train
// (controller-v1.x.y), a more numerous chart train (helm-chart-4.x.z), a
// chart-releaser gh-pages workflow, a chart whose appVersion is the
// controller version, provider manifests and a README support table.
var nginxFixture = fixture{
	repo: "github.com/kubernetes/ingress-nginx-like",
	dirs: map[string]string{"github.com/kubernetes/ingress-nginx-like": "testdata/nginxlike"},
	tags: map[string][]string{"github.com/kubernetes/ingress-nginx-like": {
		"helm-chart-4.10.0", "helm-chart-4.10.1", "helm-chart-4.11.0", "helm-chart-4.11.2", "helm-chart-4.12.0",
		"helm-chart-4.12.1", "helm-chart-4.13.0", "helm-chart-4.13.9", "helm-chart-4.14.0", "helm-chart-4.14.5",
		"helm-chart-4.15.0", "helm-chart-4.15.1",
		"controller-v1.11.0", "controller-v1.12.0", "controller-v1.12.8", "controller-v1.13.0", "controller-v1.13.9",
		"controller-v1.14.0", "controller-v1.14.5", "controller-v1.15.0", "controller-v1.15.1"}},
}

func TestTagTrainSwitch(t *testing.T) {
	res := runFixture(t, nginxFixture, nil, Request{NoFollow: true})
	def := res.Definition

	// The chart train has more tags, but the release trigger and the chart
	// appVersion name the controller train.
	if got := def.Versioning.TagPrefix; got != "controller-v" {
		t.Fatalf("tagPrefix = %q, want controller-v (train switch did not happen)", got)
	}
	if def.Versioning.TagPattern == "" {
		t.Fatal("expected a strict tagPattern for the controller train")
	}
	if !strings.Contains(def.Versioning.TagPattern, "controller-v") {
		t.Fatalf("tagPattern %q does not select the controller train", def.Versioning.TagPattern)
	}
	foundSwitch := false
	for _, d := range res.Report.Decisions {
		if d.Rule == "tags.train-switch" {
			foundSwitch = true
		}
	}
	if !foundSwitch {
		t.Fatal("no tags.train-switch decision recorded")
	}
	if res.Report.Tags == nil || res.Report.Tags.TrainSwitch == nil {
		t.Fatal("report does not record the train switch")
	}

	// The chart packages the controller: lookup by appVersion v{{.Version}}.
	chart := artifact(t, def, "ingress-nginx-chart")
	if chart.Version.Strategy != catalog.VersionLookup || chart.Version.Field != "appVersion" {
		t.Fatalf("chart version = %+v, want lookup appVersion", chart.Version)
	}
	if chart.Version.Match != "v"+tmplVersion {
		t.Fatalf("chart lookup match = %q, want v{{.Version}}", chart.Version.Match)
	}
	kinds := map[string]bool{}
	for _, ch := range chart.Channels {
		kinds[ch.Kind] = true
		switch ch.Kind {
		case catalog.LocatorHelmRepo:
			if ch.URL == "https://kubernetes.github.io/ingress-nginx-like" && ch.Chart != "ingress-nginx" {
				t.Fatalf("github.io channel = %+v", ch)
			}
		case catalog.LocatorHelmGit:
			if ch.Path != "charts/ingress-nginx" || !strings.Contains(ch.TagPattern, `helm-chart-`) {
				t.Fatalf("helm-git channel = %+v", ch)
			}
		}
	}
	if !kinds[catalog.LocatorHelmRepo] || !kinds[catalog.LocatorHelmGit] {
		t.Fatalf("chart channels missing kinds: %+v", chart.Channels)
	}
	// the raw gh-pages index is declared as a second helm-repo channel
	raw := false
	for _, ch := range chart.Channels {
		if ch.Kind == catalog.LocatorHelmRepo && strings.Contains(ch.URL, "raw.githubusercontent.com") && strings.Contains(ch.URL, "/gh-pages") {
			raw = true
		}
	}
	if !raw {
		t.Fatalf("no raw gh-pages index channel: %+v", chart.Channels)
	}

	// The controller image is tagged v{{.Version}} (not the controller-v tag).
	img := artifact(t, def, "controller")
	if len(img.Channels) == 0 || img.Channels[0].Repository != "registry.k8s.io/ingress-nginx/controller" {
		t.Fatalf("controller image channels = %+v", img.Channels)
	}
	if img.Version.Template != "v"+tmplVersion {
		t.Fatalf("controller image tag template = %q, want v{{.Version}}", img.Version.Template)
	}

	// Release notes come from the controller changelog, not the chart one.
	notes := source(t, def, "release-notes-docs")
	if !strings.Contains(notes.Locator.Path, "changelog/controller-") {
		t.Fatalf("release notes path = %q, want the controller changelog", notes.Locator.Path)
	}

	// The docs-referenced CRD bundle beats the config/crd directory.
	crds := artifact(t, def, "crds")
	if len(crds.Channels) != 1 || crds.Channels[0].Path != "deploy/crds/bundle.yaml" {
		t.Fatalf("crd channels = %+v, want deploy/crds/bundle.yaml", crds.Channels)
	}

	// The README support table is a compatibility source.
	if _, ok := def.Source("compatibility"); !ok {
		t.Fatal("no compatibility source from the README table")
	}
}

func TestElementStatusVocabulary(t *testing.T) {
	// Everything passes: every element becomes historically-validated.
	pass := &fakeChecker{outcome: func(kind, subject, release string) (string, string) {
		return ingest.OutcomePass, ""
	}}
	res := runFixture(t, nginxFixture, &Discoverer{Checker: pass, ValidationReleases: 4}, Request{NoFollow: true, NoLLM: true})
	seen := map[string]int{}
	for _, e := range res.Report.Elements {
		if e.Status == "" {
			t.Fatalf("element %s has no status", e.Key)
		}
		if !containsStr(AllStatuses, e.Status) {
			t.Fatalf("element %s has status %q outside the closed vocabulary", e.Key, e.Status)
		}
		seen[e.Status]++
		if len(e.Evidence) == 0 {
			t.Fatalf("element %s carries no grounding evidence", e.Key)
		}
		for _, ev := range e.Evidence {
			if !strings.Contains(ev, "#") {
				t.Fatalf("element %s evidence %q has no locator", e.Key, ev)
			}
		}
	}
	if seen[StatusHistoricallyValidated] == 0 {
		t.Fatalf("no historically-validated elements; statuses %v", seen)
	}

	// Channels unreachable: unverified, never "missing"; a failing artifact
	// with no pass is dropped; one failing release out of four is kept as an
	// exception.
	mixed := &fakeChecker{outcome: func(kind, subject, release string) (string, string) {
		if subject == "controller" {
			if release == "controller-v1.15.0" {
				return ingest.OutcomeFail, "registry unreachable"
			}
			return ingest.OutcomePass, ""
		}
		if subject == "crds" {
			return ingest.OutcomeUnverifiable, "repo-dir not probeable"
		}
		if subject == "compatibility" {
			return ingest.OutcomeFail, "no matching row"
		}
		return ingest.OutcomePass, ""
	}}
	res2 := runFixture(t, nginxFixture, &Discoverer{Checker: mixed, ValidationReleases: 4}, Request{NoFollow: true, NoLLM: true})
	st := map[string]string{}
	for _, e := range res2.Report.Elements {
		st[e.Key] = e.Status
	}
	if st["artifact:controller"] != StatusException {
		t.Fatalf("controller (1 failure of 4) status = %q, want exception", st["artifact:controller"])
	}
	if st["artifact:crds"] != StatusUnverified {
		t.Fatalf("crds (unverifiable) status = %q, want unverified", st["artifact:crds"])
	}
	if _, ok := res2.Definition.Source("compatibility"); ok {
		t.Fatal("compatibility source kept although it failed every release")
	}
	if st["artifact:controller"] == StatusHistoricallyValidated {
		t.Fatal("exception must outrank validated")
	}

	// No checker configured: deterministic elements stay discovered (or
	// inferred when their confidence is low), never "validated".
	res3 := runFixture(t, nginxFixture, &Discoverer{}, Request{NoFollow: true, NoLLM: true})
	for _, e := range res3.Report.Elements {
		if e.Status == StatusHistoricallyValidated {
			t.Fatalf("element %s historically-validated without any checker", e.Key)
		}
	}
	// the report renders the status column and the counts line
	md := res3.Report.Markdown()
	if !strings.Contains(md, "## Proposed elements") {
		t.Fatal("report lacks the proposed-elements section")
	}
	if !strings.Contains(md, "Statuses: ") {
		t.Fatal("report lacks the status counts line")
	}
	if !strings.Contains(md, "| artifact:controller |") && !strings.Contains(md, "| source:") {
		t.Fatal("report lacks per-element rows")
	}
}

func TestClassifyTagVPrefix(t *testing.T) {
	if class, tmpl := classifyTag("v1.15.1", "controller-v1.15.1", "1.15.1"); class != tagRelease || tmpl != "v"+tmplVersion {
		t.Fatalf("classifyTag(v1.15.1) = %s, %q", class, tmpl)
	}
	if class, _ := classifyTag("1.15.1", "controller-v1.15.1", "1.15.1"); class != tagRelease {
		t.Fatalf("classifyTag(1.15.1) = %s", class)
	}
}

func TestValuesRegistryJoin(t *testing.T) {
	// The external-secrets shape: image.repository without a host plus
	// global.imageRegistry naming the real registry (ghcr.io), not the
	// implicit Docker Hub namespace.
	res := scanFixture(t, esoFixture, "github.com/example/secretsop", ProfileSource)
	c := mustCand(t, res.Candidates, KindImage, "ghcr.io/example/secretsop/secretsop")
	if c.Attr("tagDefault") != "appVersion" {
		t.Fatalf("tag default attr = %q, want appVersion", c.Attr("tagDefault"))
	}
	if _, ok := findCand(res.Candidates, KindImage, "docker.io/example/secretsop/secretsop"); ok {
		t.Fatal("hostless repository fell through to the implicit Docker Hub namespace")
	}
	// The chart appVersion relation is proposed from Chart.yaml at the ref.
	vr, ok := findCand(res.Candidates, KindVersionRelation, "chart.appVersion = "+tmplTag)
	if !ok {
		t.Fatalf("no chart.appVersion relation from Chart.yaml appVersion v2.11.0 at tag v2.11.0")
	}
	if vr.Attr("appVersion") != "v2.11.0" {
		t.Fatalf("appVersion attr = %q", vr.Attr("appVersion"))
	}
	// The ingress-nginx shape: registry + image (with slash) + tag.
	res2 := scanFixture(t, nginxFixture, nginxFixture.repo, ProfileSource)
	if _, ok := findCand(res2.Candidates, KindImage, "registry.k8s.io/ingress-nginx/controller"); !ok {
		t.Fatal("registry+image+tag values shape not joined")
	}
}

// esoFixture mirrors external-secrets: chart in the repo, values with a
// global imageRegistry and a hostless image.repository.
var esoFixture = fixture{
	repo: "github.com/example/secretsop",
	dirs: map[string]string{"github.com/example/secretsop": "testdata/esolike"},
	tags: map[string][]string{"github.com/example/secretsop": {
		"v2.8.0", "v2.9.0", "v2.10.0", "v2.10.4", "v2.11.0"}},
}

func TestChartAppVersionLookupResolver(t *testing.T) {
	res := runFixture(t, esoFixture, nil, Request{NoFollow: true})
	chart := artifact(t, res.Definition, "secretsop-chart")
	if chart.Version.Strategy != catalog.VersionLookup || chart.Version.Field != "appVersion" {
		t.Fatalf("chart version = %+v, want lookup appVersion", chart.Version)
	}
	if chart.Version.Match != tmplTag {
		t.Fatalf("lookup match = %q, want {{.Tag}}", chart.Version.Match)
	}
}

func TestTagFamiliesLatestByVersion(t *testing.T) {
	// ls-remote order is lexicographic: v1.9.0 sorts after v1.15.0. The
	// family latest must be the newest version, not the last listed.
	repo := mustRepo(t, "github.com/example/ordered")
	ta := AnalyzeTags(repo, remoteTags("v1.15.0", "v1.9.0", "v1.2.0", "v1.1.0", "v1.0.0", "v1.14.0", "v1.13.0", "v1.12.0"))
	if len(ta.Families) != 1 || ta.Families[0].Latest != "v1.15.0" {
		t.Fatalf("families = %+v, want latest v1.15.0", ta.Families)
	}
}

func TestPromoteLineImage(t *testing.T) {
	// A manual promotion workflow whose dispatch input names a version
	// publishes release-tagged images even when the tag is a variable
	// argument (crossplane's promote-images).
	res := scanFixture(t, promoteFixture, promoteFixture.repo, ProfileSource)
	c := mustCand(t, res.Candidates, KindImage, "ghcr.io/example/promoter/promoter")
	if !setOf(c.Attr("classes"))[tagRelease] {
		t.Fatalf("promote image classes = %q, want release", c.Attr("classes"))
	}
	if c.Attr("tagTemplate") != tmplTag {
		t.Fatalf("promote image template = %q", c.Attr("tagTemplate"))
	}
}

var promoteFixture = fixture{
	repo: "github.com/example/promoter",
	dirs: map[string]string{"github.com/example/promoter": "testdata/promotelike"},
	tags: map[string][]string{"github.com/example/promoter": {"v1.0.0", "v1.1.0", "v1.2.0", "v1.2.1"}},
}

func TestDevChartChannelDropped(t *testing.T) {
	res := runFixture(t, nginxFixture, nil, Request{NoFollow: true})
	chart := artifact(t, res.Definition, "ingress-nginx-chart")
	for _, ch := range chart.Channels {
		if ch.Kind == catalog.LocatorHelmRepo && strings.Contains(ch.URL, "/master") {
			t.Fatalf("development channel kept: %+v", ch)
		}
	}
}
