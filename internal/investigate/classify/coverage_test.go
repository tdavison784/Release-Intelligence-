package classify

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Documentation-only coverage (classify.policy@v2): a render-diff pass
// marked DocumentationOnly accounts for the CRD description changes it
// names (R2.2), for any manifestation — and for nothing else.

const (
	crdObj     = "apiextensions.k8s.io/v1/CustomResourceDefinition//widgets.gizmo.example.io"
	descPath   = "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.description"
	schemaPath = "spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.mode.type"
)

func docOnly(v *domain.ValidationRecord) { v.DocumentationOnly = true }

// docFx: a behaviour item whose only correlated environment rendered change
// is the description text of its field, an established impact path from the
// controller, and a pin that covers the path (an override inspection carries
// no rendered changes).
type docFx struct {
	f    *fx
	item string
	desc string
	path string
	tr   *domain.TraversalResult
	pin  domain.ValidationRecord
}

func newDocFx(t *testing.T, manifestation func(*domain.ChangeItem), extra ...domain.RenderedChange) *docFx {
	f := newFx(t)
	item := f.item(manifestation)
	changes := append([]domain.RenderedChange{rc(domain.RenderedClassCRDDescription, crdObj, descPath, item)}, extra...)
	ids := f.pair("argocd:apps/gizmo.yaml", changes...)
	tb := &trav{}
	tb.root(item, node(controllerObj))
	p := tb.path(item, domain.EdgeEstablished, node(controllerObj), node("apps/v1/Deployment/app/consumer"))
	d := &docFx{f: f, item: item, desc: ids[0], path: p, tr: tb.result()}
	d.pin = f.vrec(item, domain.ValidationOverrideInspection, domain.ValidationPass, paths(p), withPin(f.envEvidence()))
	return d
}

func (d *docFx) doc() domain.ValidationRecord {
	return d.f.vrec(d.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(d.desc), docOnly, validator("validate.render-diff@v2"))
}

// The before/after trace of the status file: without documentation-only
// coverage the pinned behaviour item stays open on R2.2 alone; with it, R2
// holds and the finding cites both the pin and the documentation record.
func TestDocumentationOnlyCoverageClosesR2_2(t *testing.T) {
	for _, m := range []func(*domain.ChangeItem){behaviour, unknownManifest} {
		d := newDocFx(t, m)

		before, _ := run(t, d.f.in(d.tr, nil, d.pin))
		fb := finding(t, before, d.item)
		wantClass(t, fb, domain.FindingPotentiallyAffected, RulePlausibleImpact)
		if len(fb.Gaps) != 1 || fb.Gaps[0].Kind != domain.GapValidation || fb.Gaps[0].Subject != d.desc {
			t.Fatalf("before: the only gap must be the unaccounted description change %s: %+v", d.desc, fb.Gaps)
		}

		doc := d.doc()
		after, _ := run(t, d.f.in(d.tr, nil, d.pin, doc))
		fa := finding(t, after, d.item)
		wantClass(t, fa, domain.FindingNotAffected, RuleValidatedClear)
		if strings.Join(fa.Validations, ",") != strings.Join(uniqSorted([]string{d.pin.ID, doc.ID}), ",") {
			t.Fatalf("after: the clear cites the pin and the documentation record: %v", fa.Validations)
		}
		if fa.Basis != domain.LevelDeterministic || fa.Provenance.Producer != "classify.policy@v2" {
			t.Fatalf("after: basis %s producer %s", fa.Basis, fa.Provenance.Producer)
		}
		if !strings.Contains(fa.Reasoning, "documentation-only CRD description changes accounted for by validate.render-diff@v2") {
			t.Fatalf("after: the reasoning must name the documentation-only coverage: %s", fa.Reasoning)
		}
		t.Logf("%s before: %s %s — %s", fa.Manifestation, fb.Class, fb.Rule, fb.Gaps[0].Detail)
		t.Logf("%s after:  %s %s — %s", fa.Manifestation, fa.Class, fa.Rule, fa.Reasoning)
	}
}

