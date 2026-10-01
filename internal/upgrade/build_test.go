package upgrade

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// bare returns a release whose release-notes source succeeded (so tests are
// not dominated by gap warnings).
func bare(tag string) *rel {
	return newRel("demo", tag).source("notes", "repo-file", domain.SourceOK, "", domain.RoleReleaseNotes)
}

// simpleInput builds an Input for from → path… → to with PolicyAll.
func simpleInput(from, to *rel, mid ...*rel) Input {
	var path []*domain.Release
	var vs []domain.Version
	for _, m := range mid {
		path = append(path, m.r)
		vs = append(vs, m.r.Version)
	}
	path = append(path, to.r)
	vs = append(vs, to.r.Version)
	return Input{
		From: from.r, To: to.r, Path: path,
		Selection: &PathSelection{Policy: PolicyAll, Path: vs, Skipped: []domain.Version{}},
		Now:       fixedNow,
	}
}

func TestBuildFixturesValidateAndAreDeterministic(t *testing.T) {
	for name, mk := range map[string]func(testing.TB) Input{"cert-manager": cmInput, "argo-cd": argoInput} {
		t.Run(name, func(t *testing.T) {
			a := mustBuild(t, mk(t))
			b := mustBuild(t, mk(t))
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
			ja, _ := json.Marshal(a)
			jb, _ := json.Marshal(b)
			if string(ja) != string(jb) {
				t.Fatal("Build is not deterministic")
			}
			if a.SchemaVersion != domain.UpgradeEdgeSchemaVersion || !a.GeneratedAt.Equal(fixedNow) {
				t.Errorf("header: %s %s", a.SchemaVersion, a.GeneratedAt)
			}
			ids := map[string]bool{}
			for _, c := range a.Changes {
				if ids[c.ID] {
					t.Errorf("duplicate change id %s", c.ID)
				}
				ids[c.ID] = true
				if c.Title == "" || len([]rune(c.Title)) > MaxTitle {
					t.Errorf("bad title %q", c.Title)
				}
			}
			assertSorted(t, a.Changes)
		})
	}
}

func assertSorted(t *testing.T, cs []domain.Change) {
	t.Helper()
	key := func(c domain.Change) []int {
		b, a := 1, 1
		if c.Breaking {
			b = 0
		}
		if c.ActionRequired {
			a = 0
		}
		return []int{b, a, catIdx(c.Category)}
	}
	for i := 1; i < len(cs); i++ {
		p, c := key(cs[i-1]), key(cs[i])
		for k := range p {
			if p[k] < c[k] {
				break
			}
			if p[k] > c[k] {
				t.Fatalf("changes not sorted at %d: %q before %q", i, cs[i-1].Title, cs[i].Title)
			}
		}
		if reflect.DeepEqual(p, c) && compareReleaseStrings(cs[i-1].Release, cs[i].Release) > 0 {
			t.Fatalf("release order at %d: %q before %q", i, cs[i-1].Release, cs[i].Release)
		}
	}
}

