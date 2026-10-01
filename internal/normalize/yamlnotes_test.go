package normalize

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

func yamlInput(name, content string) DocInput {
	in := testInput(content, domain.RoleReleaseNotes)
	in.URI = "https://github.com/istio/istio/blob/1.31.1/releasenotes/notes/" + name
	in.Repository = "istio/istio"
	return in
}

func TestParseReleaseNoteYAMLFixtures(t *testing.T) {
	var files []DocInput
	for _, n := range []string{"61020", "backendtlspolicy-sidecar-failclosed", "sni-dnat-default", "spawn-upstream-span-for-gateway"} {
		files = append(files, yamlInput(n+".yaml", string(fixture(t, "istio/releasenotes/"+n+".yaml"))))
	}
	items, evs, err := ParseReleaseNoteYAML(files, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 4 releaseNotes + 2 upgradeNotes (one of them under the typo key "upgradeNodes")
	if len(items) != 6 || len(evs) != 6 {
		t.Fatalf("items=%d evidence=%d: %q", len(items), len(evs), texts(items))
	}
	byPrefix := func(p string) domain.NoteItem {
		t.Helper()
		for _, it := range items {
			if strings.HasPrefix(it.Text, p) {
				return it
			}
		}
		t.Fatalf("no item starting %q in %q", p, texts(items))
		return domain.NoteItem{}
	}

	fixed := byPrefix("Fixed an issue in ambient mode")
	checkClass(t, fixed, classWant{cat: domain.CategoryBugfix, method: domain.MethodDeclared, conf: domain.ConfidenceHigh, ruleHas: []string{"label:**Fixed**"}})
	if fixed.Section != "traffic-management" || fixed.Role != domain.RoleReleaseNotes || fixed.Release != "1.2.3" {
		t.Errorf("metadata: %+v", fixed)
	}
	if len(fixed.References) != 1 || fixed.References[0].ID != "istio/istio#61020" || fixed.References[0].Type != "issue" {
		t.Errorf("issue reference: %+v", fixed.References)
	}

	// kind security-fix beats the "Fixed" verb; the GHSA id is a reference
	sec := byPrefix("Fixed GHSA-qm8v-g4f9-qhjx")
	checkClass(t, sec, classWant{cat: domain.CategorySecurity, method: domain.MethodDeclared, conf: domain.ConfidenceHigh, ruleHas: []string{"kind:security-fix"}})
	if len(sec.References) != 1 || sec.References[0].Type != "ghsa" {
		t.Errorf("references = %+v", sec.References)
	}

	updated := byPrefix("Updated the default installation")
	if updated.Category != domain.CategoryOther {
		t.Errorf("Updated => other, got %s", updated.Category)
	}
	improved := byPrefix("Improved environment variable")
	if improved.Category != domain.CategoryFeature {
		t.Errorf("Improved => feature, got %s", improved.Category)
	}

	// upgradeNotes => migration + ActionRequired, declared, text is "title: content" without code
	up := byPrefix("`AUTO_PASSTHROUGH` Gateway mode: Previously")
	checkClass(t, up, classWant{cat: domain.CategoryMigration, action: true, method: domain.MethodDeclared, conf: domain.ConfidenceHigh, ruleHas: []string{"yaml:upgradeNotes"}})
	if strings.Contains(up.Text, "ISTIO_META_ROUTER_MODE\n") || strings.Contains(up.Text, "{{<") {
		t.Errorf("shortcode block leaked: %q", up.Text)
	}
	if len(up.Text) > maxItemText+4 {
		t.Errorf("not bounded: %d", len(up.Text))
	}
	// the typo key is tolerated
	typo := byPrefix("enable PILOT_SPAWN_UPSTREAM_SPAN_FOR_GATEWAY by default: ")
	if typo.Category != domain.CategoryMigration || !typo.ActionRequired {
		t.Errorf("upgradeNodes typo: %+v", typo)
	}

	// evidence: per entry, URI of the file, locator = file name + lines of the entry
	byID := evidenceByID(evs)
	e := byID[fixed.Evidence[0]]
	if e.URI != files[0].URI || e.Locator != "61020.yaml L7-L10" || e.Kind != domain.EvidenceDocument || e.ContentDigest != "sha256:abc" {
		t.Errorf("evidence = %+v", e)
	}
	if !strings.Contains(e.Excerpt, "**Fixed** an issue in ambient mode") {
		t.Errorf("excerpt = %q", e.Excerpt)
	}
	e = byID[up.Evidence[0]]
	lines := strings.Split(string(fixture(t, "istio/releasenotes/sni-dnat-default.yaml")), "\n")
	if e.Locator != "sni-dnat-default.yaml L11-L36" || !strings.Contains(lines[10], "title:") {
		t.Errorf("upgrade note locator %q (line 11 = %q)", e.Locator, lines[10])
	}
}

func TestParseReleaseNoteYAMLCases(t *testing.T) {
	t.Run("kind test is skipped", func(t *testing.T) {
		items, evs, err := ParseReleaseNoteYAML([]DocInput{yamlInput("t.yaml", "apiVersion: release-notes/v2\nkind: test\narea: istioctl\nreleaseNotes:\n- \"**Added** a test\"\n")}, nil)
		if err != nil || len(items) != 0 || len(evs) != 0 {
			t.Errorf("%v %v %v", items, evs, err)
		}
	})
	t.Run("kinds without verb", func(t *testing.T) {
		for kind, want := range map[string]domain.Category{
			"feature":       domain.CategoryFeature,
			"bug-fix":       domain.CategoryBugfix,
			"bug":           domain.CategoryBugfix,
			"promotion":     domain.CategoryFeature,
			"enhancement":   domain.CategoryFeature,
			"security-fix":  domain.CategorySecurity,
			"documentation": domain.CategoryOther,
		} {
			items, _, err := ParseReleaseNoteYAML([]DocInput{yamlInput("k.yaml", "kind: "+kind+"\narea: x\nreleaseNotes:\n- plain text without verb\n")}, nil)
			if err != nil || len(items) != 1 {
				t.Fatalf("%s: %v %v", kind, items, err)
			}
			if items[0].Category != want || items[0].Classification.Method != domain.MethodDeclared || !strings.Contains(items[0].Classification.Rule, "kind:"+kind) {
				t.Errorf("kind %s => %s (%s, %s)", kind, items[0].Category, items[0].Classification.Method, items[0].Classification.Rule)
			}
		}
	})
	t.Run("verb beats kind", func(t *testing.T) {
		items, _, _ := ParseReleaseNoteYAML([]DocInput{yamlInput("v.yaml", "kind: feature\nreleaseNotes:\n- \"**Removed** the flag\"\n")}, nil)
		if len(items) != 1 || items[0].Category != domain.CategoryRemoval || !items[0].ActionRequired {
			t.Errorf("%+v", items)
		}
	})
	t.Run("unknown and misspelled keys are tolerated", func(t *testing.T) {
		in := "apiVersion: release-notes/v2\nkind: feature\narea: telemetry\nIssues: [12, \"https://github.com/istio/istio/issues/99\"]\nreleaseNote:\n- \"**Added** x\"\nsecurityNote:\n- \"A security note\"\nupgradeNote:\n- title: T\n  content: C\nweird: {a: b}\ndocs:\n- \"[usage] https://istio.io/foo\"\n"
		items, _, err := ParseReleaseNoteYAML([]DocInput{yamlInput("m.yaml", in)}, nil)
		if err != nil || len(items) != 3 {
			t.Fatalf("%v %v", texts(items), err)
		}
		if items[0].Category != domain.CategoryFeature || items[1].Category != domain.CategorySecurity || items[2].Category != domain.CategoryMigration {
			t.Errorf("categories: %s %s %s", items[0].Category, items[1].Category, items[2].Category)
		}
		if got := items[0].References; len(got) != 2 || got[0].ID != "istio/istio#12" || got[1].ID != "istio/istio#99" {
			t.Errorf("issue references: %+v", got)
		}
	})
	t.Run("securityNotes are security", func(t *testing.T) {
		items, _, _ := ParseReleaseNoteYAML([]DocInput{yamlInput("s.yaml", "kind: feature\narea: security\nsecurityNotes:\n- text a\n- text b\n")}, nil)
		if len(items) != 2 || items[0].Category != domain.CategorySecurity || items[1].Category != domain.CategorySecurity {
			t.Errorf("%+v", items)
		}
		if items[0].Classification.Method != domain.MethodDeclared || !strings.Contains(items[0].Classification.Rule, "yaml:securityNotes") {
			t.Errorf("%+v", items[0].Classification)
		}
	})
	t.Run("product rules apply first", func(t *testing.T) {
		rules := []catalog.ClassifyRule{{Section: "^documentation$", Skip: true}, {Text: "special", Category: domain.CategoryConfiguration}}
		items, _, _ := ParseReleaseNoteYAML([]DocInput{
			yamlInput("a.yaml", "kind: feature\narea: documentation\nreleaseNotes:\n- skip me\n"),
			yamlInput("b.yaml", "kind: feature\narea: x\nreleaseNotes:\n- \"**Added** a special thing\"\n"),
		}, rules)
		if len(items) != 1 || items[0].Category != domain.CategoryConfiguration || !strings.Contains(items[0].Classification.Rule, "product:1") {
			t.Errorf("%+v", items)
		}
	})
	t.Run("invalid file is reported, others are kept", func(t *testing.T) {
		items, _, err := ParseReleaseNoteYAML([]DocInput{
			yamlInput("bad.yaml", "kind: [unclosed\n"),
			yamlInput("good.yaml", "kind: bug-fix\nreleaseNotes:\n- \"**Fixed** z\"\n"),
			yamlInput("empty.yaml", "  \n"),
			yamlInput("list.yaml", "- a\n- b\n"),
		}, nil)
		if len(items) != 1 {
			t.Errorf("items = %v", texts(items))
		}
		if err == nil || !strings.Contains(err.Error(), "bad.yaml") || !strings.Contains(err.Error(), "list.yaml") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("duplicate text across files merges", func(t *testing.T) {
		items, evs, _ := ParseReleaseNoteYAML([]DocInput{
			yamlInput("a.yaml", "kind: bug-fix\nreleaseNotes:\n- \"**Fixed** same\"\n"),
			yamlInput("b.yaml", "kind: bug-fix\nreleaseNotes:\n- \"**Fixed** same\"\n"),
		}, nil)
		if len(items) != 1 || len(items[0].Evidence) != 2 || len(evs) != 2 {
			t.Errorf("%d items, evidence %v", len(items), items[0].Evidence)
		}
	})
	t.Run("line offset and file name", func(t *testing.T) {
		in := yamlInput("o.yaml", "kind: bug-fix\nreleaseNotes:\n- \"**Fixed** z\"\n")
		in.LineOffset = 10
		_, evs, _ := ParseReleaseNoteYAML([]DocInput{in}, nil)
		if len(evs) != 1 || evs[0].Locator != "o.yaml L13-L13" {
			t.Errorf("%+v", evs)
		}
	})
	t.Run("invalid rule", func(t *testing.T) {
		if _, _, err := ParseReleaseNoteYAML(nil, []catalog.ClassifyRule{{Text: "("}}); err == nil {
			t.Error("expected error")
		}
	})
}

func TestCanonicalNoteKey(t *testing.T) {
	for in, want := range map[string]string{
		"kind": "kind", "area": "area", "issue": "issue", "issues": "issue", "Issue": "issue",
		"releaseNotes": "releaseNotes", "releaseNote": "releaseNotes", "releasenotes": "releaseNotes",
		"upgradeNotes": "upgradeNotes", "upgradeNodes": "upgradeNotes", "upgradeNote": "upgradeNotes",
		"securityNotes": "securityNotes", "securityNote": "securityNotes",
		"docs": "", "apiVersion": "", "release": "", "notes": "",
	} {
		if got := canonicalNoteKey(in); got != want {
			t.Errorf("canonicalNoteKey(%q) = %q, want %q", in, got, want)
		}
	}
}