// No new path to not-affected without a clearing pass: documentation-only
// coverage never counts for R2.4, never stands in for a pin or an observed
// outcome, whatever else accompanies it.
func TestDocumentationOnlyNeverClears(t *testing.T) {
	for _, m := range []func(*domain.ChangeItem){behaviour, unknownManifest} {
		d := newDocFx(t, m)
		silent := d.f.vrec(d.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(d.desc), paths(d.path))
		join := d.f.vrec(d.item, domain.ValidationOverrideInspection, domain.ValidationPass, validator("validate.join@v1"), paths(d.path))
		for name, vals := range map[string][]domain.ValidationRecord{
			"documentation only":                {d.doc()},
			"documentation and render silence":  {d.doc(), silent},
			"documentation and a pinless pass":  {d.doc(), join},
			"documentation, silence and a join": {d.doc(), silent, join},
		} {
			cls, _ := run(t, d.f.in(d.tr, nil, vals...))
			f := finding(t, cls, d.item)
			if f.Class == domain.FindingNotAffected {
				t.Fatalf("%s: %s item cleared without a pin or an observed outcome", name, f.Manifestation)
			}
			if !hasGap(f, domain.GapValidation, d.path) || !hasGap(f, domain.GapValidation, d.item) {
				t.Fatalf("%s: the path and the item stay uncovered: %+v", name, f.Gaps)
			}
			if hasGap(f, domain.GapValidation, d.desc) {
				t.Fatalf("%s: the description change is accounted for: %+v", name, f.Gaps)
			}
		}
	}
	// A rendered-config item: documentation-only coverage is not "a passing
	// validation of the item" either.
	d := newDocFx(t, func(*domain.ChangeItem) {})
	d.tr = (&trav{}).result()
	cls, _ := run(t, d.f.in(d.tr, nil, d.doc()))
	if f := finding(t, cls, d.item); f.Class == domain.FindingNotAffected {
		t.Fatalf("a rendered-config item cleared by documentation-only coverage alone: %s", f.Reasoning)
	}
}