func TestChangeOrdering(t *testing.T) {
	prov := computed("x")
	cs := []domain.Change{
		{ID: "1", Title: "b feature", Category: domain.CategoryFeature, Release: "1.2.0", Provenance: prov},
		{ID: "2", Title: "a feature", Category: domain.CategoryFeature, Release: "1.2.0", Provenance: prov},
		{ID: "3", Title: "old feature", Category: domain.CategoryFeature, Release: "1.10.0", Provenance: prov},
		{ID: "4", Title: "diff", Category: domain.CategoryFeature, Provenance: prov},
		{ID: "5", Title: "values", Category: domain.CategoryHelmValues, ActionRequired: true, Provenance: prov},
		{ID: "6", Title: "migration", Category: domain.CategoryMigration, ActionRequired: true, Provenance: prov},
		{ID: "7", Title: "break", Category: domain.CategoryBugfix, Breaking: true, Provenance: prov},
		{ID: "8", Title: "deprecation", Category: domain.CategoryDeprecation, Provenance: prov},
	}
	sortChanges(cs)
	var got []string
	for _, c := range cs {
		got = append(got, c.ID)
	}
	want := []string{"7", "6", "5", "8", "2", "1", "3", "4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestBuildErrors(t *testing.T) {
	a, b := bare("v1.0.0"), bare("v1.1.0")
	if _, err := Build(Input{From: a.r}); err == nil {
		t.Error("expected error without To")
	}
	if _, err := Build(simpleInput(b, a)); err == nil || !strings.Contains(err.Error(), "lower") {
		t.Errorf("expected from<to error, got %v", err)
	}
}

func TestPathStepsAndFallbacks(t *testing.T) {
	e := mustBuild(t, cmInput(t))
	if e.PathPolicy != PolicyMinorLineage || len(e.Path) != 2 {
		t.Fatalf("path: %s %+v", e.PathPolicy, e.Path)
	}
	if e.Path[0].Reason != ReasonMinorRelease || e.Path[1].Reason != ReasonTargetLinePatch || e.Path[0].PublishedAt == nil {
		t.Errorf("steps: %+v", e.Path)
	}
	if got := tagsOf(e.SkippedReleases); !reflect.DeepEqual(got, []string{"v1.14.5", "v1.14.6", "v1.14.7"}) {
		t.Errorf("skipped = %v", got)
	}

	// no Selection: policy derived from the definition, path from ingested releases;
	// a selection version that was not ingested yields a warning.
	in := cmInput(t)
	in.Selection = nil
	e = mustBuild(t, in)
	if e.PathPolicy != PolicyMinorLineage || len(e.Path) != 2 {
		t.Errorf("fallback path: %s %+v", e.PathPolicy, e.Path)
	}
	in = cmInput(t)
	in.Path = []*domain.Release{in.To}
	e = mustBuild(t, in)
	if !hasWarning(e, "v1.15.0 is on the upgrade path but was not ingested") {
		t.Errorf("missing not-ingested warning: %v", e.Warnings)
	}
	for _, c := range e.Changes {
		if c.Provenance.Producer == "normalize.notes@v1" && c.Release == "1.15.0" {
			t.Errorf("notes of a non-ingested release must not appear: %q", c.Title)
		}
	}
}

// ---------------------------------------------------------------- notes

func TestNoteChanges(t *testing.T) {
	e := mustBuild(t, cmInput(t))
	if findTitle(e, "must not be reported") != nil {
		t.Error("notes of the From release must not be included")
	}
	panics := e.ChangesWhere(func(c domain.Change) bool { return strings.Contains(c.Title, "Fix a panic in the ACME issuer") })
	if len(panics) != 1 {
		t.Fatalf("backport duplicate not merged: %d changes", len(panics))
	}
	p := panics[0]
	if p.Release != "1.15.0" || len(p.Evidence) != 2 || p.Category != domain.CategoryBugfix {
		t.Errorf("merged change: release=%s evidence=%v category=%s", p.Release, p.Evidence, p.Category)
	}
	if strings.Contains(p.Title, "#7001") || !strings.Contains(p.Detail, "#7001") {
		t.Errorf("title should drop PR refs, detail keep full text: %q / %q", p.Title, p.Detail)
	}

	brk := findTitle(e, "no longer published from this repository")
	if brk == nil || !brk.Breaking || brk.Provenance.Method != domain.MethodDeclared || brk.Category != domain.CategoryRemoval {
		t.Fatalf("breaking note: %+v", brk)
	}
	if strings.Contains(brk.Title, "cmctl now lives") || !strings.Contains(brk.Detail, "cmctl now lives") {
		t.Errorf("title must be the first sentence and detail the full text: %q", brk.Title)
	}
	mig := findTitle(e, "switch to the standalone cmctl release")
	if mig == nil || !mig.ActionRequired || mig.Category != domain.CategoryMigration {
		t.Errorf("upgrade-guide item must keep ActionRequired: %+v", mig)
	}
	sec := findTitle(e, "CVE-2024-45338")
	if sec == nil || sec.Provenance.Method != domain.MethodHeuristic || len(sec.References) != 1 || sec.References[0].ID != "CVE-2024-45338" {
		t.Errorf("security note: %+v", sec)
	}
}

func TestNoteMergeKeepsStrongestClassification(t *testing.T) {
	from := bare("v1.17.0")
	mid := bare("v1.18.0").note("notes", domain.RoleReleaseNotes, "https://notes/1.18", "Other", "Remove the deprecated `--legacy` flag (#100).", domain.CategoryOther, prov(heuristic("default")))
	to := bare("v1.18.1").
		note("upgrade", domain.RoleUpgradeGuide, "https://upgrade/1.18", "Breaking", "Remove the deprecated `--legacy` flag (#120, @bot)", domain.CategoryRemoval, action, breaking).
		note("notes", domain.RoleReleaseNotes, "https://notes/1.18", "Other", "", domain.CategoryOther).
		note("notes", domain.RoleReleaseNotes, "https://notes/1.18", "Other", "Item with invalid provenance", domain.CategoryOther, prov(domain.Provenance{Method: "guess"})).
		note("notes", domain.RoleReleaseNotes, "https://notes/1.18", "Other", "Uncategorised item", "")
	e := mustBuild(t, simpleInput(from, to, mid))
	cs := e.ChangesWhere(func(c domain.Change) bool { return strings.Contains(c.Title, "--legacy") })
	if len(cs) != 1 {
		t.Fatalf("expected one merged change, got %d", len(cs))
	}
	c := cs[0]
	if c.Release != "1.18.0" || !c.Breaking || !c.ActionRequired || c.Category != domain.CategoryRemoval || c.Provenance.Method != domain.MethodDeclared || len(c.Evidence) != 2 {
		t.Errorf("merged: %+v", c)
	}
	if findTitle(e, "invalid provenance") != nil || !hasWarning(e, "1 release-note item was skipped") {
		t.Errorf("invalid provenance must be dropped with a warning: %v", e.Warnings)
	}
	if u := findTitle(e, "Uncategorised"); u == nil || u.Category != domain.CategoryOther {
		t.Errorf("empty category must become other: %+v", u)
	}
	if findTitle(e, "(empty note)") != nil {
		t.Error("empty note text must be skipped")
	}
}

func TestNoteTitle(t *testing.T) {
	tests := []struct {
		text, title string
		more        bool
	}{
		{"Fix a panic in the ACME issuer", "Fix a panic in the ACME issuer", false},
		{"Fix a panic. It happened on restart.", "Fix a panic", true},
		{"Support e.g. PKCS#12 keystores. Second sentence.", "Support e.g. PKCS#12 keystores", true},
		{"- **Deprecated** the `sidecar.istio.io/statsCompression` annotation ([Issue #48051](https://github.com/istio/istio/issues/48051))",
			"Deprecated the `sidecar.istio.io/statsCompression` annotation", true},
		{"The binary is no longer published. cmctl now lives elsewhere.", "The binary is no longer published", true},
		{"First line\nsecond line", "First line", true},
		{"Bump Go (see docs. Really) to 1.22.", "Bump Go (see docs. Really) to 1.22", false},
	}
	for _, tt := range tests {
		title, more := noteTitle(tt.text)
		if title != tt.title || more != tt.more {
			t.Errorf("noteTitle(%q) = %q, %v; want %q, %v", tt.text, title, more, tt.title, tt.more)
		}
	}
}

func TestNoteTitleBounded(t *testing.T) {
	title, more := noteTitle(strings.Repeat("word ", 60))
	if n := len([]rune(title)); n > MaxTitle || n < MaxTitle-10 || !strings.HasSuffix(title, "word…") || !more {
		t.Errorf("bounded title (%d runes): %q", n, title)
	}
}

func TestNormalizeNoteText(t *testing.T) {
	same := [][2]string{
		{"Fix X ([`#7001`](https://github.com/o/r/pull/7001), [`@wallrj`](https://github.com/wallrj))", "fix x"},
		{"Fix X (#7123, @cert-manager-bot).", "fix x"},
		{"- **Fix** `X`", "fix x"},
		{"fix:   x (https://github.com/o/r/pull/1)", "fix: x"},
	}
	for _, p := range same {
		if got := normalizeNoteText(p[0]); got != p[1] {
			t.Errorf("normalizeNoteText(%q) = %q, want %q", p[0], got, p[1])
		}
	}
	if normalizeNoteText("Fix X (see #12 for details)") == "fix x" {
		t.Error("prose in parentheses must be kept")
	}
}

// ---------------------------------------------------------------- values

func TestValuesDiff(t *testing.T) {
	from := bare("v1.0.0").values("chart", "demo", "https://x/v1.0.0/values.yaml",
		"a.x", "1", "a.y", "2", // top-level section removed → breaking
		"g.only", "1", // single-key top-level section removed → breaking
		"b.keep", "1", "b.gone", "2", // per-key removal
		"b.sub.one", "1", "b.sub.two", "2", // sub-section removed → grouped
		"c.leaf", `"x"`, // becomes a section → not removed
		"d", "1", // default changed
		"e", "true", // top-level scalar removed → not breaking
	)
	from.r.Snapshots[0].Values.Comments = map[string]string{"b.gone": "gone is going away"}
	to := bare("v1.1.0").values("chart", "demo", "https://x/v1.1.0/values.yaml",
		"b.keep", "1", "b.new1", "1", "b.new2", "2",
		"c.leaf.inner", "1",
		"d", "2",
		"f.one", "1", "f.two", "2",
	)
	e := mustBuild(t, simpleInput(from, to))

	type want struct {
		title            string
		breaking, action bool
		rule             string
		subjects         []string
	}
	for _, w := range []want{
		{"Helm values section `a.*` removed (2 keys)", true, true, RuleValuesSectionRemoved, []string{"a.x", "a.y"}},
		{"Helm values section `g.*` removed (1 key)", true, true, RuleValuesSectionRemoved, []string{"g.only"}},
		{"Helm value `b.gone` removed", false, true, RuleValuesRemoved, []string{"b.gone"}},
		{"Helm values `b.sub.*` removed (2 keys)", false, true, RuleValuesRemoved, []string{"b.sub.one", "b.sub.two"}},
		{"Helm value `e` removed", false, true, RuleValuesRemoved, []string{"e"}},
		{"Default of Helm value `d` changed: 1 → 2", false, false, RuleValuesDefaultChanged, []string{"d"}},
		{"2 new Helm values under `b.*`", false, false, RuleValuesAdded, []string{"b.new1", "b.new2"}},
		{"New Helm value `c.leaf.inner`", false, false, RuleValuesAdded, []string{"c.leaf.inner"}},
		{"New Helm values section `f.*` (2 values)", false, false, RuleValuesAdded, []string{"f.one", "f.two"}},
	} {
		c := findTitle(e, w.title)
		if c == nil {
			t.Errorf("missing change %q", w.title)
			continue
		}
		if c.Breaking != w.breaking || c.ActionRequired != w.action || c.Provenance.Rule != w.rule || !reflect.DeepEqual(c.Subjects, w.subjects) {
			t.Errorf("%q: breaking=%v action=%v rule=%s subjects=%v", w.title, c.Breaking, c.ActionRequired, c.Provenance.Rule, c.Subjects)
		}
		if c.Category != domain.CategoryHelmValues || c.Provenance.Method != domain.MethodComputed || c.Provenance.Producer != Producer ||
			c.Provenance.Confidence != domain.ConfidenceHigh || c.Release != "" || len(c.Evidence) != 2 || len(c.Facts) != 2 {
			t.Errorf("%q: unexpected metadata %+v", w.title, c)
		}
	}
	if c := findTitle(e, "`c.leaf` removed"); c != nil {
		t.Error("a leaf that became a section must not be reported as removed")
	}
	if c := findTitle(e, "`b.gone` removed"); c == nil || !strings.Contains(c.Detail, "gone is going away") || !strings.Contains(c.Detail, "Default in v1.0.0: 2") {
		t.Errorf("removed key detail: %+v", c)
	}
	if c := findTitle(e, "`d` changed"); c == nil || c.Detail != "1 → 2" {
		t.Errorf("changed default detail: %+v", c)
	}
	if n := len(findChanges(e, RuleValuesRemoved)) + len(findChanges(e, RuleValuesSectionRemoved)); n != 5 {
		t.Errorf("removal changes = %d, want 5", n)
	}
}

func TestValuesDiffMultipleChartsAndGaps(t *testing.T) {
	from := bare("v1.0.0").
		values("base", "base", "https://x/base-1.0.0", "k", "1").
		values("istiod", "istiod", "https://x/istiod-1.0.0", "k", "1").
		values("gateway", "gateway", "https://x/gateway-1.0.0", "k", "1")
	to := bare("v1.1.0").
		values("base", "base", "https://x/base-1.1.0", "k", "2").
		values("istiod", "istiod", "https://x/istiod-1.1.0", "k", "3")
	e := mustBuild(t, simpleInput(from, to))
	if findTitle(e, "(chart base)") == nil || findTitle(e, "(chart istiod)") == nil {
		t.Errorf("chart suffix expected with several charts: %v", e.Changes)
	}
	if !hasWarning(e, "Helm values of gateway were not captured for v1.1.0") {
		t.Errorf("one-sided snapshot warning missing: %v", e.Warnings)
	}
}

// ---------------------------------------------------------------- CRDs

func TestCRDDiff(t *testing.T) {
	from := bare("v1.0.0").crds("crds", "https://x/v1.0.0/crds.yaml",
		crd("foos.example.io", "example.io", "Foo",
			crdVer("v1alpha1", true, false),
			crdVer("v1alpha2", true, false),
			crdVer("v1beta1", true, true, "spec", "spec.a", "spec.a.b", "spec.c"),
		),
		crd("bars.example.io", "example.io", "Bar", crdVer("v1", true, true)),
		crd("quxes.example.io", "example.io", "Qux", crdVer("v1", true, true, "spec", "spec.q")),
		crd("olds.example.io", "example.io", "Old", crdVer("v1", true, true), crdVer("v0", false, false)),
	)
	deprecated := crdVer("v1beta1", true, false, "spec", "spec.c", "spec.d")
	deprecated.Deprecated, deprecated.DeprecationWarning = true, "example.io/v1beta1 Foo is deprecated; use v1"
	to := bare("v1.1.0").crds("crds", "https://x/v1.1.0/crds.yaml",
		crd("foos.example.io", "example.io", "Foo",
			crdVer("v1alpha2", false, false),
			deprecated,
			crdVer("v1", true, true),
		),
		crd("bazs.example.io", "example.io", "Baz", crdVer("v1", true, true)),
		crd("quxes.example.io", "example.io", "Qux", crdVer("v1", true, true)),
		crd("olds.example.io", "example.io", "Old", crdVer("v1", true, true)),
	)
	e := mustBuild(t, simpleInput(from, to))
	type want struct {
		title            string
		cat              domain.Category
		breaking, action bool
		rule             string
	}
	for _, w := range []want{
		{"CRD `bars.example.io` (Bar) removed", domain.CategoryCRDSchema, true, false, RuleCRDRemoved},
		{"New CRD `bazs.example.io` (Baz, v1)", domain.CategoryCRDSchema, false, false, RuleCRDAdded},
		{"API version `example.io/v1alpha1` of Foo removed", domain.CategoryAPI, true, true, RuleCRDVersionRemoved},
		{"API version `example.io/v1alpha2` of Foo no longer served", domain.CategoryAPI, true, true, RuleCRDVersionUnserved},
		{"API version `example.io/v1beta1` of Foo deprecated", domain.CategoryDeprecation, false, false, RuleCRDVersionDeprecated},
		{"New API version `example.io/v1` of Foo", domain.CategoryAPI, false, false, RuleCRDVersionAdded},
		{"Storage version of `foos.example.io` changes v1beta1 → v1", domain.CategoryAPI, false, true, RuleCRDStorageChanged},
		{"Foo v1beta1 schema: 1 field removed: `spec.a`", domain.CategoryCRDSchema, true, false, RuleCRDFieldsRemoved},
		{"Foo v1beta1 schema: 1 field added: `spec.d`", domain.CategoryCRDSchema, false, false, RuleCRDFieldsAdded},
		{"Unserved API version `example.io/v0` of Old removed", domain.CategoryAPI, false, false, RuleCRDVersionRemoved},
	} {
		c := findTitle(e, w.title)
		if c == nil {
			t.Errorf("missing change %q", w.title)
			continue
		}
		if c.Category != w.cat || c.Breaking != w.breaking || c.ActionRequired != w.action || c.Provenance.Rule != w.rule || len(c.Evidence) != 2 {
			t.Errorf("%q: %+v", w.title, c)
		}
	}
	if c := findTitle(e, "field removed: `spec.a`"); c == nil || !reflect.DeepEqual(c.Subjects, []string{"spec.a", "spec.a.b"}) {
		t.Errorf("removed paths must all be kept in Subjects: %+v", c)
	}
	if c := findTitle(e, "deprecated"); c == nil || !strings.Contains(c.Detail, "use v1") {
		t.Errorf("deprecation warning text expected in detail: %+v", c)
	}
	if !hasWarning(e, "Schema of quxes.example.io v1 was not captured for v1.1.0") {
		t.Errorf("schema gap warning missing: %v", e.Warnings)
	}
	if findTitle(e, "Qux v1 schema") != nil {
		t.Error("missing target schema must not be reported as removed fields")
	}
}

func TestCRDFieldTitleTruncation(t *testing.T) {
	var paths []string
	for _, p := range []string{"a", "b", "c", "d", "e"} {
		paths = append(paths, "spec."+p)
	}
	from := bare("v1.0.0").crds("crds", "https://x/1", crd("foos.example.io", "example.io", "Foo", crdVer("v1", true, true, append([]string{"spec"}, paths...)...)))
	to := bare("v1.1.0").crds("crds", "https://x/2", crd("foos.example.io", "example.io", "Foo", crdVer("v1", true, true, "spec")))
	e := mustBuild(t, simpleInput(from, to))
	c := findTitle(e, "5 fields removed")
	if c == nil || !strings.Contains(c.Title, "`spec.a`, `spec.b`, `spec.c` and 2 more") || len(c.Subjects) != 5 {
		t.Fatalf("truncated title: %+v", c)
	}
}

// ---------------------------------------------------------------- artifacts

func TestArtifactChanges(t *testing.T) {
	from := bare("v1.14.4").
		artifact("ctl", domain.ArtifactContainerImage, "cert-manager-ctl", "quay.io/jetstack/cert-manager-ctl:v1.14.4", domain.ArtifactVerified, "").
		artifact("startupapicheck", domain.ArtifactContainerImage, "cert-manager-startupapicheck", "", domain.ArtifactNotApplicable, "availability >= 1.15.0").
		artifact("controller", domain.ArtifactContainerImage, "cert-manager-controller", "quay.io/jetstack/cert-manager-controller:v1.14.4", domain.ArtifactVerified, "").
		artifact("crds", domain.ArtifactCRD, "crds", "https://x/crds.yaml", domain.ArtifactVerified, "").
		artifact("webhook", domain.ArtifactContainerImage, "cert-manager-webhook", "quay.io/jetstack/cert-manager-webhook:v1.14.4", domain.ArtifactVerified, "").
		artifact("legacy", domain.ArtifactBinary, "legacy", "", domain.ArtifactNotApplicable, "")
	to := bare("v1.15.0").
		artifact("ctl", domain.ArtifactContainerImage, "cert-manager-ctl", "", domain.ArtifactNotApplicable, "availability < 1.15.0").
		artifact("startupapicheck", domain.ArtifactContainerImage, "cert-manager-startupapicheck", "quay.io/jetstack/cert-manager-startupapicheck:v1.15.0", domain.ArtifactReferenced, "").
		artifact("controller", domain.ArtifactContainerImage, "cert-manager-controller", "quay.io/jetstack/cert-manager-controller:v1.15.0", domain.ArtifactExpected, "").
		artifact("crds", domain.ArtifactCRD, "crds", "https://x/crds.yaml", domain.ArtifactVerified, "").
		artifact("webhook", domain.ArtifactContainerImage, "cert-manager-webhook", "quay.io/jetstack/cert-manager-webhook:v1.15.0", domain.ArtifactMissing, "manifest unknown").
		artifact("legacy", domain.ArtifactBinary, "legacy", "", domain.ArtifactNotApplicable, "")
	in := simpleInput(from, to)
	in.Definition = &catalog.ProductDefinition{ID: "demo", Name: "Demo", Artifacts: []catalog.Artifact{{ID: "controller"}, {ID: "webhook"}, {ID: "startupapicheck"}, {ID: "ctl"}}}
	e := mustBuild(t, in)

	got := map[string]domain.ArtifactChange{}
	var order []string
	for _, ac := range e.Artifacts {
		got[ac.ArtifactID] = ac
		order = append(order, ac.ArtifactID)
	}
	if !reflect.DeepEqual(order, []string{"controller", "webhook", "startupapicheck", "ctl", "crds"}) {
		t.Errorf("artifact order = %v (definition order, then id)", order)
	}
	if _, ok := got["legacy"]; ok {
		t.Error("artifact not applicable on both sides must be omitted")
	}
	if ac := got["ctl"]; ac.Change != domain.ChangeRemoved || ac.From == nil || ac.To == nil || ac.To.Status != domain.ArtifactNotApplicable || ac.From.Status != domain.ArtifactVerified {
		t.Errorf("ctl: %+v", ac)
	}
	if ac := got["startupapicheck"]; ac.Change != domain.ChangeAdded || ac.To.Status != domain.ArtifactReferenced || ac.From.Status != domain.ArtifactNotApplicable {
		t.Errorf("startupapicheck: %+v", ac)
	}
	if ac := got["controller"]; ac.Change != domain.ChangeUpdated || ac.To.Status != domain.ArtifactExpected || ac.From.Status != domain.ArtifactVerified {
		t.Errorf("controller must keep both statuses: %+v", ac)
	}
	if ac := got["crds"]; ac.Change != domain.ChangeUnchanged {
		t.Errorf("crds: %+v", ac)
	}
	rm := findTitle(e, "`cert-manager-ctl` is no longer published")
	if rm == nil || !rm.ActionRequired || rm.Category != domain.CategoryArtifact || rm.Provenance.Rule != RuleArtifactRemoved || !strings.Contains(rm.Detail, "availability < 1.15.0") {
		t.Errorf("removed artifact change: %+v", rm)
	}
	add := findTitle(e, "New container image `cert-manager-startupapicheck` published")
	if add == nil || add.ActionRequired || !strings.Contains(add.Detail, "mirror") || !strings.Contains(add.Detail, "referenced") {
		t.Errorf("added artifact change: %+v", add)
	}
	if !hasWarning(e, "webhook of v1.15.0 is missing at its channel") {
		t.Errorf("missing-artifact warning: %v", e.Warnings)
	}
	if !hasWarning(e, "1 artifact of v1.15.0 only expected") {
		t.Errorf("expected-artifact warning: %v", e.Warnings)
	}
}

func TestArtifactsAllExpectedAndNoEvidence(t *testing.T) {
	from := bare("v1.0.0").artifact("img", domain.ArtifactContainerImage, "img", "r/img:v1.0.0", domain.ArtifactExpected, "")
	to := bare("v1.1.0").
		artifact("img", domain.ArtifactContainerImage, "img", "r/img:v1.1.0", domain.ArtifactExpected, "").
		artifact("new", domain.ArtifactContainerImage, "new", "r/new:v1.1.0", domain.ArtifactExpected, "")
	e := mustBuild(t, simpleInput(from, to))
	if !hasWarning(e, "None of the 2 artifacts of v1.1.0 could be verified") {
		t.Errorf("all-expected warning: %v", e.Warnings)
	}
	// an added artifact without any evidence is visible as ArtifactChange and
	// warning, but not as an (unsupported) Change
	if findTitle(e, "`new`") != nil || !hasWarning(e, "no evidence is available: New container image `new`") {
		t.Errorf("evidence-less change must become a warning: %v", e.Warnings)
	}
	if len(e.Artifacts) != 2 || e.Artifacts[1].Change != domain.ChangeAdded {
		t.Errorf("artifacts: %+v", e.Artifacts)
	}
}

func TestPickInstancesPrefersConfirmedAndHighest(t *testing.T) {
	got := pickInstances([]domain.ArtifactInstance{
		{ArtifactID: "chart", Version: "8.5.8", Status: domain.ArtifactVerified},
		{ArtifactID: "chart", Version: "9.0.0", Status: domain.ArtifactVerified},
		{ArtifactID: "chart", Version: "9.1.0", Status: domain.ArtifactExpected},
	})
	if got["chart"].Version != "9.0.0" {
		t.Errorf("picked %s", got["chart"].Version)
	}
}

// ---------------------------------------------------------------- images

func TestImageChanges(t *testing.T) {
	e := mustBuild(t, argoInput(t))
	dex := findTitle(e, "Third-party image `ghcr.io/dexidp/dex`: v2.41.1 → v2.43.0")
	if dex == nil || dex.Category != domain.CategoryDependency || dex.Provenance.Rule != RuleImageTagsChanged {
		t.Errorf("dex bump: %+v", dex)
	}
	redis := findChanges(e, RuleImageMoved)
	if len(redis) != 1 || !reflect.DeepEqual(redis[0].Subjects, []string{"redis", "public.ecr.aws/docker/library/redis"}) {
		t.Errorf("redis move: %+v", redis)
	}
	for _, c := range e.Changes {
		if strings.Contains(c.Title, "quay.io/argoproj/argocd") || strings.Contains(c.Title, "haproxy") {
			t.Errorf("product image / unchanged image reported as dependency change: %q", c.Title)
		}
	}

	from := bare("v1.0.0").images("manifest", "https://x/1/install.yaml", "example.io/app:v1.0.0", "docker.io/library/nginx:1.25", "docker.io/library/busybox:1.36")
	to := bare("v1.1.0").images("manifest", "https://x/2/install.yaml", "example.io/app:v1.1.0", "docker.io/library/busybox:1.36", "ghcr.io/example/sidecar:v0.3.0", "example.io/agent:v1.1.0")
	e = mustBuild(t, simpleInput(from, to))
	for title, cat := range map[string]domain.Category{
		"Third-party image `docker.io/library/nginx:1.25` no longer referenced": domain.CategoryDependency,
		"New third-party image `ghcr.io/example/sidecar:v0.3.0`":                domain.CategoryDependency,
		"Manifests now reference image `example.io/agent:v1.1.0`":               domain.CategoryArtifact,
	} {
		if c := findTitle(e, title); c == nil || c.Category != cat || len(c.Evidence) != 2 {
			t.Errorf("missing %q (%s): %+v", title, cat, c)
		}
	}
	if findTitle(e, "example.io/app") != nil {
		t.Error("an image following the release tag is the product's own image")
	}
}

// ---------------------------------------------------------------- compatibility

func TestCompareConstraints(t *testing.T) {
	c := func(constraint, raw string, vs ...string) *domain.CompatibilityConstraint {
		return &domain.CompatibilityConstraint{Platform: "kubernetes", Constraint: constraint, Raw: raw, Versions: vs}
	}
	tests := []struct {
		name                     string
		f, t                     *domain.CompatibilityConstraint
		fromDisp, toDisp         string
		drops, adds              string
		narrowed, unchanged, cmp bool
	}{
		{"shifted range", c(">=1.29.0-0 <=1.33.x", "1.29 → 1.33"), c(">=1.30.0-0 <=1.34.x", "1.30 → 1.34"), "1.29–1.33", "1.30–1.34", "1.29", "1.34", true, false, true},
		{"widened", c(">=1.29.0-0 <=1.32.x", "1.29 → 1.32"), c(">=1.29.0-0 <=1.33.x", "1.29 → 1.33"), "1.29–1.32", "1.29–1.33", "", "1.33", false, false, true},
		{"explicit lists", c("", "", "v1.32", "v1.31", "v1.30", "v1.29"), c("", "", "v1.33", "v1.32", "v1.31", "v1.30"), "1.29–1.32", "1.30–1.33", "1.29", "1.33", true, false, true},
		{"non-contiguous list", c("", "", "1.28", "1.30"), c("", "", "1.30"), "1.28, 1.30", "1.30", "1.28", "", true, false, true},
		{"open-ended floor raised", c(">= 1.22.0-0", ">= 1.22.0-0"), c(">= 1.25.0-0", ">= 1.25.0-0"), "≥ 1.22", "≥ 1.25", "1.22–1.24", "", true, false, true},
		{"open becomes bounded", c(">= 1.25.0-0", ">= 1.25.0-0"), c(">=1.25.0-0 <=1.33.x", "1.25 → 1.33"), "≥ 1.25", "1.25–1.33", "> 1.33", "", true, false, true},
		{"unchanged", c(">=1.29.0-0 <=1.33.x", "1.29 → 1.33"), c(">=1.29.0-0 <1.34.0-0", "1.29 - 1.33"), "1.29–1.33", "1.29–1.33", "", "", false, true, true},
		{"raw only", c("", "TBD"), c("", "see docs"), "TBD", "see docs", "", "", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := compareConstraints(tt.f, tt.t)
			if r.fromDisp != tt.fromDisp || r.toDisp != tt.toDisp || r.drops != tt.drops || r.adds != tt.adds ||
				r.narrowed != tt.narrowed || r.unchanged != tt.unchanged || r.computable != tt.cmp {
				t.Errorf("got %+v", r)
			}
		})
	}
}

