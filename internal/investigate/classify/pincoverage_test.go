package classify

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Exact coverage (R2.2, mission E3, PLAN-M4 lane applicability change 4): a
// passing field pin covers exactly the rendered changes on its own field — a
// change of the pinned kind at the pinned path, and the description text of
// that field in the CRD serving the kind. The policy re-checks the record's
// claim against the bundle, so a validator that claims more covers nothing
// more.

const (
	widgetObj     = "gizmo.example.io/v1/Widget/apps/w-root"
	widgetCRDName = "widgets.gizmo.example.io"
	otherDesc     = "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.size.description"
	defaultPath   = "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.default"
	v2Desc        = "spec.versions[name=v2].schema.openAPIV3Schema.properties.spec.properties.mode.description"
)

func withFieldPinAt(ev domain.EvidenceID, subject, path string) vmod {
	return func(v *domain.ValidationRecord) {
		v.Pin = &domain.PinnedSetting{Kind: domain.PinField, Subject: subject, Path: path, Value: `"safe"`, Evidence: []domain.EvidenceID{ev}}
	}
}

func fieldPin(ev domain.EvidenceID, crd string) vmod {
	return func(v *domain.ValidationRecord) {
		v.Pin = &domain.PinnedSetting{Kind: domain.PinField, Subject: widgetObj, Path: "spec.mode", Value: `"safe"`, CRD: crd, Evidence: []domain.EvidenceID{ev}}
	}
}

// pinFx: a behaviour item with one established impact path, correlated
// environment rendered changes, and a field pin covering the path.
func pinFx(t *testing.T, changes ...domain.RenderedChange) (*fx, string, string, []string, *domain.TraversalResult) {
	f := newFx(t)
	item := f.item(behaviour)
	for i := range changes {
		changes[i].ChangeItems = []string{item}
		changes[i].Correlation = "subject"
	}
	ids := f.pair("argocd:apps/gizmo.yaml", changes...)
	tb := &trav{}
	tb.root(item, node(controllerObj))
	p := tb.path(item, domain.EdgeEstablished, node(controllerObj), node("apps/v1/Deployment/app/consumer"))
	return f, item, p, ids, tb.result()
}

func TestPinCoversExactlyItsField(t *testing.T) {
	cases := []struct {
		name   string
		change domain.RenderedChange
		crd    string
		covers bool
	}{
		{"the pinned field's description", rc(domain.RenderedClassCRDDescription, crdObj, descPath), widgetCRDName, true},
		{"a change of the pinned field on a pinned object", rc("field-changed", widgetObj, "spec.mode"), widgetCRDName, true},
		{"another field's description in the same CRD", rc(domain.RenderedClassCRDDescription, crdObj, otherDesc), widgetCRDName, false},
		{"a schema change on the pinned field", rc("field-changed", crdObj, schemaPath), widgetCRDName, false},
		{"a default change on the pinned field", rc("field-added", crdObj, defaultPath), widgetCRDName, false},
		{"another version's description", rc(domain.RenderedClassCRDDescription, crdObj, v2Desc), widgetCRDName, false},
		{"an unrelated resource", rc("field-changed", "apps/v1/Deployment/app/consumer", "spec.replicas"), widgetCRDName, false},
		{"another field of a pinned object", rc("field-changed", widgetObj, "spec.size"), widgetCRDName, false},
		{"the description, but the CRD serving the kind is unknown", rc(domain.RenderedClassCRDDescription, crdObj, descPath), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, item, p, ids, tr := pinFx(t, tc.change)
			// the record claims the change; the policy decides
			pin := f.vrec(item, domain.ValidationOverrideInspection, domain.ValidationPass, paths(p), rcs(ids...), fieldPin(f.envEvidence(), tc.crd))
			cls, _ := run(t, f.in(tr, nil, pin))
			got := finding(t, cls, item)
			if tc.covers {
				wantClass(t, got, domain.FindingNotAffected, RuleValidatedClear)
				return
			}
			if got.Class == domain.FindingNotAffected || !hasGap(got, domain.GapValidation, ids[0]) {
				t.Fatalf("%s must stay a gap: %s %+v", tc.change.Path, got.Class, got.Gaps)
			}
		})
	}
}

// A pin that claims every change of the item covers only its own field: the
// mutation set of the brief stays open beside the covered description.
func TestPinCoverageMutationsStayGaps(t *testing.T) {
	f, item, p, ids, tr := pinFx(t,
		rc(domain.RenderedClassCRDDescription, crdObj, descPath),
		rc(domain.RenderedClassCRDDescription, crdObj, otherDesc),
		rc("field-changed", crdObj, schemaPath),
		rc("field-changed", "apps/v1/Deployment/app/consumer", "spec.replicas"))
	pin := f.vrec(item, domain.ValidationOverrideInspection, domain.ValidationPass, paths(p), rcs(ids...), fieldPin(f.envEvidence(), widgetCRDName))
	cls, _ := run(t, f.in(tr, nil, pin))
	got := finding(t, cls, item)
	if got.Class == domain.FindingNotAffected {
		t.Fatalf("cleared: %s", got.Reasoning)
	}
	if hasGap(got, domain.GapValidation, ids[0]) {
		t.Fatalf("the pinned field's description must be covered: %+v", got.Gaps)
	}
	for _, id := range ids[1:] {
		if !hasGap(got, domain.GapValidation, id) {
			t.Fatalf("%s must stay a gap: %+v", id, got.Gaps)
		}
	}
	// a flag pin covers no rendered change at all
	flag := f.vrec(item, domain.ValidationOverrideInspection, domain.ValidationPass, paths(p), rcs(ids...), withPin(f.envEvidence()))
	cls, _ = run(t, f.in(tr, nil, flag))
	for _, id := range ids {
		if !hasGap(finding(t, cls, item), domain.GapValidation, id) {
			t.Fatalf("a flag pin covers %s", id)
		}
	}
}
