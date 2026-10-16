package renderdiff

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// queryBundle renders two complete targets into a bundle.
func queryBundle(t *testing.T) (*fixture, *domain.EvidenceBundle) {
	t.Helper()
	f := newFixture(t)
	f.targets = []render.Target{
		helmTarget("argocd:"+f.repo+"/app.yaml#0", render.TargetArgoCD, f.repo+"/values.yaml", "replicaCount: 2\n"),
		helmTarget("values:"+f.repo+"/standalone.yaml", render.TargetValuesFiles, f.repo+"/standalone.yaml", "replicaCount: 1\n"),
	}
	b := f.bundle(t, renderOK(t, f))
	must(t, b.Validate())
	return f, b
}

func TestQueryFilters(t *testing.T) {
	_, b := queryBundle(t)
	all, err := QueryPage(b, investigate.DiffQuery{})
	must(t, err)
	if all.Total != 20 || len(all.Changes) != 20 || all.Next != "" {
		t.Fatalf("all: total %d, page %d, next %q", all.Total, len(all.Changes), all.Next)
	}
	ci := domain.ChangeItemID("chg-a", "", "")
	cases := []struct {
		name string
		q    investigate.DiffQuery
		want int
		each func(domain.RenderedChange) bool
	}{
		{"target", investigate.DiffQuery{Target: "argocd:app.yaml#0"}, 10, nil},
		{"change item", investigate.DiffQuery{ChangeItem: ci}, 2, func(c domain.RenderedChange) bool { return contains(c.ChangeItems, ci) }},
		{"uncorrelated (every change anchors an item now)", investigate.DiffQuery{Uncorrelated: true}, 0, nil},
		{"classes", investigate.DiffQuery{Classes: []string{"image-changed", "resource-added"}}, 4, nil},
		{"kinds (case-insensitive)", investigate.DiffQuery{Kinds: []string{"clusterrole"}}, 4, nil},
		{"cluster-scoped namespace", investigate.DiffQuery{Namespaces: []string{"-"}}, 4, nil},
		{"namespace", investigate.DiffQuery{Namespaces: []string{"acme"}}, 16, nil},
		{"path prefix", investigate.DiffQuery{PathPrefix: "spec.template.spec.containers[name=controller].args"}, 6, nil},
		{"combined", investigate.DiffQuery{Target: "values:standalone.yaml", Kinds: []string{"Deployment"}, Classes: []string{"container-arg-added", "image-changed"}}, 3, nil},
		{"release scope (empty)", investigate.DiffQuery{Scope: domain.RenderRelease}, 0, nil},
	}
	for _, c := range cases {
		p, err := QueryPage(b, c.q)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if p.Total != c.want || len(p.Changes) != c.want {
			t.Errorf("%s: total %d page %d, want %d", c.name, p.Total, len(p.Changes), c.want)
		}
		for _, rc := range p.Changes {
			if c.each != nil && !c.each(rc) {
				t.Errorf("%s: %s does not match", c.name, rc.ID)
			}
		}
	}
	// Query (DESIGN signature) is the first page
	got, err := Query(b, nil, investigate.DiffQuery{Target: "argocd:app.yaml#0"})
	if err != nil || len(got) != 10 {
		t.Errorf("Query: %d %v", len(got), err)
	}
}

func TestQueryOrderAndPagination(t *testing.T) {
	_, b := queryBundle(t)
	all, err := QueryPage(b, investigate.DiffQuery{Limit: MaxLimit})
	must(t, err)
	// ordered by target, then object, path, class: the first target's
	// changes come first, each target's sorted by (object, path)
	first := map[string]bool{}
	for _, c := range findPair(b.Renders, "argocd:app.yaml#0").Changes {
		first[c.ID] = true
	}
	for i, c := range all.Changes {
		if first[c.ID] != (i < 10) {
			t.Fatalf("change %d (%s) out of target order", i, c.ID)
		}
	}
	for _, part := range [][]domain.RenderedChange{all.Changes[:10], all.Changes[10:]} {
		if !sort.SliceIsSorted(part, func(i, j int) bool {
			return part[i].Object < part[j].Object || part[i].Object == part[j].Object && part[i].Path < part[j].Path
		}) {
			t.Errorf("not ordered by object, path")
		}
	}
	var paged []domain.RenderedChange
	q := investigate.DiffQuery{Limit: 3}
	for pages := 0; ; pages++ {
		p, err := QueryPage(b, q)
		must(t, err)
		if p.Total != 20 || len(p.Changes) > 3 {
			t.Fatalf("page: total %d size %d", p.Total, len(p.Changes))
		}
		paged = append(paged, p.Changes...)
		if p.Next == "" {
			if pages != 6 {
				t.Errorf("pages = %d, want 7", pages+1)
			}
			break
		}
		q.After = p.Next
	}
	if len(paged) != len(all.Changes) {
		t.Fatalf("paged %d, all %d", len(paged), len(all.Changes))
	}
	for i := range paged {
		if paged[i].ID != all.Changes[i].ID {
			t.Fatalf("page order differs at %d", i)
		}
	}
	if p, _ := QueryPage(b, investigate.DiffQuery{Limit: 1000}); len(p.Changes) != 20 {
		t.Errorf("limit above max not clamped")
	}
	if p, _ := QueryPage(b, investigate.DiffQuery{}); len(p.Changes) != 20 {
		t.Errorf("default limit")
	}
}

