package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// FixtureClusterPrefix names every cluster a fixture run creates (lane
// verify: synthetic fixtures, never an evaluation environment). A fixture run
// and an environment run never overlap: each refuses to start while a cluster
// of either prefix exists.
const FixtureClusterPrefix = "ri-verify-"

func isIsolatedCluster(name string) bool {
	return strings.HasPrefix(name, ClusterPrefix) || strings.HasPrefix(name, FixtureClusterPrefix) || strings.HasPrefix(name, AdmissionClusterPrefix)
}

// FixtureSpec is an isolated upgrade of a synthetic fixture instead of a
// bundle's environment (Phase 5 PROTOCOL §7.1): the product's controller at
// FROM and TO (raw manifests, e.g. a chart-defaults render) and a few
// synthetic objects applied unchanged at both versions. The run is otherwise
// exactly an environment run: the same phases, exercises, observations,
// degradation rules and cleanup on every exit path.
type FixtureSpec struct {
	// ID is the fixture's content id: it names the cluster
	// (FixtureClusterPrefix + 8 hex) and the run's artifacts.
	ID                string
	Product, From, To string
	// KubernetesVersion selects the pinned node image ("" = kind's default).
	KubernetesVersion string
	// Controller is the product at each version.
	FromController, ToController RawRender
	// Objects are the synthetic objects (a YAML stream), applied at both
	// versions into Namespace when they name none.
	Objects   []byte
	Namespace string
	// Observe are object ids ("<group>/<version>/<kind>/<namespace>/<name>",
	// core group "core") of objects controllers produce from the fixture
	// (e.g. the Secret a subject object's controller writes): observed like
	// applied objects, their data digested per key.
	Observe []string
	// Consumers are consumer observations made at each version
	// (consumer.go; lane verify2). Optional.
	Consumers []ConsumerCheck
}

// FixtureResult is a fixture run with its observations by phase.
type FixtureResult struct {
	Run          *Run
	Observations map[string]*Observation
	// Artifacts are the stored run, observation and exercise artifacts (none
	// without a store).
	Artifacts []domain.ArtifactRef
}

// RunFixture performs one isolated upgrade of a synthetic fixture. Problems
// are recorded in the run (not-run / error with the reason); the error is for
// invalid input and cancellation only.
func (v *Validator) RunFixture(ctx context.Context, spec FixtureSpec) (*FixtureResult, error) {
	if spec.ID == "" || spec.Product == "" || spec.From == "" || spec.To == "" {
		return nil, fmt.Errorf("runtime: fixture needs an id, product, from and to")
	}
	if err := validateConsumers(spec.Consumers, v.exercises); err != nil {
		return nil, fmt.Errorf("runtime: fixture: %w", err)
	}
	b := &domain.EvidenceBundle{ID: spec.ID}
	b.Snapshot.KubernetesVersion = spec.KubernetesVersion
	g := &domain.RelationshipGraph{ID: "fixture", Bundle: spec.ID, Builder: "runtime.fixture@v1"}
	for _, key := range spec.Observe {
		if _, ok := ParseRef(key); !ok {
			return nil, fmt.Errorf("runtime: fixture observe %q is not an object id", key)
		}
		g.Nodes = append(g.Nodes, domain.GraphNode{ID: domain.NodeID(domain.NodeConfigObject, key), Kind: domain.NodeConfigObject, Key: key})
	}
	s := &session{v: v, b: b, g: g, prefix: FixtureClusterPrefix, prepared: true, allConsumed: true, consumers: spec.Consumers,
		run: &Run{SchemaVersion: RunSchemaVersion, Validator: Name, Bundle: spec.ID, Product: spec.Product, From: spec.From, To: spec.To}}
	s.from, s.to = newManifestSet(), newManifestSet()
	for _, side := range []struct {
		set  *manifestSet
		raw  RawRender
		name string
	}{{s.from, spec.FromController, "controller@" + spec.From}, {s.to, spec.ToController, "controller@" + spec.To}} {
		ms, skipped, err := parseManifests(side.raw.Manifests, side.name, side.raw.Namespace, true)
		if err != nil {
			return nil, fmt.Errorf("runtime: fixture %s: %w", side.name, err)
		}
		if len(ms) == 0 {
			return nil, fmt.Errorf("runtime: fixture %s has no objects", side.name)
		}
		for _, sk := range skipped {
			s.note("%s", sk)
		}
		side.set.add(ms)
		s.run.Targets = append(s.run.Targets, TargetUse{Target: side.name, Status: domain.CollectionOK, Product: true, Namespace: side.raw.Namespace, Objects: len(ms), Verified: true})
	}
	objs, skipped, err := parseManifests(spec.Objects, "fixture", spec.Namespace, false)
	if err != nil {
		return nil, fmt.Errorf("runtime: fixture objects: %w", err)
	}
	for _, sk := range skipped {
		s.note("%s", sk)
	}
	s.from.add(objs)
	s.to.add(objs)
	s.run.Targets = append(s.run.Targets, TargetUse{Target: "fixture", Status: domain.CollectionNotApplicable, Namespace: spec.Namespace, Objects: len(objs), Verified: true})
	v.logf("runtime: fixture %s %s → %s (%s)", spec.Product, spec.From, spec.To, spec.ID)
	s.execute(ctx)
	if s.cancelled != nil {
		return nil, s.cancelled
	}
	if err := s.store(context.WithoutCancel(ctx)); err != nil {
		return nil, fmt.Errorf("runtime: fixture: %w", err)
	}
	v.logf("runtime: fixture %s: %s %s (%.0fs)", spec.ID, s.run.Status, s.run.Reason, s.run.DurationSeconds)
	return &FixtureResult{Run: s.run, Observations: s.obs, Artifacts: s.arts}, nil
}
