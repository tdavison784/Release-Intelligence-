package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/redaction"
)

// A synthetic Gadget whose settings below the credential-named map
// spec.credentials are stored redacted (as the collector stores them): the
// enum setting is read back for the model, the free text is not.

var gadgetFile = "apiVersion: example.test/v1\nkind: Gadget\nmetadata:\n  name: g\n  namespace: lab\nspec:\n  credentials:\n" +
	"    rotation: \"" + redaction.Marker("Never") + "\"\n" +
	"    comment: \"" + redaction.Marker("rotate by hand") + "\"\n" +
	"    size: 256\n"

const gadgetCRD = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: gadgets.example.test
spec:
  group: example.test
  names: {kind: Gadget, plural: gadgets}
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              credentials:
                type: object
                properties:
                  rotation: {type: string, enum: [Never, Always]}
                  comment: {type: string}
                  size: {type: integer}
`

func gadgetBundle(t *testing.T, withCRD bool) (*store.Store, *domain.EvidenceBundle, domain.ArtifactRef) {
	t.Helper()
	ctx := context.Background()
	st, err := store.New(t.TempDir())
	must(t, err)
	file, err := st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFile, Path: "env/lab/gadget.yaml", Origin: domain.OriginEnvironment,
		MediaType: "application/yaml", URI: "lab/gadget.yaml", Producer: "test.snapshot@v1", Redacted: true}, []byte(gadgetFile))
	must(t, err)
	arts := []domain.ArtifactRef{file}
	if withCRD {
		crd, err := st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactCRDDefinition, Path: "upstream/gadgets/v1.0.0/crds.yaml", Origin: domain.OriginUpstream,
			MediaType: "application/yaml", URI: "https://example.test/crds.yaml", Producer: "test.collect@v1"}, []byte(gadgetCRD))
		must(t, err)
		arts = append(arts, crd)
	}
	return st, &domain.EvidenceBundle{Manifest: domain.BundleManifest{Artifacts: arts}}, file
}

func TestReadArtifactReadsBackSchemaEnumOnly(t *testing.T) {
	st, b, file := gadgetBundle(t, true)
	s := &Session{cfg: SessionConfig{Bundle: b, Store: st}}
	got := s.readBack(context.Background(), file, gadgetFile, gadgetFile)
	if len(got) != 1 || got[0].Path != "spec.credentials.rotation" || got[0].Value != "Never" || got[0].Marker != redaction.Marker("Never") ||
		got[0].Object != "example.test/v1/Gadget/lab/g" || !strings.Contains(got[0].Note, "not a credential") {
		t.Fatalf("read back %+v, want only spec.credentials.rotation = Never", got)
	}
	// only markers inside the window are explained
	if w := s.readBack(context.Background(), file, gadgetFile, "    size: 256"); len(w) != 0 {
		t.Fatalf("explained markers outside the window: %+v", w)
	}
	// no CRD in the bundle: nothing is read back
	st2, b2, file2 := gadgetBundle(t, false)
	s2 := &Session{cfg: SessionConfig{Bundle: b2, Store: st2}}
	if w := s2.readBack(context.Background(), file2, gadgetFile, gadgetFile); len(w) != 0 {
		t.Fatalf("read back without a schema: %+v", w)
	}
}

func TestDossierReadsBackSchemaEnumOnly(t *testing.T) {
	for _, withCRD := range []bool{true, false} {
		st, b, file := gadgetBundle(t, withCRD)
		db := &dossierBuilder{in: DossierInput{Bundle: b, Read: StoreReader(st)}, b: b, d: &Dossier{}}
		must(t, db.view(context.Background()))
		_ = file
		db.objectSetting(settingSpec{kind: "object-field", group: "example.test", objKd: "Gadget", path: "spec.credentials.rotation"}, nil)
		db.objectSetting(settingSpec{kind: "object-field", group: "example.test", objKd: "Gadget", path: "spec.credentials.comment"}, nil)
		if len(db.d.Exposed) != 2 {
			t.Fatalf("exposed %+v", db.d.Exposed)
		}
		rot, com := db.d.Exposed[0], db.d.Exposed[1]
		dec := func(v string) (s string) { _ = json.Unmarshal([]byte(v), &s); return s } // values are JSON-encoded
		if dec(com.Value) != redaction.Marker("rotate by hand") || com.Reason != "" {
			t.Errorf("free text re-exposed (crd=%v): %+v", withCRD, com)
		}
		switch {
		case withCRD && (dec(rot.Value) != "Never" || !strings.Contains(rot.Reason, "stored redacted as "+redaction.Marker("Never"))):
			t.Errorf("enum setting not read back: %+v", rot)
		case !withCRD && dec(rot.Value) != redaction.Marker("Never"):
			t.Errorf("read back without a schema: %+v", rot)
		}
	}
}

// effectiveFixture: a kustomize base declares the Gadget with the rotation
// unset; the prod overlay patches it (stored redacted). The overlay does not
// deploy the product: its version-independent render is the effective object.
func effectiveFixture(t *testing.T, withCRD, overlayRendered bool) (*store.Store, *domain.EvidenceBundle) {
	t.Helper()
	ctx := context.Background()
	st, err := store.New(t.TempDir())
	must(t, err)
	put := func(kind domain.ArtifactKind, origin domain.ArtifactOrigin, path, content string) domain.ArtifactRef {
		ref, err := st.Put(ctx, investigate.ArtifactSpec{Kind: kind, Path: path, Origin: origin, MediaType: "application/yaml", URI: path, Producer: "test@v1"}, []byte(content))
		must(t, err)
		return ref
	}
	g := func(rotation string) string {
		s := "apiVersion: example.test/v1\nkind: Gadget\nmetadata:\n  name: g\n  namespace: lab\nspec:\n  credentials:\n    size: 256\n"
		if rotation != "" {
			s += "    rotation: \"" + rotation + "\"\n"
		}
		return s
	}
	arts := []domain.ArtifactRef{
		put(domain.ArtifactEnvironmentFile, domain.OriginEnvironment, "env/lab/base/kustomization.yaml", "resources:\n  - gadget.yaml\n"),
		put(domain.ArtifactEnvironmentFile, domain.OriginEnvironment, "env/lab/base/gadget.yaml", g("")),
		put(domain.ArtifactEnvironmentFile, domain.OriginEnvironment, "env/lab/overlays/prod/kustomization.yaml", "resources:\n  - ../../base\npatches:\n  - path: rotation.yaml\n"),
		put(domain.ArtifactEnvironmentFile, domain.OriginEnvironment, "env/lab/overlays/prod/rotation.yaml", g(redaction.Marker("Never"))),
	}
	if withCRD {
		arts = append(arts, put(domain.ArtifactCRDDefinition, domain.OriginUpstream, "upstream/gadgets/v1.0.0/crds.yaml", gadgetCRD))
	}
	pair := domain.ManifestPair{Scope: domain.RenderEnvironment, Target: "kustomize:lab/overlays/prod", Status: domain.CollectionNotApplicable,
		Reason: "version-independent: the overlay neither pins nor deploys gadgets"}
	if overlayRendered {
		r := put(domain.ArtifactRenderedManifests, domain.OriginDerived, "derived/render/env/kustomize-lab-overlays-prod/from.yaml", g(redaction.RenderMarker("Never")))
		arts, pair.From = append(arts, r), r.ID
	}
	return st, &domain.EvidenceBundle{Manifest: domain.BundleManifest{Artifacts: arts}, Renders: []domain.ManifestPair{pair}}
}

func exposedFor(t *testing.T, st *store.Store, b *domain.EvidenceBundle, subject string) (settingSpec, []DossierObject) {
	t.Helper()
	db := &dossierBuilder{in: DossierInput{Bundle: b, Read: StoreReader(st)}, b: b, d: &Dossier{}}
	must(t, db.view(context.Background()))
	s := db.objectSpec(strings.Split(subject, "."), "item subject")
	db.objectSetting(s, nil)
	return s, db.d.Exposed
}

// The dossier reads the pin validator's effective object view: the overlay
// render (base + patch), not the base file; a Go-cased subject is the JSON
// field path its CRD confirms; the patched value is read back.
func TestDossierEffectiveViewAndJSONPath(t *testing.T) {
	st, b := effectiveFixture(t, true, true)
	s, objs := exposedFor(t, st, b, "Gadget.Spec.Credentials.Rotation")
	if s.path != "spec.credentials.rotation" || s.unverified {
		t.Fatalf("spec %+v", s)
	}
	if len(objs) != 1 {
		t.Fatalf("objects %+v", objs)
	}
	o := objs[0]
	var v string
	_ = json.Unmarshal([]byte(o.Value), &v)
	if o.State != SettingExplicit || v != "Never" || o.View != "render kustomize:lab/overlays/prod (version-independent overlay)" ||
		!strings.Contains(o.Reason, "stored redacted as "+redaction.RenderMarker("Never")) {
		t.Fatalf("object %+v", o)
	}
}

// Fail closed: a path no CRD confirms proves nothing when unset; an object
// whose consuming render is unavailable is unknown whatever its file says.
func TestDossierFailsClosed(t *testing.T) {
	t.Run("unconfirmed Go path", func(t *testing.T) {
		st, b := effectiveFixture(t, false, true)
		s, objs := exposedFor(t, st, b, "Gadget.Spec.Credentials.RotationWindow")
		if s.path != "spec.credentials.rotationWindow" || !s.unverified {
			t.Fatalf("spec %+v", s)
		}
		if len(objs) != 1 || objs[0].State != SettingUnknown || !strings.Contains(objs[0].Reason, "no CRD in the bundle confirms") {
			t.Fatalf("objects %+v", objs)
		}
	})
	t.Run("overlay render unavailable", func(t *testing.T) {
		st, b := effectiveFixture(t, true, false)
		_, objs := exposedFor(t, st, b, "Gadget.Spec.Credentials.Rotation")
		if len(objs) == 0 {
			t.Fatal("no objects")
		}
		for _, o := range objs {
			if o.State != SettingUnknown || !strings.Contains(o.Reason, "whose rendered objects are not available") {
				t.Fatalf("object %+v", o)
			}
		}
	})
}

func TestLowerCamel(t *testing.T) {
	for in, want := range map[string]string{"RotationPolicy": "rotationPolicy", "TLSConfig": "tlsConfig", "CABundle": "caBundle", "Spec": "spec", "ID": "id", "spec": "spec"} {
		if got := lowerCamel(in); got != want {
			t.Errorf("lowerCamel(%q) = %q, want %q", in, got, want)
		}
	}
}
