package renderdiff

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

func TestCompletePairIndexesCorrelatedChanges(t *testing.T) {
	f := newFixture(t)
	f.targets = []render.Target{helmTarget("argocd:"+f.repo+"/apps/acme.yaml#0", render.TargetArgoCD, f.repo+"/values.yaml", "replicaCount: 2\n")}
	res := renderOK(t, f)
	if len(res.Pairs) != 1 {
		t.Fatalf("pairs = %d", len(res.Pairs))
	}
	p := res.Pairs[0]
	if p.Target != "argocd:apps/acme.yaml#0" {
		t.Errorf("target %q is not repo-relative", p.Target)
	}
	if p.Status != domain.CollectionOK || p.Reason != "" || p.From == "" || p.To == "" || p.Diff == "" {
		t.Fatalf("pair = %+v", p)
	}
	byKey := map[string]domain.RenderedChange{}
	for _, c := range p.Changes {
		byKey[c.Class+" "+c.Path] = c
		if c.ID != domain.RenderedChangeID(domain.RenderEnvironment, p.Target, c.Object, c.Path, c.Class) {
			t.Errorf("%s: id not derived", c.ID)
		}
		if len(c.Evidence) != 1 {
			t.Errorf("%s: evidence = %v", c.ID, c.Evidence)
		}
	}
	argPath := "spec.template.spec.containers[name=controller].args"
	rot, ok := byKey["container-arg-added "+argPath+"[=--default-private-key-rotation-policy]"]
	if !ok {
		t.Fatalf("rotation arg change missing; got %v", keys(byKey))
	}
	if want := []string{domain.ChangeItemID("chg-a", "", "")}; !reflect.DeepEqual(rot.ChangeItems, want) || rot.Correlation != "subject" {
		t.Errorf("rotation arg: items %v rule %q", rot.ChangeItems, rot.Correlation)
	}
	if rot.After != `"--default-private-key-rotation-policy=Always"` {
		t.Errorf("after = %q", rot.After)
	}
	// two arg changes on the same args list stay distinct (member selector)
	if _, ok := byKey["container-arg-added "+argPath+"[=--enable-foo]"]; !ok {
		t.Errorf("--enable-foo change missing")
	}
	// a link to a change without a change item (routine) does not correlate
	if c := byKey["container-arg-added "+argPath+"[=--enable-foo]"]; len(c.ChangeItems) != 0 {
		t.Errorf("routine-only link correlated: %v", c.ChangeItems)
	}
	if c := byKey["service-port-changed spec.ports[name=http]"]; !reflect.DeepEqual(c.ChangeItems, []string{domain.ChangeItemID("chg-b", "", "")}) {
		t.Errorf("service port items = %v", c.ChangeItems)
	}
	// an artifact-delta link names no change: uncorrelated without an anchored item
	img := byKey["image-changed spec.template.spec.containers[name=controller].image"]
	if img.ID == "" || len(img.ChangeItems) != 0 || img.Correlation != "" {
		t.Errorf("image change = %+v", img)
	}
	// RBAC permissions on one rules list are distinct changes
	if _, ok := byKey["rbac-permission-added rules[=apps/deployments:watch]"]; !ok {
		t.Errorf("rbac add missing: %v", keys(byKey))
	}
	if _, ok := byKey["rbac-permission-removed rules[=apps/deployments:list]"]; !ok {
		t.Errorf("rbac remove missing")
	}
	// sensitive: no values recorded, none stored
	sec, ok := byKey["field-changed stringData.token"]
	if !ok || !sec.Sensitive || sec.Before != "" || sec.After != "" {
		t.Errorf("secret change = %+v (ok %v)", sec, ok)
	}
	for _, a := range res.Artifacts {
		if s := f.st.text(t, a.Digest); strings.Contains(s, "s3cr3t") {
			t.Errorf("artifact %s stores a secret value", a.Path)
		}
		if a.Kind == domain.ArtifactRenderedManifests && !a.Redacted {
			t.Errorf("%s holds Secret data but is not marked redacted", a.Path)
		}
	}
	// the bundle built from it validates, every change resolves
	b := f.bundle(t, res)
	if err := b.Validate(); err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if un := b.Uncorrelated(); len(un) == 0 {
		t.Errorf("expected uncorrelated changes (image bump, ConfigMap)")
	}
	assertResolves(t, f.st, b)
	if len(res.Notes) == 0 || strings.Contains(strings.Join(res.Notes, "\n"), f.repo) {
		t.Errorf("notes not carried or not repo-relative: %v", res.Notes)
	}
}