func TestCompatibilityChanges(t *testing.T) {
	e := mustBuild(t, cmInput(t))
	if len(e.Compatibility) != 2 || e.Compatibility[0].Platform != "kubernetes" || e.Compatibility[1].Platform != "openshift" {
		t.Fatalf("compatibility: %+v", e.Compatibility)
	}
	k := e.Compatibility[0]
	if k.Summary != "Kubernetes (supported): 1.24–1.29 → 1.25–1.30; drops 1.24, adds 1.30" || !k.Narrowed || k.From == nil || k.To == nil {
		t.Errorf("summary: %+v", k)
	}
	c := findTitle(e, "Kubernetes support narrowed")
	if c == nil || !c.ActionRequired || c.Category != domain.CategoryCompatibility || len(c.Evidence) != 2 ||
		c.Detail != "Verify the cluster runs a supported Kubernetes version (1.25–1.30) before upgrading." || len(c.Facts) != 2 {
		t.Errorf("narrowed change: %+v", c)
	}

	from := bare("v1.0.0").compat("kubernetes", "supported", ">=1.29.0-0 <=1.32.x", "1.29 → 1.32", "https://c").
		compat("kubernetes", "minimum", "", "TBD", "https://c").
		compat("helm", "supported", ">= 3.8.0", ">= 3.8", "https://c")
	to := bare("v1.1.0").compat("kubernetes", "supported", ">=1.29.0-0 <=1.33.x", "1.29 → 1.33", "https://c").
		compat("kubernetes", "minimum", "", "1.29 (see docs)", "https://c").
		compat("kubernetes", "tested", "", "1.31, 1.32", "https://c", "1.31", "1.32").
		compat("openshift", "supported", ">=4.16.0-0 <=4.18.x", "4.16 → 4.18", "https://c")
	e = mustBuild(t, simpleInput(from, to))
	var sums []string
	for _, cc := range e.Compatibility {
		sums = append(sums, cc.Summary)
	}
	want := []string{
		"Kubernetes (supported): 1.29–1.32 → 1.29–1.33; adds 1.33",
		"Kubernetes (minimum): TBD → 1.29 (see docs)",
		"Kubernetes (tested): v1.1.0 requires 1.31–1.32 (not stated for v1.0.0)",
		"OpenShift (supported): v1.1.0 requires 4.16–4.18 (not stated for v1.0.0)",
		"Helm (supported): ≥ 3.8 → not stated for v1.1.0",
	}
	if !reflect.DeepEqual(sums, want) {
		t.Errorf("summaries:\n%s\nwant:\n%s", strings.Join(sums, "\n"), strings.Join(want, "\n"))
	}
	if c := findTitle(e, "Kubernetes support extended"); c == nil || c.ActionRequired {
		t.Errorf("widening must not require action: %+v", c)
	}
	if c := findTitle(e, "Kubernetes minimum version changed: TBD → 1.29 (see docs)"); c == nil || !c.ActionRequired || c.Provenance.Confidence != domain.ConfidenceMedium {
		t.Errorf("raw change: %+v", c)
	}
	if c := findTitle(e, "v1.1.0 requires OpenShift 4.16–4.18 (supported)"); c == nil || !c.ActionRequired || c.Provenance.Rule != RuleCompatTargetOnly {
		t.Errorf("target-only requirement: %+v", c)
	}
	if c := findTitle(e, "v1.1.0 is tested on Kubernetes 1.31–1.32"); c == nil || c.ActionRequired {
		t.Errorf("target-only tested: %+v", c)
	}
	if !hasWarning(e, "No Helm (supported) compatibility data for target v1.1.0") {
		t.Errorf("from-only warning: %v", e.Warnings)
	}
}

