package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/redaction"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// bundleSchemas indexes the CustomResourceDefinitions a bundle holds — the
// upstream CRD artifacts and those its environment files and environment
// renders declare — for reading back withheld enum values
// (redaction.Schemas.Recover). An artifact that cannot be read or parsed
// contributes nothing: fewer schemas only means fewer values read back.
func bundleSchemas(ctx context.Context, b *domain.EvidenceBundle, read func(context.Context, domain.ArtifactRef) ([]byte, error)) *redaction.Schemas {
	ix := redaction.NewSchemas()
	byID := map[string]domain.ArtifactRef{}
	for _, a := range b.Manifest.Artifacts {
		byID[a.ID] = a
	}
	var arts []domain.ArtifactRef
	for _, a := range b.Manifest.Artifacts {
		if a.Kind == domain.ArtifactCRDDefinition || a.Kind == domain.ArtifactEnvironmentFile {
			arts = append(arts, a)
		}
	}
	for _, p := range b.Renders {
		if p.Scope != domain.RenderEnvironment {
			continue
		}
		for _, id := range []string{p.From, p.To} {
			if a, ok := byID[id]; ok {
				arts = append(arts, a)
			}
		}
	}
	for _, a := range arts {
		if ctx.Err() != nil {
			break
		}
		if !strings.HasSuffix(a.Path, ".yaml") && !strings.HasSuffix(a.Path, ".yml") && !strings.HasSuffix(a.Path, ".json") {
			continue
		}
		data, err := read(ctx, a)
		if err != nil || isBinary(data) {
			continue
		}
		objs, err := render.ParseObjects(data)
		if err != nil {
			continue
		}
		for _, o := range objs {
			if o.ID.Kind == "CustomResourceDefinition" && o.ID.Group == "apiextensions.k8s.io" {
				ix.AddCRD(o.Body)
			}
		}
	}
	return ix
}

// readBackNote states that a value was read back from its redaction digest.
func readBackNote(r redaction.Recovered) string {
	return fmt.Sprintf("stored redacted as %s: the digest of %q, an enum value its CRD declares for %s (not a credential)", r.Marker, r.Value, r.Path)
}

// readBack lists the withheld enum values the objects of a read window
// stand for: every marker of the artifact's objects that its CRD's enum
// reads back and that occurs in the window text. The text itself stays
// verbatim, so quotes from it still verify.
func (s *Session) readBack(ctx context.Context, a domain.ArtifactRef, all, window string) []investigate.ReadBack {
	if !strings.Contains(window, redaction.MarkerPrefix) {
		return nil
	}
	objs, err := render.ParseObjects([]byte(all))
	if err != nil {
		return nil
	}
	ix := s.schemaIndex(ctx)
	var out []investigate.ReadBack
	for _, o := range objs {
		for _, r := range ix.Recover(o.ID.Group, o.ID.Version, o.ID.Kind, o.Body) {
			if strings.Contains(window, r.Marker) {
				out = append(out, investigate.ReadBack{Object: o.ID.String(), Path: r.Path, Marker: r.Marker, Value: r.Value, Note: readBackNote(r)})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Object+out[i].Path < out[j].Object+out[j].Path })
	return out
}

// schemaIndex is the session's bundleSchemas, built once (reads through the
// store directly: they are not the model's reads).
func (s *Session) schemaIndex(ctx context.Context) *redaction.Schemas {
	s.schemaOnce.Do(func() {
		s.schemas = bundleSchemas(ctx, s.cfg.Bundle, StoreReader(s.cfg.Store))
	})
	return s.schemas
}
