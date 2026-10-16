package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Name is the validator name (component@vN): its records' Validator and
// provenance producer.
const Name = "runtime.isolated-upgrade@v1"

// Defaults (DESIGN §2.6 runtime validation; brief: ≤ 10 min per
// investigation, configurable).
const (
	DefaultTimeout     = 10 * time.Minute
	DefaultMinFreeDisk = 10 << 30 // bytes
	DefaultSettle      = 20 * time.Second
)

// Clock is the validator's time source (a fake in tests).
type Clock interface {
	Now() time.Time
	// Sleep waits d or until ctx ends (then it returns ctx.Err()).
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Option configures the validator.
type Option func(*Validator)

// WithStore gives access to the bundle's artifacts (the stored renders the
// re-renders are verified against) and stores the run's observations.
// Required: without it every record is not-run.
func WithStore(st investigate.ArtifactStore) Option { return func(v *Validator) { v.store = st } }

// WithRenderer sets how raw, applyable manifests are obtained
// (NewAppRenderer re-renders through internal/render). Required.
func WithRenderer(r Renderer) Option { return func(v *Validator) { v.renderer = r } }

// WithDriver sets the cluster driver (default: KindDriver).
func WithDriver(d Driver) Option { return func(v *Validator) { v.driver = d } }

// WithExercises sets the declared exercises (FromSets over
// catalog.LoadExercises("products/exercises")). Without exercises nothing
// can pass: a quiet window proves nothing.
func WithExercises(xs ...DeclaredExercise) Option {
	return func(v *Validator) { v.exercises = append(v.exercises, xs...) }
}

// WithTimeout bounds the wall clock of one run (cluster creation to
// observation; deletion has its own bound).
func WithTimeout(d time.Duration) Option { return func(v *Validator) { v.timeout = d } }

// WithSettle sets how long controllers get to act before an observation
// (exercises may declare their own).
func WithSettle(d time.Duration) Option { return func(v *Validator) { v.settle = d } }

// WithMinFreeDisk sets the free-disk threshold below which no cluster is
// created.
func WithMinFreeDisk(bytes uint64) Option { return func(v *Validator) { v.minFreeDisk = bytes } }

// WithNodeImage overrides the kind node image ("" keeps the pinned table).
func WithNodeImage(image string) Option { return func(v *Validator) { v.nodeImageOverride = image } }

// WithClock sets the clock (tests).
func WithClock(c Clock) Option { return func(v *Validator) { v.clock = c } }

// WithLogf receives progress lines (cluster created, phase done, ...).
func WithLogf(f func(format string, args ...any)) Option { return func(v *Validator) { v.logf = f } }

// Validator is the runtime validator: it installs the bundle's environment
// at the FROM version in a throwaway kind cluster, upgrades it to TO, runs
// the declared exercises at both versions and records, per change item, what
// was observed on the objects of the item's impact paths (level observed).
// One cluster run serves every item of a bundle; it is cached per bundle.
type Validator struct {
	store             investigate.ArtifactStore
	renderer          Renderer
	driver            Driver
	exercises         []DeclaredExercise
	clock             Clock
	logf              func(string, ...any)
	timeout           time.Duration
	settle            time.Duration
	minFreeDisk       uint64
	nodeImageOverride string
	// finer bounds (not options: derived from the timeout's scale)
	createTimeout, deleteTimeout, applyTimeout, reconcileTimeout, readyTimeout, stableWindow time.Duration
	pollInterval, retryInterval                                                              time.Duration
	parallelism                                                                              int

	mu   sync.Mutex
	runs map[string]*session
}

// New returns the runtime validator.
func New(opts ...Option) *Validator {
	v := &Validator{clock: realClock{}, timeout: DefaultTimeout, settle: DefaultSettle, minFreeDisk: DefaultMinFreeDisk,
		createTimeout: 3 * time.Minute, deleteTimeout: 3 * time.Minute, applyTimeout: 3 * time.Minute, reconcileTimeout: time.Minute,
		readyTimeout: 4 * time.Minute, stableWindow: 45 * time.Second, pollInterval: 3 * time.Second, retryInterval: 5 * time.Second,
		parallelism: 8, runs: map[string]*session{}}
	for _, o := range opts {
		o(v)
	}
	if v.driver == nil {
		v.driver = NewKindDriver()
	}
	if v.logf == nil {
		v.logf = func(string, ...any) {}
	}
	return v
}

func (v *Validator) Name() string                { return Name }
func (v *Validator) Kind() domain.ValidationKind { return domain.ValidationRuntime }

// Applies: an isolated upgrade observes every family; whether an item can
// pass depends on its paths and the declared exercises, decided per item.
func (v *Validator) Applies(domain.ChangeItemFamily) bool { return true }

// Validate runs (or reuses) the bundle's cluster run and derives the item's
// record. Errors are for invalid input and cancellation only; every other
// problem is a not-run or error record with the reason.
func (v *Validator) Validate(ctx context.Context, in investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if in.Bundle == nil {
		return nil, fmt.Errorf("runtime: %w: no bundle", investigate.ErrRejected)
	}
	item, ok := in.Bundle.Item(in.ChangeItem)
	if !ok {
		return nil, fmt.Errorf("runtime: change item %q: %w", in.ChangeItem, investigate.ErrNotFound)
	}
	cov, err := coverageOf(in, item)
	if err != nil {
		return nil, err
	}
	s, err := v.session(ctx, in.Bundle, in.Graph)
	if err != nil {
		return nil, err
	}
	return s.record(in, item, cov)
}

// Run performs the bundle's isolated upgrade (once per bundle; later calls
// and every Validate of the bundle reuse it) and returns the run record. A
// cancelled run is not cached and returns the context error.
func (v *Validator) Run(ctx context.Context, b *domain.EvidenceBundle, g *domain.RelationshipGraph) (*Run, error) {
	s, err := v.session(ctx, b, g)
	if err != nil {
		return nil, err
	}
	return s.run, nil
}

func (v *Validator) session(ctx context.Context, b *domain.EvidenceBundle, g *domain.RelationshipGraph) (*session, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if s, ok := v.runs[b.ID]; ok {
		return s, nil
	}
	s := &session{v: v, b: b, g: g, run: &Run{SchemaVersion: RunSchemaVersion, Validator: Name, Bundle: b.ID,
		Product: string(b.Change.Product.ID), From: b.Change.From.String(), To: b.Change.To.String()}}
	v.logf("runtime: %s %s → %s on %s", s.run.Product, s.run.From, s.run.To, b.ID)
	s.execute(ctx)
	if s.cancelled != nil {
		return nil, s.cancelled
	}
	if err := s.store(context.WithoutCancel(ctx)); err != nil {
		return nil, investigate.TransientError(domain.StageValidate, fmt.Errorf("runtime: %w", err))
	}
	v.logf("runtime: run %s: %s %s (%.0fs)", b.ID, s.run.Status, s.run.Reason, s.run.DurationSeconds)
	v.runs[b.ID] = s
	return s, nil
}

// RunRecord returns the run record of a bundle already run (nil otherwise).
func (v *Validator) RunRecord(bundleID string) *Run {
	v.mu.Lock()
	defer v.mu.Unlock()
	if s, ok := v.runs[bundleID]; ok {
		return s.run
	}
	return nil
}

// kindNodeImages pins the node image per Kubernetes minor: the images kind
// v0.24.0 publishes (release notes, github.com/kubernetes-sigs/kind/releases/tag/v0.24.0).
var kindNodeImages = map[string]string{
	"1.31": "kindest/node:v1.31.0@sha256:53df588e04085fd41ae12de0c3fe4c72f7013bba32a20e7325357a1ac94ba865",
	"1.30": "kindest/node:v1.30.4@sha256:976ea815844d5fa93be213437e3ff5754cd599b040946b5cca43ca45c2047114",
	"1.29": "kindest/node:v1.29.8@sha256:d46b7aa29567e93b27f7531d258c372e829d7224b25e3fc6ffdefed12476d3aa",
	"1.28": "kindest/node:v1.28.13@sha256:45d319897776e11167e4698f6b14938eb4d52eb381d9e3d7a9086c16c69a8110",
	"1.27": "kindest/node:v1.27.16@sha256:3fd82731af34efe19cd54ea5c25e882985bafa2c9baefe14f8deab1737d9fabe",
	"1.26": "kindest/node:v1.26.15@sha256:1cc15d7b1edd2126ef051e359bf864f37bbcf1568e61be4d2ed1df7a3e87b354",
	"1.25": "kindest/node:v1.25.16@sha256:6110314339b3b44d10da7d27881849a87e092124afab5956f2e10ecdb463b025",
}

// nodeImage returns the node image for the environment's Kubernetes version
// ("" = the driver's default when the bundle states none), or why none fits.
func (v *Validator) nodeImage(kube string) (string, string) {
	if v.nodeImageOverride != "" {
		return v.nodeImageOverride, ""
	}
	if kube == "" {
		return "", ""
	}
	minor := strings.TrimPrefix(kube, "v")
	if parts := strings.Split(minor, "."); len(parts) >= 2 {
		minor = parts[0] + "." + parts[1]
	}
	if img, ok := kindNodeImages[minor]; ok {
		return img, ""
	}
	return "", fmt.Sprintf("no pinned kind node image for Kubernetes %s (pinned: 1.25–1.31): the environment's cluster version cannot be reproduced", kube)
}