func TestQueryRejectsWhatTheBundleDoesNotHave(t *testing.T) {
	_, b := queryBundle(t)
	for name, q := range map[string]investigate.DiffQuery{
		"unknown target": {Target: "argocd:nope.yaml"},
		"unknown item":   {ChangeItem: "ci-000000000000"},
		"unknown class":  {Classes: []string{"image-swapped"}},
		"unknown scope":  {Scope: "cluster"},
		"bad cursor":     {After: "rc-000000000000"},
		"negative limit": {Limit: -1},
		"cursor of another query": {Target: "values:standalone.yaml", After: func() string {
			p, _ := QueryPage(b, investigate.DiffQuery{Target: "argocd:app.yaml#0", Limit: 1})
			return p.Next
		}()},
	} {
		if _, err := QueryPage(b, q); !errors.Is(err, investigate.ErrRejected) {
			t.Errorf("%s: err = %v, want ErrRejected", name, err)
		}
	}
}

func TestObject(t *testing.T) {
	f, b := queryBundle(t)
	ctx := context.Background()
	get := func(side, obj, path string) *investigate.ObjectResult {
		t.Helper()
		r, err := Object(ctx, b, f.st, investigate.ObjectRequest{Target: "argocd:app.yaml#0", Side: side, Object: obj, Path: path})
		if err != nil {
			t.Fatalf("%s %s %s: %v", side, obj, path, err)
		}
		return r
	}
	const dep = "apps/v1/Deployment/acme/acme"
	whole := get("to", dep, "")
	if !whole.Found || !strings.Contains(whole.YAML, "image: example.io/acme/controller:2.0.0") || whole.Artifact.Kind != domain.ArtifactRenderedManifests {
		t.Errorf("whole object: %+v", whole)
	}
	if r := get("from", dep, "spec.template.spec.containers[name=controller].image"); r.YAML != "example.io/acme/controller:1.0.0\n" {
		t.Errorf("from image = %q", r.YAML)
	}
	if r := get("to", dep, "spec.template.spec.containers[name=controller].args[=--log-level]"); r.YAML != "- --log-level=debug\n" {
		t.Errorf("member selector = %q", r.YAML)
	}
	if r := get("to", dep, "spec.template.spec.containers[0].ports[containerPort=8080/TCP]"); !r.Found || !strings.Contains(r.YAML, "containerPort: 8080") {
		t.Errorf("index + port selector = %+v", r)
	}
	if r := get("to", "core/v1/Service/acme/acme", `metadata.annotations["example.io/team"]`); r.YAML != "core\n" {
		t.Errorf("quoted key = %q", r.YAML)
	}
	if r := get("to", "core/v1/Secret/acme/acme-token", ""); strings.Contains(r.YAML, "s3cr3t") || !strings.Contains(r.YAML, sensitiveMarker) {
		t.Errorf("secret not redacted: %s", r.YAML)
	}
	if r := get("from", "core/v1/ConfigMap/acme/acme-config", ""); r.Found || r.YAML != "" {
		t.Errorf("an object absent on that side must be Found=false: %+v", r)
	}
	if r := get("to", dep, "spec.nope"); r.Found {
		t.Errorf("absent path found")
	}
	for name, req := range map[string]investigate.ObjectRequest{
		"unknown target": {Target: "x", Side: "to", Object: dep},
		"bad side":       {Target: "argocd:app.yaml#0", Side: "after", Object: dep},
		"bad path":       {Target: "argocd:app.yaml#0", Side: "to", Object: dep, Path: "spec[name=x"},
	} {
		if _, err := Object(ctx, b, f.st, req); !errors.Is(err, investigate.ErrRejected) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestObjectOfAnIncompletePairSide(t *testing.T) {
	f := newFixture(t)
	f.targets = []render.Target{helmTarget("values:"+f.repo+"/reject.yaml", render.TargetValuesFiles, f.repo+"/reject.yaml", "reject: true\n")}
	b := f.bundle(t, renderOK(t, f))
	_, err := Object(context.Background(), b, f.st, investigate.ObjectRequest{Target: "values:reject.yaml", Side: "to", Object: "apps/v1/Deployment/acme/acme"})
	if !errors.Is(err, investigate.ErrNotFound) || !strings.Contains(err.Error(), "invalid-values") {
		t.Errorf("err = %v", err)
	}
	r, err := Object(context.Background(), b, f.st, investigate.ObjectRequest{Target: "values:reject.yaml", Side: "from", Object: "apps/v1/Deployment/acme/acme"})
	if err != nil || !r.Found {
		t.Errorf("the source render of a rejecting target is readable: %+v %v", r, err)
	}
}

func TestObjectTooLargeIsRejectedWithKeys(t *testing.T) {
	f, b := queryBundle(t)
	big := strings.Repeat("x", MaxObjectBytes)
	objs := []render.Object{{ID: render.ObjectID{Group: "", Version: "v1", Kind: "ConfigMap", Namespace: "acme", Name: "big"}, Body: map[string]any{"data": map[string]any{"blob": big}}}}
	stream, _ := EncodeObjects(objs)
	ref, err := f.st.Put(context.Background(), investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/big/to.yaml",
		Origin: domain.OriginDerived, MediaType: "application/yaml", URI: "ri:test", Producer: Producer}, stream)
	must(t, err)
	b.Manifest.Artifacts = append(b.Manifest.Artifacts, ref)
	b.Renders[0].To = ref.ID
	_, err = Object(context.Background(), b, f.st, investigate.ObjectRequest{Target: b.Renders[0].Target, Side: "to", Object: "core/v1/ConfigMap/acme/big"})
	if !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), "data") {
		t.Errorf("err = %v", err)
	}
	r, err := Object(context.Background(), b, f.st, investigate.ObjectRequest{Target: b.Renders[0].Target, Side: "to", Object: "core/v1/ConfigMap/acme/big", Path: "metadata"})
	if err != nil || r.Found {
		t.Errorf("a small field of a big object: %+v %v", r, err)
	}
}

