package upgrade

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

const lcBanner = "https://raw.githubusercontent.com/example/demo/main/README.md"

func lcDefinition(l ...catalog.Lifecycle) *catalog.ProductDefinition {
	return &catalog.ProductDefinition{
		APIVersion: catalog.APIVersion, Kind: catalog.Kind, ID: "demo", Name: "demo",
		Versioning: catalog.Versioning{Scheme: domain.SchemeSemver, TagPrefix: "v", Lineage: catalog.LineageLinear},
		Sources: []catalog.Source{
			{ID: "notes", Roles: []domain.SourceRole{domain.RoleReleaseNotes}},
			{ID: "banner", Roles: []domain.SourceRole{domain.RoleReleaseNotes}},
		},
		Lifecycle: l,
	}
}

// lcRelease has an ordinary note and, when banner is set, the retirement
// banner bullet (a note item of the lifecycle source).
func lcRelease(tag string, banner bool) *rel {
	b := bare(tag).note("notes", domain.RoleReleaseNotes, "https://example.test/notes/"+tag, "Changes", "Fix a crash on reload", domain.CategoryBugfix)
	if banner {
		b.note("banner", domain.RoleReleaseNotes, lcBanner, "Retiring", "There will be no further releases or bug fixes", domain.CategoryOther)
	}
	return b
}

func TestLifecycleEndOfLifeTarget(t *testing.T) {
	def := lcDefinition(catalog.Lifecycle{
		State: catalog.LifecycleEndOfLife, Since: "2026-03",
		Summary: "demo is retired. Nothing will be fixed anymore.", Sources: []string{"banner"},
	})
	from, mid, to := lcRelease("v1.0.0", true), lcRelease("v1.1.0", true), lcRelease("v1.2.0", true)
	in := simpleInput(from, to, mid)
	in.Definition = def
	e := mustBuild(t, in)

	cs := findChanges(e, RuleLifecycleEndOfLife)
	if len(cs) != 1 {
		t.Fatalf("want one end-of-life change, got %d: %+v", len(cs), e.Changes)
	}
	c := cs[0]
	if c.Category != domain.CategoryDeprecation || !c.ActionRequired || c.Breaking {
		t.Errorf("category/flags: %+v", c)
	}
	if c.Title != "demo v1.2.0 is end-of-life since 2026-03" {
		t.Errorf("title: %q", c.Title)
	}
	if !strings.Contains(c.Detail, "demo is retired") || !strings.Contains(c.Detail, "plan the migration") {
		t.Errorf("detail: %q", c.Detail)
	}
	if c.Provenance.Method != domain.MethodDeclared || c.Provenance.Rule != RuleLifecycleEndOfLife {
		t.Errorf("provenance: %+v", c.Provenance)
	}
	if len(c.Evidence) != 1 {
		t.Fatalf("evidence: %v", c.Evidence)
	}
	found := false
	for _, ev := range e.Evidence {
		if ev.ID == c.Evidence[0] && ev.URI == lcBanner && strings.Contains(ev.Excerpt, "no further releases") {
			found = true
		}
	}
	if !found {
		t.Error("the change must cite the banner evidence, resolvable in the edge")
	}
	if !hasWarning(e, "Target v1.2.0 is end-of-life since 2026-03: demo is retired.") {
		t.Errorf("warnings: %v", e.Warnings)
	}
	// the banner is not an ordinary per-release note, however many releases carry it
	for _, ch := range e.Changes {
		if strings.Contains(ch.Title, "no further releases") {
			t.Errorf("banner reported as a release note: %q", ch.Title)
		}
	}
	if findTitle(e, "Fix a crash on reload") == nil {
		t.Error("ordinary notes must still be reported")
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleVersionRange(t *testing.T) {
	def := lcDefinition(catalog.Lifecycle{
		State: catalog.LifecycleEndOfLife, Versions: "< 1.2.0",
		Summary: "Lines before 1.2 are no longer maintained.", Sources: []string{"banner"},
	})
	// leaving an end-of-life line: informational, not action required
	from, to := lcRelease("v1.0.0", false), lcRelease("v1.2.0", true)
	in := simpleInput(from, to)
	in.Definition = def
	e := mustBuild(t, in)
	cs := findChanges(e, RuleLifecycleLeavesEOL)
	if len(cs) != 1 {
		t.Fatalf("leaves-end-of-life change: %+v", e.Changes)
	}
	if cs[0].ActionRequired || cs[0].Title != "demo v1.0.0 is end-of-life; the upgrade target v1.2.0 is not" {
		t.Errorf("change: %+v", cs[0])
	}
	if len(findChanges(e, RuleLifecycleEndOfLife)) != 0 || hasWarning(e, "end-of-life") {
		t.Errorf("target is not affected, nothing may be warned: %v", e.Warnings)
	}

	// neither endpoint affected: silent
	from, to = lcRelease("v1.2.0", true), lcRelease("v1.3.0", true)
	in = simpleInput(from, to)
	in.Definition = def
	e = mustBuild(t, in)
	for _, c := range e.Changes {
		if strings.HasPrefix(c.Provenance.Rule, "lifecycle:") {
			t.Errorf("unexpected lifecycle change %+v", c)
		}
	}
}

func TestLifecycleDeprecatedIsInformational(t *testing.T) {
	def := lcDefinition(catalog.Lifecycle{
		State: catalog.LifecycleDeprecated, Summary: "Use the successor.", Sources: []string{"banner"},
	})
	in := simpleInput(lcRelease("v1.0.0", true), lcRelease("v1.1.0", true))
	in.Definition = def
	e := mustBuild(t, in)
	cs := findChanges(e, RuleLifecycleDeprecated)
	if len(cs) != 1 || cs[0].ActionRequired || cs[0].Title != "demo v1.1.0 is deprecated" {
		t.Fatalf("changes: %+v", e.Changes)
	}
}

// A statement without retrievable evidence is not reported as a change: it is
// a warning, like any other conclusion without evidence.
func TestLifecycleWithoutEvidence(t *testing.T) {
	def := lcDefinition(catalog.Lifecycle{
		State: catalog.LifecycleEndOfLife, Summary: "demo is retired.", Sources: []string{"banner"},
	})
	in := simpleInput(lcRelease("v1.0.0", false), lcRelease("v1.1.0", false))
	in.Definition = def
	e := mustBuild(t, in)
	if len(findChanges(e, RuleLifecycleEndOfLife)) != 0 {
		t.Fatalf("must not report without evidence: %+v", e.Changes)
	}
	if !hasWarning(e, "no evidence is available: demo v1.1.0 is end-of-life") {
		t.Errorf("warnings: %v", e.Warnings)
	}
}

func TestNoLifecycleKeepsBannerNotes(t *testing.T) {
	in := simpleInput(lcRelease("v1.0.0", false), lcRelease("v1.1.0", true))
	in.Definition = lcDefinition()
	e := mustBuild(t, in)
	if findTitle(e, "no further releases") == nil {
		t.Error("without a lifecycle statement the banner is an ordinary note")
	}
}
