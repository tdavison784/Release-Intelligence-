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
	"github.com/tdavison784/release-intelligence/internal/render"
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
		objs, err := render.ParseObjects([]byte(gadgetFile))
		must(t, err)
		db := &dossierBuilder{in: DossierInput{Bundle: b, Read: StoreReader(st)}, b: b, d: &Dossier{},
			objs: []viewObject{{id: objs[0].ID, body: objs[0].Body, target: "lab/gadget.yaml", artifact: file.ID}}}
		db.readBack(context.Background())
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
