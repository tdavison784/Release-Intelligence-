package validate

import (
	"encoding/json"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

var (
	_ investigate.Validator = JoinVerdicts(nil)
	_ investigate.Validator = RenderDiff()
	_ investigate.Validator = OverrideInspection()
	_ investigate.Validator = SchemaCompat()
	_ investigate.Validator = StaticReference()
)

func allValidators(st investigate.ArtifactStore) []investigate.Validator {
	opts := []Option{WithClock(clock)}
	if st != nil {
		opts = append(opts, WithStore(st))
	}
	return []investigate.Validator{JoinVerdicts(nil, opts...), RenderDiff(opts...), OverrideInspection(opts...), SchemaCompat(opts...), StaticReference(opts...)}
}

func TestValidatorIdentity(t *testing.T) {
	want := map[string]domain.ValidationKind{
		NameJoin: domain.ValidationOverrideInspection, NameRenderDiff: domain.ValidationRenderDiff,
		NameOverrideInspection: domain.ValidationOverrideInspection, NameSchemaCompat: domain.ValidationSchemaCompat,
		NameStaticReference: domain.ValidationStaticReference,
	}
	for _, v := range allValidators(nil) {
		if k, ok := want[v.Name()]; !ok || k != v.Kind() {
			t.Fatalf("%s: kind %s", v.Name(), v.Kind())
		}
	}
}

// With nothing to decide on — no report, no render, no store, no graph —
// no validator passes any item, of any family or manifestation.
func TestCannotDecideNeverPasses(t *testing.T) {
	for _, fam := range domain.ChangeItemFamilies {
		for _, m := range domain.Manifestations {
			for _, withStore := range []bool{false, true} {
				f := newFx(t)
				item := f.change(domain.Change{ID: "chg-" + string(fam), Category: domain.CategoryOther, Breaking: true, Title: "a change",
					Subjects: []string{"x"}, Provenance: domain.Provenance{Rule: "values:default-changed"}},
					func(it *domain.ChangeItem) {
						it.Family, it.Manifestation, it.ManifestationRule = fam, m, "manifest:test"
					})
				f.file("apps/a.yaml", widget("w1", "apps", "Never"))
				b := f.bundle()
				var st investigate.ArtifactStore
				if withStore {
					st = f.st
				}
				for _, v := range allValidators(st) {
					out := run(t, v, investigate.ValidationInput{Bundle: b, ChangeItem: item})
					for _, r := range out.Records {
						if r.Outcome == domain.ValidationPass || r.Outcome == domain.ValidationFail {
							t.Fatalf("%s on %s/%s (store %v): decisive %s without evidence to decide on: %s", v.Name(), fam, m, withStore, r.Outcome, r.Detail)
						}
						if r.Detail == "" {
							t.Fatalf("%s: a not-run record states its reason", v.Name())
						}
					}
				}
			}
		}
	}
}

// Same inputs ⇒ byte-identical output (ids are content-derived; the clock
// is the only time source).
func TestValidatorsAreDeterministic(t *testing.T) {
	o := newOverrideFx(t)
	o.withFact(flagDefaultFact("vf-flag", domain.VerifiedDeterministic, o.f.up))
	o.f.file("apps/a/widgets.yaml", stream(widget("w1", "apps", "Never"), widget("w2", "apps", "Never")))
	o.f.pair("argocd:apps/gizmo.yaml", controller("--mode=safe"), controller("--mode=safe"),
		rc("container-arg-changed", deployKey, "spec.template.spec.containers[name=controller].args", `"a"`, `"b"`, "mentions-name", o.item))
	b := o.f.bundle()
	g, tr, _, _ := withGraph(t, b, o.item)
	in := investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: o.item}
	for _, v := range allValidators(o.f.st) {
		a, b2 := run(t, v, in), run(t, v, in)
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b2)
		if string(ja) != string(jb) {
			t.Fatalf("%s is not deterministic:\n%s\n%s", v.Name(), ja, jb)
		}
	}
}
