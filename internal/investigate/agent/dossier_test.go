package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func buildDossier(t *testing.T, f *widgetFixture, item string) *Dossier {
	t.Helper()
	d, err := BuildDossier(context.Background(), f.dossierInput(), item)
	must(t, err)
	return d
}

func objectStates(d *Dossier) map[string]DossierObject {
	out := map[string]DossierObject{}
	for _, o := range d.Exposed {
		out[o.Object] = o
	}
	return out
}

func hasGap(d *Dossier, kind, subject string) bool {
	for _, g := range d.Gaps {
		if g.Kind == kind && strings.Contains(g.Subject, subject) {
			return true
		}
	}
	return false
}

func TestDossierReproducible(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	a, err := buildDossier(t, f, f.item).Encode()
	must(t, err)
	b, err := buildDossier(t, f, f.item).Encode()
	must(t, err)
	if !bytes.Equal(a, b) {
		t.Fatal("two builds of the same dossier differ")
	}
	r1, err := StoreDossier(context.Background(), f.st, buildDossier(t, f, f.item))
	must(t, err)
	r2, err := StoreDossier(context.Background(), f.st, buildDossier(t, f, f.item))
	must(t, err)
	if r1.ID != r2.ID || r1.Kind != domain.ArtifactItemDossier {
		t.Fatalf("stored dossier is not content-addressed: %s vs %s (%s)", r1.ID, r2.ID, r1.Kind)
	}
	must(t, r1.Validate())
}

func TestDossierSettingStates(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	d := buildDossier(t, f, f.item)
	if d.Change.Release != "v1.5.0" || !strings.Contains(d.Change.Detail, "spec.cleanup.policy: Retain") || len(d.Change.Upstream) != 1 || d.Change.Upstream[0].Artifact != f.notes.ID {
		t.Fatalf("change section: %+v", d.Change)
	}
	if len(d.Settings) == 0 || d.Settings[0].Setting != "example.dev/Widget spec.cleanup.policy" || !strings.HasPrefix(d.Settings[0].From, "knowledge fact") {
		t.Fatalf("settings: the fact's subject comes first: %+v", d.Settings)
	}
	if d.Settings[0].State != SettingMixed {
		t.Fatalf("setting state: %+v", d.Settings[0])
	}
	st := objectStates(d)
	for _, id := range f.explicit {
		if o := st[id]; o.State != SettingExplicit || o.Value != `"Retain"` || !strings.Contains(o.View, "complete") {
			t.Errorf("%s: %+v, want explicit \"Retain\" from the complete render", id, o)
		}
	}
	for _, id := range f.unset {
		if o := st[id]; o.State != SettingInheritedDefault || !o.Root {
			t.Errorf("%s: %+v, want inherited-default (an exposure root)", id, o)
		}
	}
	if len(d.Unchanged) != len(f.explicit) {
		t.Errorf("unchanged: %+v, want the %d explicit settings identical before and after", d.Unchanged, len(f.explicit))
	}
	if len(d.RenderedChanges) != 1 || d.RenderedChanges[0].ID != f.rc {
		t.Errorf("rendered changes: %+v", d.RenderedChanges)
	}
	if len(d.Knowledge) != 1 || d.Knowledge[0].Fact != f.fact.ID || d.Knowledge[0].Level != string(domain.VerifiedConsensus) {
		t.Errorf("knowledge: %+v", d.Knowledge)
	}
	var fromText *DossierSetting
	for i := range d.Settings {
		if d.Settings[i].From == "upstream text" {
			fromText = &d.Settings[i]
		}
	}
	if fromText == nil || fromText.Setting != "key spec.cleanup.policy" || fromText.State != SettingExplicit || !strings.Contains(fromText.Detail, "not listed") {
		t.Errorf("the upstream text's code span names the setting: %+v", d.Settings)
	}
	if hasGap(d, GapMissing, "setting") || hasGap(d, GapContradictory, "") {
		t.Errorf("a complete view has no setting gap: %+v", d.Gaps)
	}
	// the values item: the key is set explicitly in the values file
	v := buildDossier(t, f, f.other)
	if len(v.Settings) != 1 || v.Settings[0].Kind != "values-key" || v.Settings[0].State != SettingExplicit || !strings.Contains(v.Settings[0].Where[0], "L2") {
		t.Fatalf("values setting: %+v", v.Settings)
	}
}

// An unset field seen only in a file as declared is unknown, never an
// inherited default: a patch elsewhere may set it (fail closed).
func TestDossierUnknownWithoutCompleteRender(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{renderIncomplete: true})
	d := buildDossier(t, f, f.item)
	st := objectStates(d)
	for _, id := range f.unset {
		if o := st[id]; o.State != SettingUnknown || o.View != "file apps/shop/widgets.yaml" {
			t.Errorf("%s: %+v, want unknown from the file", id, o)
		}
	}
	for _, id := range f.explicit {
		if o := st[id]; o.State != SettingExplicit {
			t.Errorf("%s: %+v, want explicit", id, o)
		}
	}
	if !hasGap(d, GapMissing, "setting example.dev/Widget") || !hasGap(d, GapPartial, "render argocd:apps/widgets.yaml") {
		t.Fatalf("gaps: %+v", d.Gaps)
	}
	if len(d.Unchanged) != 0 {
		t.Errorf("unchanged needs both renders: %+v", d.Unchanged)
	}
	v := buildDossier(t, f, f.other)
	if v.Settings[0].State != SettingExplicit {
		t.Errorf("an explicit value stays explicit without a render: %+v", v.Settings[0])
	}
}

func TestDossierContradictionsAndPartialEvidence(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{contradictFact: true, valuesConflict: true, uncorrelated: true})
	d := buildDossier(t, f, f.item)
	if !hasGap(d, GapContradictory, "knowledge crd-field:widgets") {
		t.Errorf("contradicting facts are not a gap: %+v", d.Gaps)
	}
	if !hasGap(d, GapPartial, "rendered changes") {
		t.Errorf("an uncorrelated rendered change is not a gap: %+v", d.Gaps)
	}
	v := buildDossier(t, f, f.other)
	if !hasGap(v, GapContradictory, "values controller.legacyMode") {
		t.Errorf("conflicting values are not a gap: %+v", v.Gaps)
	}
	if v.Gaps[0].Kind != GapContradictory {
		t.Errorf("contradictions are listed first: %+v", v.Gaps)
	}
}

func TestDossierBounded(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{widgets: 600})
	d := buildDossier(t, f, f.item)
	data, err := d.Encode()
	must(t, err)
	if len(data) > MaxDossierBytes {
		t.Fatalf("dossier is %d bytes (max %d)", len(data), MaxDossierBytes)
	}
	if len(d.Truncated) == 0 {
		t.Fatal("a truncated dossier must say what it cut")
	}
	if !strings.Contains(d.Settings[0].Detail, "603 Widget object(s)") {
		t.Errorf("the setting summary counts every object, listed or not: %s", d.Settings[0].Detail)
	}
}