// assertResolves checks that every indexed change resolves back to the
// artifact bytes: its evidence cites the diff artifact's digest with a
// locator naming its entry, the entry is in the stored diff, and its object
// (and path, when the object exists on that side) reads from the stored
// render with text that occurs in the bytes.
func assertResolves(t *testing.T, st *memStore, b *domain.EvidenceBundle) {
	t.Helper()
	ev := b.EvidenceByID()
	for _, p := range b.Renders {
		if p.Status != domain.CollectionOK {
			continue
		}
		diffRef, _ := b.Manifest.Lookup(p.Diff)
		var doc DiffDocument
		raw := st.text(t, diffRef.Digest)
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatal(err)
		}
		if !doc.Indexed || doc.Status != domain.CollectionOK {
			t.Errorf("%s: diff document not marked indexed", p.Target)
		}
		for _, c := range p.Changes {
			e := ev[c.Evidence[0]]
			if e.ContentDigest != diffRef.Digest || e.Locator != "changes[id="+c.ID+"]" || e.SourceID != diffRef.ID || e.Kind != domain.EvidenceRenderedDiff {
				t.Errorf("%s: evidence %+v does not cite the diff entry", c.ID, e)
			}
			if !strings.Contains(raw, `"id": "`+c.ID+`"`) {
				t.Errorf("%s: no diff entry", c.ID)
			}
			side := "to"
			if strings.HasSuffix(c.Class, "-removed") {
				side = "from"
			}
			obj, err := Object(context.Background(), b, st, investigate.ObjectRequest{Target: p.Target, Side: side, Object: c.Object, Path: c.Path})
			if err != nil {
				t.Errorf("%s: object %s %s: %v", c.ID, c.Object, c.Path, err)
				continue
			}
			if !obj.Found {
				if !strings.HasPrefix(c.Class, "resource-") && !strings.HasSuffix(c.Class, "-removed") && !strings.HasSuffix(c.Class, "-added") {
					t.Errorf("%s: %s %s not found on %s side", c.ID, c.Object, c.Path, side)
				}
				continue
			}
			if !investigate.QuoteOccurs(st.text(t, obj.Artifact.Digest), obj.YAML) {
				t.Errorf("%s: object text does not occur in the artifact:\n%s", c.ID, obj.YAML)
			}
		}
	}
}

func TestIncompletePairsCarryReasonsAndNoChanges(t *testing.T) {
	f := newFixture(t)
	incomplete := helmTarget("flux:"+f.repo+"/hr.yaml#0", render.TargetFlux, f.repo+"/hr.yaml#0/$.spec.values", "replicaCount: 2\n")
	incomplete.ValuesComplete, incomplete.IncompleteReason = false, "spec.valuesFrom[0] (Secret acme-values) is outside the repository"
	f.targets = []render.Target{
		incomplete,
		helmTarget("values:"+f.repo+"/uncovered.yaml", render.TargetValuesFiles, f.repo+"/uncovered.yaml", "replicaCount: 2\nnotAKey:\n  x: 1\nredis:\n  enabled: true\nglobal:\n  a: b\n"),
		helmTarget("values:"+f.repo+"/reject.yaml", render.TargetValuesFiles, f.repo+"/reject.yaml", "reject: true\nreplicaCount: 1\n"),
	}
	broken := helmTarget("helmfile:"+f.repo+"/helmfile.yaml#acme", render.TargetHelmfile, f.repo+"/helmfile.yaml", "")
	broken.ReleaseName = "broken"
	f.targets = append(f.targets, broken)
	res := renderOK(t, f)
	b := f.bundle(t, res)
	if err := b.Validate(); err != nil {
		t.Fatalf("bundle: %v", err)
	}
	cases := []struct {
		prefix string
		status domain.CollectionStatus
		reason string
		diff   bool
	}{
		{"flux:", domain.CollectionPartial, "values-incomplete: spec.valuesFrom[0]", true},
		{"values:uncovered", domain.CollectionPartial, "chart-coverage: values keys notAKey are defined by neither", true},
		{"values:reject", domain.CollectionPartial, "invalid-values: the target chart rejects the values", false},
		{"helmfile:", domain.CollectionUnavailable, "renderer-unavailable: helm is not installed", false},
	}
	for _, c := range cases {
		p := pairByPrefix(t, res, c.prefix)
		if p.Status != c.status || !strings.HasPrefix(p.Reason, c.reason) {
			t.Errorf("%s: %s %q, want %s %q…", p.Target, p.Status, p.Reason, c.status, c.reason)
		}
		if len(p.Changes) != 0 {
			t.Errorf("%s: an incomplete pair indexes %d changes", p.Target, len(p.Changes))
		}
		if (p.Diff != "") != c.diff {
			t.Errorf("%s: diff artifact %q", p.Target, p.Diff)
		}
		if p.Diff != "" {
			ref, _ := b.Manifest.Lookup(p.Diff)
			var doc DiffDocument
			must(t, json.Unmarshal([]byte(f.st.text(t, ref.Digest)), &doc))
			if doc.Indexed || doc.Status != c.status || len(doc.Changes) == 0 {
				t.Errorf("%s: diff document indexed=%v status=%s changes=%d", p.Target, doc.Indexed, doc.Status, len(doc.Changes))
			}
			if len(ref.ChangeItems) != 0 {
				t.Errorf("%s: an incomplete diff relates to change items %v", p.Target, ref.ChangeItems)
			}
		}
	}
	if p := pairByPrefix(t, res, "values:reject"); p.From == "" || p.To != "" {
		t.Errorf("rejecting target: from %q to %q (the source render is kept, the target has none)", p.From, p.To)
	}
	if len(b.Uncorrelated()) != 0 {
		t.Errorf("incomplete pairs yielded rendered changes: %v", b.Uncorrelated())
	}
	m := Measure(res.Pairs)
	if m.Targets != 4 || m.OK != 0 || m.Partial != 3 || m.Unavailable != 1 || m.Changes != 0 || m.Reasons["values-incomplete"] != 1 {
		t.Errorf("measure = %+v", m)
	}
}

