package bundle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// AssemblerProducer identifies the assembler in provenance.
const AssemblerProducer = "bundle.assemble@v1"

// pinnedClock is the default clock of the assembler and the snapshot
// builder: the zero time. Principle 6: a bundle (and the snapshot inside it)
// is fully determined by its inputs, so re-assembling identical inputs yields
// byte-identical content and storing it again is a no-op. When the run
// happened is recorded where time belongs: the run record's CreatedAt and
// its audit trail. Callers that want real stamps pin a clock explicitly
// (NewAssemblerWithClock / NewSnapshotBuilderWithClock).
func pinnedClock() time.Time { return time.Time{} }

// Assembler joins change evidence, environment evidence and the environment
// renders into the immutable bundle.
type Assembler struct {
	st  investigate.ArtifactStore
	now func() time.Time
}

var _ investigate.BundleAssembler = (*Assembler)(nil)

// NewAssembler returns the bundle assembler. st is used to verify that every
// listed artifact is stored before the bundle is built.
func NewAssembler(st investigate.ArtifactStore) investigate.BundleAssembler {
	return &Assembler{st: st, now: pinnedClock}
}

// NewAssemblerWithClock is NewAssembler with CreatedAt taken from now. The
// clock must be pinned by the caller if byte-identical re-assembly matters.
func NewAssemblerWithClock(st investigate.ArtifactStore, now func() time.Time) investigate.BundleAssembler {
	return &Assembler{st: st, now: now}
}

// Assemble builds and validates the bundle. renders may be nil (no
// environment renders were produced); the bundle then states none, and an
// investigation over it cannot be approved automatically (DESIGN.md §2.4).
func (a *Assembler) Assemble(ctx context.Context, change *investigate.ChangeResult, snap *investigate.SnapshotResult, renders *investigate.RenderResult) (*domain.EvidenceBundle, error) {
	if change == nil || snap == nil {
		return nil, errors.New("assemble: change evidence and the environment snapshot are required")
	}
	var refs []domain.ArtifactRef
	refs = append(refs, change.Artifacts...)
	refs = append(refs, snap.Artifacts...)
	var envRenders []domain.ManifestPair
	var renderEvidence []domain.Evidence
	var envItems []domain.ChangeItem
	if renders != nil {
		refs = append(refs, renders.Artifacts...)
		envRenders = append(envRenders, renders.Pairs...)
		renderEvidence = renders.Evidence
		envItems = append(envItems, renders.Items...)
	}
	sort.SliceStable(envRenders, func(i, j int) bool { return envRenders[i].Target < envRenders[j].Target })
	sort.SliceStable(envItems, func(i, j int) bool { return envItems[i].ID < envItems[j].ID })

	byID, byPath := map[string]domain.ArtifactRef{}, map[string]domain.ArtifactRef{}
	for _, r := range refs {
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("assemble: %w", err)
		}
		if prev, ok := byPath[r.Path]; ok && prev.ID != r.ID {
			return nil, fmt.Errorf("assemble: two different artifacts claim the path %s (%s and %s)", r.Path, prev.Digest, r.Digest)
		}
		byID[r.ID], byPath[r.Path] = r, r
	}
	if a.st != nil {
		for _, r := range byID {
			rc, err := a.st.Open(ctx, r.Digest)
			if err != nil {
				return nil, fmt.Errorf("assemble: artifact %s (%s) is not retrievable: %w", r.ID, r.Path, err)
			}
			rc.Close()
		}
	}
	m := domain.BundleManifest{}
	for _, r := range byID {
		m.Artifacts = append(m.Artifacts, r)
	}
	sort.Slice(m.Artifacts, func(i, j int) bool { return m.Artifacts[i].Path < m.Artifacts[j].Path })

	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, set := range [][]domain.Evidence{change.Evidence, snap.Evidence, renderEvidence} {
		for _, e := range set {
			if _, ok := pool[e.ID]; !ok {
				// Retrieval time is wall clock, not identity (the id excludes it):
				// keeping it would make re-assembly change the bundle's bytes.
				e.RetrievedAt = time.Time{}
				pool[e.ID] = e
			}
		}
	}
	var evidence []domain.Evidence
	for _, e := range pool {
		evidence = append(evidence, e)
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].ID < evidence[j].ID })

	b := &domain.EvidenceBundle{
		SchemaVersion: domain.EvidenceBundleSchemaVersion,
		Change:        change.Record, Snapshot: snap.Snapshot, Renders: envRenders, EnvironmentItems: envItems,
		Manifest: m, Evidence: evidence, Producer: AssemblerProducer, CreatedAt: a.now().UTC().Truncate(time.Second),
	}
	b.ID = domain.BundleID(b.Change.ID, b.Snapshot.ID, b.Manifest.Digest(), b.Renders)
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("assemble: bundle does not validate: %w", err)
	}
	return b, nil
}