// Tested lists say what the project's CI covers, not what it supports: a
// narrowed (or uncomparable) tested list is shown but never requires action,
// while the same change of the supported list does.
func TestTestedNarrowingIsInformational(t *testing.T) {
	from := bare("v1.0.0").
		compat("kubernetes", "tested", "", "1.29, 1.30, 1.31", "https://c", "1.29", "1.30", "1.31").
		compat("kubernetes", "supported", ">=1.29.0-0 <=1.31.x", "1.29 → 1.31", "https://c")
	to := bare("v1.1.0").
		compat("kubernetes", "tested", "", "1.30, 1.31, 1.32", "https://c", "1.30", "1.31", "1.32").
		compat("kubernetes", "supported", ">=1.30.0-0 <=1.32.x", "1.30 → 1.32", "https://c")
	e := mustBuild(t, simpleInput(from, to))

	tested := findTitle(e, "Kubernetes tested versions narrowed")
	if tested == nil {
		t.Fatalf("narrowed tested list must stay visible as a change: %+v", e.Changes)
	}
	if tested.ActionRequired || tested.Breaking || tested.Category != domain.CategoryCompatibility || tested.Provenance.Rule != RuleCompatNarrowed {
		t.Errorf("tested narrowing must not require action: %+v", tested)
	}
	if want := "Informational: the project tests this release on Kubernetes 1.30–1.32; other versions may work but are not covered by its testing."; tested.Detail != want {
		t.Errorf("detail = %q, want %q", tested.Detail, want)
	}
	if strings.Contains(tested.Detail, "Verify") {
		t.Errorf("tested hint must not ask for verification: %q", tested.Detail)
	}
	if sup := findTitle(e, "Kubernetes support narrowed"); sup == nil || !sup.ActionRequired {
		t.Errorf("supported narrowing is action required: %+v", sup)
	}
	var narrowed bool
	for _, cc := range e.Compatibility {
		if cc.To != nil && cc.To.Kind == "tested" {
			narrowed = cc.Narrowed
		}
	}
	if !narrowed {
		t.Error("the compatibility summary still records that the tested list narrowed")
	}

	// Uncomparable tested lists are informational as well.
	from = bare("v1.0.0").compat("kubernetes", "tested", "", "see matrix", "https://c")
	to = bare("v1.1.0").compat("kubernetes", "tested", "", "see the new matrix", "https://c")
	e = mustBuild(t, simpleInput(from, to))
	if c := findTitle(e, "Kubernetes tested versions changed"); c == nil || c.ActionRequired {
		t.Errorf("uncomparable tested change: %+v", c)
	}
}