func TestKustomizeOverlayWithoutProductVersion(t *testing.T) {
	f := newFixture(t)
	consumer := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: shop
  namespace: shop
spec:
  template:
    spec:
      containers:
        - name: web
          image: nginx:1.25
`
	unpinned := strings.Replace(consumer, "nginx:1.25", "example.io/acme/controller:1.0.0", 1)
	f.r.engine.Kustomize = &fakeKustomize{streams: map[string]string{"shop": consumer, "acme": unpinned}}
	for _, d := range []string{"shop", "acme"} {
		dir := filepath.Join(f.repo, d)
		must(t, os.MkdirAll(dir, 0o755))
		must(t, os.WriteFile(filepath.Join(dir, "kustomization.yaml"), []byte("resources:\n  - deploy.yaml\n"), 0o644))
		must(t, os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte(consumer), 0o644))
		f.targets = append(f.targets, render.Target{ID: "kustomize:" + dir, Kind: render.TargetKustomize, Origin: dir, Why: "test", ValuesComplete: true,
			Kustomize: &render.KustomizeInput{Dir: dir, Root: f.repo}})
	}
	res := renderOK(t, f)
	shop := pairByPrefix(t, res, "kustomize:shop")
	if shop.Status != domain.CollectionNotApplicable || !strings.HasPrefix(shop.Reason, "version-independent:") || shop.From == "" || shop.To != "" {
		t.Errorf("consumer overlay = %+v", shop)
	}
	if strings.Contains(shop.Reason, f.repo) {
		t.Errorf("reason not repo-relative: %s", shop.Reason)
	}
	acme := pairByPrefix(t, res, "kustomize:acme")
	if acme.Status != domain.CollectionUnavailable || !strings.Contains(acme.Reason, "example.io/acme/controller") {
		t.Errorf("unpinned product overlay = %+v", acme)
	}
	if err := f.bundle(t, res).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLayeredValuesFileIsNotRenderedAlone(t *testing.T) {
	f := newFixture(t)
	argo := helmTarget("argocd:"+f.repo+"/app.yaml#0", render.TargetArgoCD, f.repo+"/app.yaml#0", "")
	argo.Values = []render.ValuesLayer{{Origin: f.repo + "/values.yaml", Content: []byte("replicaCount: 2\n")}, {Origin: f.repo + "/values-prod.yaml", Content: []byte("replicaCount: 3\n")}}
	f.targets = []render.Target{
		argo,
		helmTarget("values:"+f.repo+"/values.yaml", render.TargetValuesFiles, f.repo+"/values.yaml", "replicaCount: 2\n"),
		helmTarget("values:"+f.repo+"/values-prod.yaml", render.TargetValuesFiles, f.repo+"/values-prod.yaml", "replicaCount: 3\n"),
		helmTarget("values:"+f.repo+"/standalone.yaml", render.TargetValuesFiles, f.repo+"/standalone.yaml", "replicaCount: 1\n"),
	}
	res := renderOK(t, f)
	if got := targetNames(res.Pairs); !reflect.DeepEqual(got, []string{"argocd:app.yaml#0", "values:standalone.yaml"}) {
		t.Errorf("targets = %v", got)
	}
	notes := strings.Join(res.Notes, "\n")
	if !strings.Contains(notes, "values file values-prod.yaml: rendered as a values layer of") {
		t.Errorf("notes = %s", notes)
	}
}

func TestReleaseAnchoredItemCorrelatesEnvironmentChange(t *testing.T) {
	f := newFixture(t)
	f.targets = []render.Target{helmTarget("argocd:"+f.repo+"/app.yaml#0", render.TargetArgoCD, f.repo+"/values.yaml", "replicaCount: 2\n")}
	// a release-scope pair whose image change no Change explains, and the
	// collector's item anchored on it
	rec := &f.change.Record
	const imgPath = "spec.template.spec.containers[name=controller].image"
	art := func(kind domain.ArtifactKind, path, body string) domain.ArtifactRef {
		ref, err := f.st.Put(context.Background(), investigate.ArtifactSpec{Kind: kind, Path: path, Origin: domain.OriginDerived,
			MediaType: "application/yaml", URI: "ri:test", Producer: "bundle.collect@v1"}, []byte(body))
		must(t, err)
		f.base.Manifest.Artifacts = append(f.base.Manifest.Artifacts, ref)
		return ref
	}
	from, to, diff := art(domain.ArtifactRenderedManifests, "derived/render/release/from.yaml", "a"), art(domain.ArtifactRenderedManifests, "derived/render/release/to.yaml", "b"),
		art(domain.ArtifactRenderDiff, "derived/render/release/diff.json", "{}")
	rc := domain.RenderedChange{Class: "image-changed", Object: "apps/v1/Deployment/acme/acme", Path: imgPath}
	rc.ID = domain.RenderedChangeID(domain.RenderRelease, "chart-defaults", rc.Object, rc.Path, rc.Class)
	rec.Renders = []domain.ManifestPair{{Scope: domain.RenderRelease, Target: "chart-defaults", From: from.ID, To: to.ID, Diff: diff.ID,
		Status: domain.CollectionOK, Changes: []domain.RenderedChange{rc}}}
	item := rec.Items[0]
	item.ChangeID, item.RenderedChange, item.Family, item.Title = "", rc.ID, domain.FamilyRendered, "controller image changed"
	item.ManifestationRule, item.Manifestation = "manifest:rendered-anchor", domain.ManifestRenderedConfig
	item.ID = domain.ChangeItemID("", "", rc.ID)
	rec.Items = append(rec.Items, item)
	rec.ID = rec.DerivedID()
	must(t, rec.Validate())
	res := renderOK(t, f)
	var got *domain.RenderedChange
	for i, c := range res.Pairs[0].Changes {
		if c.Class == "image-changed" {
			got = &res.Pairs[0].Changes[i]
		}
	}
	if got == nil || !reflect.DeepEqual(got.ChangeItems, []string{item.ID}) || got.Correlation != RuleReleaseAnchor {
		t.Fatalf("image change = %+v", got)
	}
	if err := f.bundle(t, res).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRenderDeterministic(t *testing.T) {
	run := func() (*investigate.RenderResult, string) {
		f := newFixture(t)
		f.targets = []render.Target{
			helmTarget("argocd:"+f.repo+"/app.yaml#0", render.TargetArgoCD, f.repo+"/values.yaml", "replicaCount: 2\n"),
			helmTarget("values:"+f.repo+"/standalone.yaml", render.TargetValuesFiles, f.repo+"/standalone.yaml", "replicaCount: 1\n"),
		}
		res := renderOK(t, f)
		return res, f.bundle(t, res).ID
	}
	a, ida := run()
	b, idb := run()
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("two runs differ (the repo lives in different temp dirs: ids must not depend on it)")
	}
	if ida != idb {
		t.Errorf("bundle ids differ: %s vs %s", ida, idb)
	}
}

func TestRenderDisabledAndErrors(t *testing.T) {
	f := newFixture(t)
	res, err := f.r.RenderEnvironment(context.Background(), f.change, f.snap, investigate.Config{Render: false})
	if err != nil || len(res.Pairs) != 0 || len(res.Notes) != 1 {
		t.Errorf("disabled: %+v %v", res, err)
	}
	f.targets = nil
	res = renderOK(t, f)
	if len(res.Pairs) != 0 || !strings.Contains(strings.Join(res.Notes, "\n"), "no deployment of acme was found") {
		t.Errorf("no targets: notes %v", res.Notes)
	}
	bad := *f.change
	bad.Artifacts = nil
	if _, err := f.r.RenderEnvironment(context.Background(), &bad, f.snap, investigate.Config{Render: true}); investigate.ClassOf(err) != investigate.Permanent {
		t.Errorf("missing edge artifact: err %v (class %s)", err, investigate.ClassOf(err))
	}
	if _, err := f.r.RenderEnvironment(context.Background(), nil, f.snap, investigate.Config{Render: true}); err == nil {
		t.Errorf("nil change accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.targets = []render.Target{helmTarget("values:x", render.TargetValuesFiles, "x", "replicaCount: 1\n")}
	if _, err := f.r.RenderEnvironment(ctx, f.change, f.snap, investigate.Config{Render: true}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
