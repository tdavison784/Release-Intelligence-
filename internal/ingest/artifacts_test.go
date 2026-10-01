package ingest

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

func TestIngestReleaseArtifacts(t *testing.T) {
	type want struct {
		status     domain.ArtifactStatus
		version    string
		channel    string
		coordinate string
		detail     []string // substrings
	}
	cases := []struct {
		name    string
		release string
		setup   func(w *world, def *catalog.ProductDefinition)
		want    map[string]want
		check   func(t *testing.T, rel *domain.Release)
	}{
		{
			name:    "verified at channels",
			release: "1.2.0",
			want: map[string]want{
				"controller-image": {status: domain.ArtifactVerified, version: "v1.2.0", channel: "oci", coordinate: "quay.io/acme/acme-controller:v1.2.0"},
				"install-manifest": {status: domain.ArtifactVerified, version: "v1.2.0", channel: "http", coordinate: manifestURL("v1.2.0")},
				"chart":            {status: domain.ArtifactVerified, version: "0.5.1", channel: "helm-repo", coordinate: "https://charts.acme.example/acme:0.5.1"},
				"crds":             {status: domain.ArtifactVerified, version: "v1.2.0", channel: "repo-dir"},
				"cli":              {status: domain.ArtifactExpected, detail: []string{"independent"}},
			},
			check: func(t *testing.T, rel *domain.Release) {
				img := artifactOf(t, rel, "controller-image")
				if img.Digest != "sha256:0000000000000000000000000000000000000000000000000000000000000005" {
					t.Errorf("digest: %q", img.Digest)
				}
				man := artifactOf(t, rel, "install-manifest")
				if man.Digest != domain.Digest([]byte(manifestFor("v1.2.0"))) {
					t.Errorf("manifest digest from retrieved body: %q", man.Digest)
				}
				ev, _ := evidenceByID(rel, man.Evidence[0])
				if ev.Kind != domain.EvidenceReleaseAsset || ev.ContentDigest != man.Digest {
					t.Errorf("manifest evidence: %+v", ev)
				}
				chart := artifactOf(t, rel, "chart")
				if len(chart.Evidence) != 2 { // index entry + probe
					t.Errorf("chart evidence: %v", chart.Evidence)
				}
				published := map[string]bool{}
				for _, f := range factsOf(rel, domain.FactArtifactPublished) {
					published[f.Attributes["artifact"]] = true
				}
				for _, id := range []string{"controller-image", "install-manifest", "chart", "crds"} {
					if !published[id] {
						t.Errorf("no artifact.published fact for %s", id)
					}
				}
				if published["cli"] {
					t.Error("expected artifacts must not be published facts")
				}
				// lookup index status
				st := statusesOf(rel, "chart")
				if len(st) == 0 || st[0].Kind != catalog.LocatorHelmRepo || !strings.Contains(st[0].Detail, "2 with appVersion v1.2.0; selected 0.5.1") {
					t.Errorf("chart statuses: %+v", st)
				}
			},
		},
		{
			// The cert-manager situation in this sandbox: quay.io is
			// unreachable, the install manifest is downloadable and names
			// the image tag.
			name:    "registry unavailable, referenced by the install manifest",
			release: "1.2.0",
			setup:   func(w *world, _ *catalog.ProductDefinition) { w.down[catalog.LocatorOCI] = true },
			want: map[string]want{
				"controller-image": {status: domain.ArtifactReferenced, version: "v1.2.0", channel: "reference:install-manifest",
					coordinate: "quay.io/acme/acme-controller:v1.2.0",
					detail:     []string{"referenced by install-manifest", "L10", "not verifiable: quay.io/acme/acme-controller:v1.2.0 (oci: unavailable)"}},
				"install-manifest": {status: domain.ArtifactVerified, channel: "http"},
				"chart":            {status: domain.ArtifactVerified, version: "0.5.1", channel: "helm-repo"},
			},
			check: func(t *testing.T, rel *domain.Release) {
				img := artifactOf(t, rel, "controller-image")
				var ref domain.Evidence
				for _, id := range img.Evidence {
					if e, _ := evidenceByID(rel, id); e.SourceID == "install-manifest" && e.Locator != "" {
						ref = e
					}
				}
				if ref.Kind != domain.EvidenceReleaseAsset || ref.Locator != "L10" ||
					ref.Excerpt != `image: "quay.io/acme/acme-controller:v1.2.0"` || ref.ContentDigest == "" {
					t.Fatalf("reference evidence: %+v", ref)
				}
				st := statusesOf(rel, "controller-image")
				if len(st) != 1 || st[0].State != domain.SourceUnavailable || st[0].Kind != "oci" {
					t.Fatalf("channel status: %+v", st)
				}
				facts := factsOf(rel, domain.FactArtifactPublished)
				found := false
				for _, f := range facts {
					if f.Attributes["artifact"] == "controller-image" && f.Attributes["status"] == "referenced" {
						found = true
					}
				}
				if !found {
					t.Fatalf("referenced artifact must have an artifact.published fact: %+v", facts)
				}
			},
		},
		{
			name:    "registry reachable but tag absent",
			release: "1.2.0",
			setup: func(w *world, _ *catalog.ProductDefinition) {
				delete(w.images, "oci://quay.io/acme/acme-controller@v1.2.0")
			},
			want: map[string]want{
				"controller-image": {status: domain.ArtifactMissing, version: "v1.2.0",
					detail: []string{"absent from quay.io/acme/acme-controller:v1.2.0", "although absent from every reachable channel"}},
			},
		},
		{
			name:    "registry unavailable and the manifest does not reference the image",
			release: "1.2.0",
			setup: func(w *world, _ *catalog.ProductDefinition) {
				w.down[catalog.LocatorOCI] = true
				w.docs[manifestURL("v1.2.0")] = manifestFor("v1.2.0-rc.7")
			},
			want: map[string]want{
				"controller-image": {status: domain.ArtifactExpected, version: "v1.2.0",
					detail: []string{`"quay.io/acme/acme-controller:v1.2.0" not found in install-manifest`}},
			},
		},
		{
			name:    "registry and manifest unavailable",
			release: "1.2.0",
			setup: func(w *world, _ *catalog.ProductDefinition) {
				w.down[catalog.LocatorOCI] = true
				w.errs[manifestURL("v1.2.0")] = unavailable("github.com")
			},
			want: map[string]want{
				"controller-image": {status: domain.ArtifactExpected, detail: []string{"referencing artifact install-manifest was not retrieved"}},
				"install-manifest": {status: domain.ArtifactExpected},
			},
		},
		{
			name:    "lookup earliest",
			release: "1.2.0",
			setup:   func(_ *world, def *catalog.ProductDefinition) { def.Artifacts[2].Version.Select = "earliest" },
			want:    map[string]want{"chart": {status: domain.ArtifactVerified, version: "0.5.0"}},
		},
		{
			name:    "lookup all",
			release: "1.2.0",
			setup:   func(_ *world, def *catalog.ProductDefinition) { def.Artifacts[2].Version.Select = "all" },
			check: func(t *testing.T, rel *domain.Release) {
				var got []string
				for _, a := range artifactsOf(rel, "chart") {
					got = append(got, a.Version+"="+string(a.Status))
				}
				if !reflect.DeepEqual(got, []string{"0.5.0=verified", "0.5.1=verified"}) {
					t.Fatalf("instances: %v", got)
				}
				// contents are captured once, for the latest selected version
				if vs := rel.Snapshot(domain.SnapshotHelmValues, "chart"); vs == nil || vs.Values.Version != "0.5.1" {
					t.Fatalf("values snapshot: %+v", vs)
				}
			},
		},
		{
			// Argo CD: some upstream patches never ship in a chart.
			name:    "lookup without a match is missing, not an error",
			release: "1.2.1",
			want: map[string]want{
				"chart": {status: domain.ArtifactMissing, coordinate: "https://charts.acme.example/acme",
					detail: []string{"no chart version ships appVersion v1.2.1"}},
			},
			check: func(t *testing.T, rel *domain.Release) {
				for _, id := range []string{"chart/helm-values", "chart/chart-metadata"} {
					s := statusOf(t, rel, id)
					if s.State != domain.SourceSkipped || !strings.Contains(s.Detail, "artifact version") {
						t.Errorf("%s: %+v", id, s)
					}
				}
			},
		},
		{
			name:    "lookup with every index unreachable",
			release: "1.2.0",
			setup:   func(w *world, _ *catalog.ProductDefinition) { w.down[catalog.LocatorHelmRepo] = true },
			want: map[string]want{
				"chart": {status: domain.ArtifactExpected, detail: []string{"no version index answered", "helm-repo index", "unavailable", "does not expose appVersion"}},
			},
		},
		{
			name:    "lookup falls back to the next index exposing the field",
			release: "1.2.0",
			setup: func(w *world, _ *catalog.ProductDefinition) {
				w.down[catalog.LocatorHelmRepo] = true
				w.indexes["oci://ghcr.io/acme/charts/acme"] = []sources.ArtifactVersion{
					{Version: "0.5.0", Fields: map[string]string{"appVersion": "v1.2.0"}},
				}
				w.images["oci://ghcr.io/acme/charts/acme@0.5.0"] = "sha256:oci-chart"
			},
			want: map[string]want{"chart": {status: domain.ArtifactVerified, version: "0.5.0", channel: "oci", coordinate: "ghcr.io/acme/charts/acme:0.5.0"}},
		},
		{
			name:    "availability excludes an artifact",
			release: "1.0.1",
			want: map[string]want{
				"crds": {status: domain.ArtifactNotApplicable, detail: []string{`availability ">= 1.1.0"`}},
			},
			check: func(t *testing.T, rel *domain.Release) {
				if len(statusesOf(rel, "crds/crds")) != 0 || rel.Snapshot(domain.SnapshotCRDs, "crds") != nil {
					t.Fatal("contents of a non-applicable artifact must not be captured")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel, _, _ := ingestFixture(t, tc.release, tc.setup)
			for id, w := range tc.want {
				got := artifactOf(t, rel, id)
				if got.Status != w.status {
					t.Errorf("%s status = %s (%s), want %s", id, got.Status, got.Detail, w.status)
				}
				if w.version != "" && got.Version != w.version {
					t.Errorf("%s version = %q, want %q", id, got.Version, w.version)
				}
				if w.channel != "" && got.Channel != w.channel {
					t.Errorf("%s channel = %q, want %q", id, got.Channel, w.channel)
				}
				if w.coordinate != "" && got.Coordinate != w.coordinate {
					t.Errorf("%s coordinate = %q, want %q", id, got.Coordinate, w.coordinate)
				}
				for _, d := range w.detail {
					if !strings.Contains(got.Detail, d) {
						t.Errorf("%s detail %q does not contain %q", id, got.Detail, d)
					}
				}
				if got.Type == "" || got.Name == "" {
					t.Errorf("%s: type/name not set: %+v", id, got)
				}
			}
			if tc.check != nil {
				tc.check(t, rel)
			}
		})
	}
}

func TestIngestReleaseContents(t *testing.T) {
	rel, w, _ := ingestFixture(t, "1.2.0", nil)

	values := rel.Snapshot(domain.SnapshotHelmValues, "chart")
	if values == nil || values.Values.Chart != "acme" || values.Values.Version != "0.5.1" || values.Values.Entries["logLevel"] != `"info"` {
		t.Fatalf("values snapshot: %+v", values)
	}
	ev, _ := evidenceByID(rel, values.Evidence[0])
	if ev.Kind != domain.EvidenceStructured || ev.ContentDigest == "" || !strings.Contains(ev.URI, "acme-0.5.1") {
		t.Fatalf("values evidence: %+v", ev)
	}
	if s := statusOf(t, rel, "chart/helm-values"); s.State != domain.SourceOK || s.Kind != catalog.LocatorRepoFile {
		t.Fatalf("values status: %+v", s)
	}

	// Chart.yaml kubeVersion → declared minimum constraint.
	var kube *domain.CompatibilityConstraint
	for k := range rel.Compat {
		if rel.Compat[k].SourceID == "chart" {
			kube = &rel.Compat[k]
		}
	}
	if kube == nil {
		t.Fatalf("no chart constraint: %+v", rel.Compat)
	}
	if kube.Platform != "kubernetes" || kube.Kind != "chart-kubeVersion" || kube.Raw != ">= 1.25.0-0" || kube.Constraint != ">=1.25.0-0" ||
		kube.Provenance.Method != domain.MethodDeclared || kube.Provenance.Confidence != domain.ConfidenceHigh || kube.Provenance.Producer != Producer {
		t.Fatalf("chart constraint: %+v", kube)
	}
	kev, _ := evidenceByID(rel, kube.Evidence[0])
	if kev.Kind != domain.EvidenceStructured || kev.Locator != "L5" || kev.Excerpt != `kubeVersion: ">= 1.25.0-0"` {
		t.Fatalf("kubeVersion evidence: %+v", kev)
	}
	if err := kube.Provenance.Validate(); err != nil {
		t.Fatal(err)
	}

	// CRDs from every file of the repo-dir channel, in path order.
	crds := rel.Snapshot(domain.SnapshotCRDs, "crds")
	if crds == nil || len(crds.CRDs.CRDs) != 2 || crds.CRDs.CRDs[0].Name != "gadgets.acme.example" || len(crds.Evidence) != 2 {
		t.Fatalf("crd snapshot: %+v", crds)
	}
	e0, _ := evidenceByID(rel, crds.Evidence[0])
	if !strings.HasSuffix(e0.URI, "/gadgets.yaml") || e0.Locator != "deploy/crds/gadgets.yaml" {
		t.Fatalf("crd evidence must follow path order: %+v", e0)
	}

	// Image references from the install manifest; the channel body is reused.
	imgs := rel.Snapshot(domain.SnapshotImages, "install-manifest")
	if imgs == nil {
		t.Fatal("no image snapshot")
	}
	want := []domain.ImageRef{{Repository: "quay.io/acme/acme-controller", Tag: "v1.2.0"}, {Repository: "quay.io/acme/acme-sidecar", Tag: "v1.2.0"}}
	if !reflect.DeepEqual(imgs.Images.Images, want) {
		t.Fatalf("images: %+v", imgs.Images.Images)
	}
	if n := w.callCount(manifestURL("v1.2.0")); n != 1 {
		t.Fatalf("manifest fetched %d times", n)
	}

	snapFacts := factsOf(rel, domain.FactSnapshot)
	if len(snapFacts) != 4 { // values, chart metadata, crds, images
		t.Fatalf("snapshot facts: %+v", snapFacts)
	}
}

func TestIngestReleaseContentsIgnoreKeys(t *testing.T) {
	rel, _, _ := ingestFixture(t, "1.2.0", func(w *world, def *catalog.ProductDefinition) {
		for i := range def.Artifacts {
			if def.Artifacts[i].ID != "chart" {
				continue
			}
			for j := range def.Artifacts[i].Contents {
				if def.Artifacts[i].Contents[j].Kind == catalog.ContentHelmValues {
					def.Artifacts[i].Contents[j].IgnoreKeys = []string{"image", "nonexistent"}
				}
			}
		}
	})
	values := rel.Snapshot(domain.SnapshotHelmValues, "chart")
	if values == nil {
		t.Fatal("no values snapshot")
	}
	if _, ok := values.Values.Entries["image"]; ok {
		t.Errorf("ignored key must be excluded: %v", values.Values.Entries)
	}
	if values.Values.Entries["logLevel"] != `"info"` || values.Values.Entries["replicaCount"] != `"1"` {
		t.Errorf("other keys must be kept: %v", values.Values.Entries)
	}
	if s := statusOf(t, rel, "chart/helm-values"); s.State != domain.SourceOK || !strings.Contains(s.Detail, "2 values keys (1 ignored)") {
		t.Errorf("status must say how many keys were ignored: %+v", s)
	}
}

func TestIgnoreValuesKeys(t *testing.T) {
	vs, err := DefaultParser.ValuesSnapshot("c", "1.0.0", []byte(`
# the image
image:
  # registry
  hub: gcr.io/istio-testing
  tag: latest
  pullPolicy: IfNotPresent
imageRegistry: quay.io
global:
  hub: gcr.io/istio-testing
  tag: latest
  proxy:
    image: proxyv2
annotations:
  "a.b/c": x
  plain: y
replicas: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	got, n := ignoreValuesKeys(vs, []string{"global.hub", "global.tag", "image.*", `annotations.*`, "missing.*", "replicas.*"})
	var keys []string
	for k := range got.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := "global.proxy.image,imageRegistry,replicas"; strings.Join(keys, ",") != want {
		t.Errorf("remaining keys = %v, want %s", keys, want)
	}
	if n != len(vs.Entries)-len(got.Entries) || n != 7 {
		t.Errorf("removed = %d (entries %d → %d)", n, len(vs.Entries), len(got.Entries))
	}
	for k := range got.Comments {
		if strings.HasPrefix(k, "image") && k != "imageRegistry" {
			t.Errorf("comment of an ignored key survived: %q", k)
		}
	}
	if len(vs.Entries) != 10 || vs.Chart != "c" || got.Chart != "c" || got.Version != "1.0.0" {
		t.Errorf("input must not be modified and identity kept: %d entries, %+v", len(vs.Entries), got)
	}
	// exact entries match only the exact key; "image.*" does not match "imageRegistry"
	if _, ok := got.Entries["imageRegistry"]; !ok {
		t.Error("imageRegistry must not match image.*")
	}
	if exact, n := ignoreValuesKeys(vs, []string{"image"}); n != 0 || len(exact.Entries) != len(vs.Entries) {
		t.Errorf("an exact entry for a section matches no leaf: removed %d", n)
	}
}

func TestIngestReleaseContentFailure(t *testing.T) {
	rel, _, _ := ingestFixture(t, "1.2.0", func(w *world, _ *catalog.ProductDefinition) {
		w.errs["repo-file:github.com/acme/helm-charts@acme-0.5.1:charts/acme/values.yaml"] = unavailable("raw.githubusercontent.com")
		delete(w.docs, "repo-file:github.com/acme/helm-charts@acme-0.5.1:charts/acme/Chart.yaml")
	})
	if s := statusOf(t, rel, "chart/helm-values"); s.State != domain.SourceUnavailable {
		t.Fatalf("values: %+v", s)
	}
	if s := statusOf(t, rel, "chart/chart-metadata"); s.State != domain.SourceNotFound {
		t.Fatalf("chart metadata: %+v", s)
	}
	if rel.Snapshot(domain.SnapshotHelmValues, "chart") != nil {
		t.Fatal("no snapshot expected")
	}
	if a := artifactOf(t, rel, "chart"); a.Status != domain.ArtifactVerified {
		t.Fatalf("content failures must not affect the artifact: %+v", a)
	}
}

func TestSelectArtifactVersions(t *testing.T) {
	in := []sources.ArtifactVersion{{Version: "10.0.0"}, {Version: "9.1.0"}, {Version: "v9.2.0"}, {Version: "nightly"}, {Version: "9.1.0"}}
	names := func(vs []sources.ArtifactVersion) []string {
		var out []string
		for _, v := range vs {
			out = append(out, v.Version)
		}
		return out
	}
	if got := names(selectArtifactVersions(in, "all")); !reflect.DeepEqual(got, []string{"9.1.0", "v9.2.0", "10.0.0", "nightly"}) {
		t.Fatalf("all: %v", got)
	}
	if got := names(selectArtifactVersions(in, "")); !reflect.DeepEqual(got, []string{"nightly"}) {
		t.Fatalf("latest: %v", got)
	}
	if got := names(selectArtifactVersions(in, "earliest")); !reflect.DeepEqual(got, []string{"9.1.0"}) {
		t.Fatalf("earliest: %v", got)
	}
}