// Chart kubeVersion is a hard install guard of the chart (kind
// "chart-kubeVersion"), separate from the "minimum" support statement.
func TestChartKubeVersionConstraints(t *testing.T) {
	const kv = "chart-kubeVersion"
	// Narrowing the guard can break the Helm upgrade: action required, and
	// labelled as a chart kubeVersion rather than the support matrix.
	from := bare("v1.0.0").compat("kubernetes", kv, ">=1.22.0-0", ">= 1.22.0-0", "https://chart")
	to := bare("v1.1.0").compat("kubernetes", kv, ">=1.25.0-0", ">= 1.25.0-0", "https://chart")
	e := mustBuild(t, simpleInput(from, to))
	c := findTitle(e, "Kubernetes chart kubeVersion narrowed: ≥ 1.22 → ≥ 1.25")
	if c == nil || !c.ActionRequired || !strings.Contains(c.Detail, "Helm refuses to install the chart") || strings.Contains(c.Detail, "supported") {
		t.Fatalf("chart kubeVersion narrowing: %+v", c)
	}
	out := string(render(t, e, RenderOptions{}))
	if !strings.Contains(out, "  chart kubeVersion: ≥ 1.22 → ≥ 1.25") || strings.Contains(out, "chart-kubeVersion:") {
		t.Errorf("renderer must label the kind clearly:\n%s", out)
	}

	// Absent on the target (an optional chart missing for this release, or no
	// kubeVersion declared): no warning about missing data, no change.
	e = mustBuild(t, simpleInput(
		bare("v1.0.0").compat("kubernetes", kv, ">=1.22.0-0", ">= 1.22.0-0", "https://chart"),
		bare("v1.1.0")))
	for _, w := range e.Warnings {
		if strings.Contains(w, "chart-kubeVersion") || strings.Contains(w, "compatibility data") {
			t.Errorf("misleading warning for an absent chart kubeVersion: %q", w)
		}
	}
	if len(e.Compatibility) != 1 || !strings.Contains(e.Compatibility[0].Summary, "not stated for v1.1.0") {
		t.Errorf("the summary still says it is not stated: %+v", e.Compatibility)
	}
	if findChanges(e, RuleCompatNarrowed) != nil || findChanges(e, RuleCompatChangedRaw) != nil {
		t.Errorf("no change expected when the target has no chart kubeVersion: %+v", e.Changes)
	}

	// Present only on the target (new chart, or a newly declared guard):
	// informational.
	e = mustBuild(t, simpleInput(
		bare("v1.0.0"),
		bare("v1.1.0").compat("kubernetes", kv, ">=1.25.0-0", ">= 1.25.0-0", "https://chart")))
	c = findTitle(e, "v1.1.0 chart requires Kubernetes ≥ 1.25 (chart kubeVersion)")
	if c == nil || c.ActionRequired || c.Provenance.Rule != RuleCompatTargetOnly {
		t.Errorf("target-only chart kubeVersion: %+v", c)
	}
	if hasWarning(e, "compatibility data") {
		t.Errorf("unexpected warning: %v", e.Warnings)
	}

	// The same kind in the support matrix still warns when the target has no data.
	e = mustBuild(t, simpleInput(
		bare("v1.0.0").compat("kubernetes", "supported", ">=1.29.0-0 <=1.32.x", "1.29 → 1.32", "https://c"),
		bare("v1.1.0")))
	if !hasWarning(e, "No Kubernetes (supported) compatibility data for target v1.1.0") {
		t.Errorf("supported must keep warning: %v", e.Warnings)
	}
}