// The classifier re-checks the record's claim against the bundle: a
// documentation-only record over a description change whose CRD also changes
// its schema (correlated to nothing), or over a schema change, accounts for
// nothing — and such a record does not conform to the run record contract.
func TestDocumentationOnlyClaimIsRechecked(t *testing.T) {
	cases := map[string]func(d *docFx) []string{
		"description beside a schema change in the same CRD": func(d *docFx) []string { return []string{d.desc} },
		"the schema change itself": func(d *docFx) []string {
			for _, p := range d.f.pairs {
				for _, c := range p.Changes {
					if c.Path == schemaPath {
						return []string{d.desc, c.ID}
					}
				}
			}
			t.Fatal("no schema change in the fixture")
			return nil
		},
	}
	for name, covered := range cases {
		t.Run(name, func(t *testing.T) {
			d := newDocFx(t, behaviour, rc("field-changed", crdObj, schemaPath))
			doc := d.f.vrec(d.item, domain.ValidationRenderDiff, domain.ValidationPass, rcs(covered(d)...), docOnly, validator("validate.render-diff@v2"))
			in := d.f.in(d.tr, nil, d.pin, doc)
			c, err := New().Classify(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			f := finding(t, c, d.item)
			if f.Class == domain.FindingNotAffected || !hasGap(f, domain.GapValidation, d.desc) {
				t.Fatalf("a description change in a CRD that changes beyond description text was accounted for: %s %+v", f.Class, f.Gaps)
			}
			inv := record(t, in)
			if err := Apply(inv, c, testNow.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if err := inv.ValidateAgainst(in.Bundle); err == nil || !strings.Contains(err.Error(), "documentation-only coverage") {
				t.Fatalf("the run record must reject the false documentation-only claim, got %v", err)
			}
		})
	}
}

// Agent output is never an input: adding agent hypotheses and explanations
// to a cleared run changes neither the class nor what it rests on, and an
// unanswered agent concern still blocks — a documentation-only record never
// answers it.
func TestDocumentationOnlyAgentRecordsAreNoInput(t *testing.T) {
	d := newDocFx(t, behaviour)
	doc := d.doc()
	base, _ := run(t, d.f.in(d.tr, nil, d.pin, doc))
	explains := d.f.assertion(d.item, domain.StanceExplains, "the description change documents the new default")
	hyp := d.f.hypothesis(d.item, "the explicit setting keeps the old behaviour")
	withAgent, _ := run(t, d.f.in(d.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{explains}, hyp), d.pin, doc))
	fb, fa := finding(t, base, d.item), finding(t, withAgent, d.item)
	if fb.Class != fa.Class || fb.Rule != fa.Rule || strings.Join(fb.Validations, ",") != strings.Join(fa.Validations, ",") || fb.Basis != fa.Basis {
		t.Fatalf("agent records changed the decision: %s %v → %s %v", fb.Class, fb.Validations, fa.Class, fa.Validations)
	}
	concern := d.f.assertion(d.item, domain.StanceRaisesConcern, "the new default rotates keys the consumers pin")
	cls, _ := run(t, d.f.in(d.tr, agentOutcome(domain.AgentSubmitted, []domain.AgentAssertion{concern}), d.pin, doc))
	if f := finding(t, cls, d.item); f.Class == domain.FindingNotAffected || !hasGap(f, domain.GapAgent, concern.ID) {
		t.Fatalf("an unanswered concern must still block: %s %+v", f.Class, f.Gaps)
	}
}

// The shape of a documentation-only record is enforced on the record itself:
// it names rendered changes only, is a deterministic render-diff pass and
// carries no pin.
func TestDocumentationOnlyRecordShape(t *testing.T) {
	d := newDocFx(t, behaviour)
	bad := map[string]func(v *domain.ValidationRecord){
		"subjects":       func(v *domain.ValidationRecord) { v.Subjects = []string{node(controllerObj)} },
		"paths":          func(v *domain.ValidationRecord) { v.Paths = []string{d.path} },
		"answers":        func(v *domain.ValidationRecord) { v.Answers = []string{"hyp-x"} },
		"no changes":     func(v *domain.ValidationRecord) { v.RenderedChanges = nil },
		"override kind":  func(v *domain.ValidationRecord) { v.Kind = domain.ValidationOverrideInspection },
		"not a pass":     func(v *domain.ValidationRecord) { v.Outcome, v.Detail = domain.ValidationNotRun, "x" },
		"with a pin":     func(v *domain.ValidationRecord) { v.Pin = d.pin.Pin },
		"observed level": func(v *domain.ValidationRecord) { v.Kind, v.Level = domain.ValidationRuntime, domain.LevelObserved },
	}
	for name, mod := range bad {
		v := d.doc()
		mod(&v)
		v.ID = domain.ValidationRecordID(v.Validator, v.ChangeItem, v.Subjects, v.Paths, v.RenderedChanges, v.Inputs)
		if err := v.Validate(); err == nil || !strings.Contains(err.Error(), "documentation-only") {
			t.Errorf("%s: a malformed documentation-only record validated: %v", name, err)
		}
	}
	if v := d.doc(); v.ClearsBehaviour() {
		t.Fatal("a documentation-only record never clears behaviour")
	}
}

// A stored Phase 4 run asks for classify.policy@v1: it is re-classified
// byte-identically to v2 except for the version stamp (its validators never
// emitted documentation-only records), and a documentation-only record under
// v1 is refused rather than silently counted as an ordinary pass.
func TestPolicyV1StillReproduces(t *testing.T) {
	d := newDocFx(t, behaviour)
	in := d.f.in(d.tr, nil, d.pin)
	v2, _ := run(t, in)
	in.Config.Policy = PolicyV1
	v1, _ := run(t, in)
	if v1.Decision.Policy != PolicyV1 || finding(t, v1, d.item).Provenance.Producer != PolicyV1 {
		t.Fatalf("v1 run stamped %s / %s", v1.Decision.Policy, finding(t, v1, d.item).Provenance.Producer)
	}
	a, b := finding(t, v1, d.item), finding(t, v2, d.item)
	a.Provenance.Producer = b.Provenance.Producer
	if string(mustJSON(t, a)) != string(mustJSON(t, b)) {
		t.Fatalf("v1 and v2 decide differently on v1 input:\n%s\n%s", mustJSON(t, a), mustJSON(t, b))
	}
	in = d.f.in(d.tr, nil, d.pin, d.doc())
	in.Config.Policy = PolicyV1
	if _, err := New().Classify(context.Background(), in); err == nil || !strings.Contains(err.Error(), "needs classify.policy@v2") {
		t.Fatalf("a documentation-only record under v1 must be refused, got %v", err)
	}
	in.Config.Policy = "classify.policy@v0"
	if _, err := New().Classify(context.Background(), in); err == nil {
		t.Fatal("an unknown policy version must be refused")
	}
}
