package validate

import (
	"encoding/json"
	"slices"
	"strings"
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

// testRequest is an agent validation request of the given kind naming
// subjects (AI provenance; the shape the agent tool builds).
func testRequest(b *domain.EvidenceBundle, item string, kind domain.ValidationKind, subjects []string) *domain.ValidationRequest {
	at := testNow
	r := domain.ValidationRequest{ChangeItem: item, Kind: kind, Subjects: subjects, Status: domain.RequestOpen,
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "test.agent@v1", Confidence: domain.ConfidenceLow, Model: "m", ModelVersion: "m",
			PromptVersion: "p/v1", PromptDigest: domain.Digest([]byte("p")), InputEvidence: []domain.EvidenceID{b.Evidence[0].ID}, GeneratedAt: &at, CallID: "call-1"}}
	r.ID = domain.AgentRecordID(domain.ValidationRequestIDPrefix, item, string(kind)+"\x00"+strings.Join(subjects, ","), "call-1")
	return &r
}

// Every undecided record (not-run, error) echoes the coverage it was asked
// about EXACTLY as requested — subjects, paths and rendered changes — with
// its reason: the request stays linkable to its record and the audit keeps
// what was asked (it used to drop the requested rendered changes, and the
// join's no-report record dropped everything).
func TestUndecidedRecordsEchoTheRequest(t *testing.T) {
	f := newFx(t)
	item := f.change(domain.Change{ID: "chg-echo", Category: domain.CategoryConfiguration, Title: "mode defaults to fast",
		Subjects: []string{"--mode"}, Provenance: domain.Provenance{Method: domain.MethodDeclared, Rule: "marker:breaking"}},
		behaviour, notBreaking)
	f.file("apps/a.yaml", widget("w1", "apps", "Never"))
	ids := f.pair("argocd:apps/gizmo.yaml", controller(), controller("--mode=fast"),
		rc("container-arg-added", deployKey, "spec.template.spec.containers[name=controller].args", "", `"--mode=fast"`, "subject", item),
		rc("container-arg-added", deployKey, "spec.template.spec.containers[name=controller].args[#2]", "", `"--other"`, "subject", item))
	b := f.bundle()
	g := newGraph(t, b)
	dep := g.node(domain.NodeWorkload, deployKey, false)
	svc := g.node(domain.NodeService, "core/v1/Service/gizmo/gizmo", false)
	other := g.node(domain.NodeWorkload, "apps/v1/Deployment/gizmo/other", false)
	g.edge(domain.EdgeSelects, dep, svc, domain.EdgeEstablished)
	g.root(item, dep)
	g.root(item, other)
	p1 := g.path(item, dep, svc)
	g.path(item, other)
	graph, tr := g.done()
	subjects, paths, rcs := []string{dep}, []string{p1}, []string{ids[0]}
	undecided := 0
	for _, v := range allValidators(nil) {
		in := investigate.ValidationInput{Bundle: b, Graph: graph, Traversal: tr, ChangeItem: item,
			Subjects: subjects, Paths: paths, RenderedChanges: rcs, Request: testRequest(b, item, v.Kind(), subjects)}
		for _, r := range run(t, v, in).Records { // run → conform: ValidateAgainst checks the echo of the request's subjects
			if r.Outcome != domain.ValidationNotRun && r.Outcome != domain.ValidationError {
				continue
			}
			undecided++
			if !slices.Equal(r.Subjects, subjects) || !slices.Equal(r.Paths, paths) || !slices.Equal(r.RenderedChanges, rcs) {
				t.Errorf("%s (%s): coverage subjects %v paths %v rcs %v, want exactly the request %v %v %v",
					v.Name(), r.Provenance.Rule, r.Subjects, r.Paths, r.RenderedChanges, subjects, paths, rcs)
			}
			if r.Detail == "" || r.Request != in.Request.ID {
				t.Errorf("%s: an undecided record states its reason and names its request: %+v", v.Name(), r)
			}
		}
	}
	if undecided < 4 {
		t.Fatalf("fixture: only %d undecided records; every validator should be unable to decide here", undecided)
	}
}

// A decisive record is not widened: it speaks only for what it decided.
func TestEchoRequestedLeavesDecisiveRecords(t *testing.T) {
	for _, o := range []domain.ValidationOutcomeKind{domain.ValidationPass, domain.ValidationFail} {
		r := domain.ValidationRecord{Outcome: o, Subjects: []string{"gn-a"}}
		r.EchoRequested([]string{"gn-b"}, []string{"ip-1"}, []string{"rc-1"})
		if !slices.Equal(r.Subjects, []string{"gn-a"}) || r.Paths != nil || r.RenderedChanges != nil {
			t.Errorf("%s record widened: %+v", o, r)
		}
	}
	r := domain.ValidationRecord{Outcome: domain.ValidationError, Subjects: []string{"gn-b", "gn-a"}}
	r.EchoRequested([]string{"gn-b", "gn-c"}, nil, []string{"rc-1"})
	if !slices.Equal(r.Subjects, []string{"gn-a", "gn-b", "gn-c"}) || !slices.Equal(r.RenderedChanges, []string{"rc-1"}) {
		t.Errorf("error record = %+v", r)
	}
}