func TestSummaries(t *testing.T) {
	f := newFixture(t)
	f.targets = []render.Target{
		helmTarget("argocd:"+f.repo+"/app.yaml#0", render.TargetArgoCD, f.repo+"/values.yaml", "replicaCount: 2\n"),
		helmTarget("values:"+f.repo+"/reject.yaml", render.TargetValuesFiles, f.repo+"/reject.yaml", "reject: true\n"),
	}
	b := f.bundle(t, renderOK(t, f))
	s := Summaries(b)
	if len(s) != 2 {
		t.Fatalf("summaries = %+v", s)
	}
	if s[0].Target != "argocd:app.yaml#0" || s[0].Status != domain.CollectionOK || s[0].Changes != 10 || s[0].Uncorrelated != 0 || s[0].ByClass["container-arg-added"] != 2 {
		t.Errorf("ok summary = %+v", s[0])
	}
	if s[1].Status != domain.CollectionPartial || s[1].Changes != 0 || s[1].Reason == "" {
		t.Errorf("partial summary = %+v", s[1])
	}
}

func TestParsePath(t *testing.T) {
	segs, err := parsePath(`a.b["x.y/z"][0].c[name=n=1][=--f][#2].d`)
	must(t, err)
	want := []pathSeg{{key: "a"}, {key: "b"}, {key: "x.y/z"}, {index: 0, kind: segIndex}, {key: "c"},
		{field: "name", value: "n=1", kind: segKeyed}, {member: "--f", kind: segMember}, {key: "d"}}
	if len(segs) != len(want) {
		t.Fatalf("segs = %+v", segs)
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Errorf("seg %d = %+v, want %+v", i, segs[i], want[i])
		}
	}
	for _, bad := range []string{`a[`, `a["x]`, `a[x]`, `a[-1]`} {
		if _, err := parsePath(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