// ---------------------------------------------------------------- advisories

func TestAdvisories(t *testing.T) {
	e := mustBuild(t, cmInput(t))
	fixed := findTitle(e, "Fixes GHSA-synt-heti-c001 (CVE-2099-0001): Webhook may log Secret data")
	if fixed == nil || fixed.Category != domain.CategorySecurity || fixed.ActionRequired || fixed.Release != "1.15.0" ||
		fixed.Provenance.Rule != RuleAdvisoryFixed || len(fixed.References) != 2 || fixed.References[0].Type != "ghsa" || fixed.References[1].Type != "cve" {
		t.Errorf("fixed advisory: %+v", fixed)
	}
	still := findTitle(e, "GHSA-r4pg-vg54-wxx4 (CVE-2024-12401) still affects target v1.15.1")
	if still == nil || !still.ActionRequired || still.Provenance.Rule != RuleAdvisoryAffected {
		t.Errorf("still affected: %+v", still)
	}
	if !hasWarning(e, "Target v1.15.1 is affected by GHSA-r4pg-vg54-wxx4") {
		t.Errorf("still-affected warning: %v", e.Warnings)
	}
	if !hasWarning(e, "GHSA-synt-heti-c002 has no machine-readable affected-version range") {
		t.Errorf("empty-range warning: %v", e.Warnings)
	}
	if findTitle(e, "c003") != nil || hasWarning(e, "c003") {
		t.Error("advisory affecting neither endpoint must be ignored")
	}
	// advisory evidence and facts are part of the edge, and linked
	for _, c := range []*domain.Change{fixed, still} {
		if c == nil {
			continue
		}
		if len(c.Facts) != 1 {
			t.Errorf("%s: facts %v", c.ID, c.Facts)
		}
	}
	advFacts := 0
	for _, f := range e.Facts {
		if f.Kind == domain.FactAdvisory {
			advFacts++
		}
	}
	if advFacts != 2 {
		t.Errorf("advisory facts = %d, want 2", advFacts)
	}
	for _, x := range e.Evidence {
		if strings.Contains(x.URI, "c002") {
			t.Error("evidence of an advisory that was not matched should not be pulled in")
		}
	}

	// unparsable range, missing evidence, affected only in To
	from, to := bare("v1.0.0"), bare("v1.1.0")
	in := simpleInput(from, to)
	ev := advisoryEvidence("GHSA-a", "https://adv/a", "a")
	in.Advisories = []domain.Advisory{
		{ID: "GHSA-aaaa-bbbb-cccc", Summary: "bad range", Vulnerable: "<= one-dot-two", Evidence: []domain.EvidenceID{ev.ID}},
		{ID: "GHSA-dddd-eeee-ffff", Summary: "no evidence", Vulnerable: "< 1.0.1"},
		{ID: "GHSA-gggg-hhhh-iiii", Summary: "regression", Vulnerable: ">= 1.1.0, < 1.1.2", Patched: []string{"1.1.2"}, Evidence: []domain.EvidenceID{ev.ID}},
	}
	in.AdvisoryEvidence = []domain.Evidence{ev}
	e = mustBuild(t, in)
	if !hasWarning(e, `unparsable affected-version range "<= one-dot-two"`) {
		t.Errorf("unparsable warning: %v", e.Warnings)
	}
	if findTitle(e, "no evidence") != nil || !hasWarning(e, "GHSA-dddd-eeee-ffff is fixed by this upgrade, but no evidence record") {
		t.Errorf("no-evidence advisory: %v", e.Warnings)
	}
	if c := findTitle(e, "GHSA-gggg-hhhh-iiii affects target v1.1.0: regression"); c == nil || !c.ActionRequired {
		t.Errorf("newly affected target: %+v", c)
	}
}

// ---------------------------------------------------------------- sources & gaps

func TestSourcesAndGapWarnings(t *testing.T) {
	from := newRel("demo", "v1.0.0").source("notes", "repo-file", domain.SourceOK, "", domain.RoleReleaseNotes).
		source("compat", "http", domain.SourceUnavailable, "blocked", domain.RoleCompatibility)
	mid := newRel("demo", "v1.1.0").
		source("notes", "repo-file", domain.SourceUnavailable, "HTTP 503", domain.RoleReleaseNotes).
		source("upgrade", "repo-file", domain.SourceNotFound, "404", domain.RoleUpgradeGuide)
	to := newRel("demo", "v1.1.1")
	in := simpleInput(from, to, mid)
	dup := domain.SourceStatus{SourceID: "tags", Kind: "git-tags", Roles: []domain.SourceRole{domain.RoleVersions}, State: domain.SourceOK}
	in.ExtraSources = []domain.SourceStatus{dup, dup, {SourceID: "adv", Kind: "github-advisories", Roles: []domain.SourceRole{domain.RoleSecurity}, State: domain.SourceError, Detail: "rate limited"}}
	e := mustBuild(t, in)
	var ids []string
	for _, s := range e.Sources {
		ids = append(ids, s.SourceID+"@"+s.Version)
	}
	if !reflect.DeepEqual(ids, []string{"tags@", "adv@", "notes@1.0.0", "compat@1.0.0", "notes@1.1.0", "upgrade@1.1.0"}) {
		t.Errorf("sources = %v", ids)
	}
	for _, w := range []string{
		"Release notes for v1.1.0 were not retrieved (notes: unavailable — HTTP 503)",
		"No release-notes source was consulted for v1.1.1",
		"Upgrade guide for v1.1.0 was not retrieved (upgrade: not-found — 404)",
		"Compatibility data for v1.0.0 was not retrieved (compat: unavailable — blocked)",
		"Security advisories were not fully retrieved (adv: error — rate limited)",
	} {
		if !hasWarning(e, w) {
			t.Errorf("missing warning %q in %v", w, e.Warnings)
		}
	}
}

// Alternatives of one role (fallback-group members such as a docs page on
// master and on an archive tag) cover each other: the gap warning is raised
// only when no source of the role answered.
func TestGapWarningsAlternativeSourcesCoverEachOther(t *testing.T) {
	from := newRel("demo", "v1.0.0").
		source("compat-primary", "repo-file", domain.SourceNotFound, "404", domain.RoleCompatibility).
		source("compat-archive", "repo-file", domain.SourceOK, "", domain.RoleCompatibility)
	mid := newRel("demo", "v1.1.0").
		source("guide-primary", "repo-file", domain.SourceNotFound, "404", domain.RoleUpgradeGuide).
		source("guide-archive", "repo-file", domain.SourceOK, "", domain.RoleUpgradeGuide)
	to := newRel("demo", "v1.2.0").
		source("guide-primary", "repo-file", domain.SourceNotFound, "404", domain.RoleUpgradeGuide).
		source("guide-archive", "repo-file", domain.SourceUnavailable, "blocked", domain.RoleUpgradeGuide)
	e := mustBuild(t, simpleInput(from, to, mid))
	for _, w := range []string{"Upgrade guide for v1.1.0", "Compatibility data for v1.0.0"} {
		if hasWarning(e, w) {
			t.Errorf("unexpected warning %q although an alternative source answered: %v", w, e.Warnings)
		}
	}
	for _, w := range []string{
		"Upgrade guide for v1.2.0 was not retrieved (guide-primary: not-found — 404)",
		"Upgrade guide for v1.2.0 was not retrieved (guide-archive: unavailable — blocked)",
	} {
		if !hasWarning(e, w) {
			t.Errorf("missing warning %q in %v", w, e.Warnings)
		}
	}
}

func TestFactsAreLinkedAndResolvable(t *testing.T) {
	in := cmInput(t)
	// a fact whose evidence is not part of any release is dropped with a warning
	in.To.Facts = append(in.To.Facts, domain.NewFact(domain.FactSnapshot, "x", "1.15.1", "dangling", "test", nil, "ev-missing"))
	e := mustBuild(t, in)
	if !hasWarning(e, "1 fact was dropped") {
		t.Errorf("dangling fact warning: %v", e.Warnings)
	}
	facts := map[domain.FactID]domain.Fact{}
	for _, f := range e.Facts {
		facts[f.ID] = f
	}
	linked := 0
	for _, c := range e.Changes {
		for _, id := range c.Facts {
			f, ok := facts[id]
			if !ok {
				t.Fatalf("change %s references unknown fact %s", c.ID, id)
			}
			shared := false
			for _, fe := range f.Evidence {
				for _, ce := range c.Evidence {
					shared = shared || fe == ce
				}
			}
			if !shared {
				t.Errorf("fact %s linked to %s without shared evidence", id, c.ID)
			}
			linked++
		}
	}
	if linked == 0 {
		t.Error("expected facts linked to changes")
	}
}
