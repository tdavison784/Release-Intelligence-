package validate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Validator names (component@vN). A validator's name is its records'
// Validator and provenance producer.
const (
	NameJoin               = "validate.join@v1"
	NameRenderDiff         = "validate.render-diff@v1"
	NameOverrideInspection = "validate.override-inspection@v1"
	NameSchemaCompat       = "validate.schema-compat@v1"
	NameStaticReference    = "validate.static-reference@v1"
)

// Option configures a validator.
type Option func(*config)

type config struct {
	store          investigate.ArtifactStore
	now            func() time.Time
	facts          []domain.VerifiedFact
	reportArtifact string
}

// WithStore gives the validator read access to the bundle's artifact bytes
// (renders, environment files, the upgrade edge, knowledge facts). Without a
// store a validator that needs bytes returns not-run.
func WithStore(st investigate.ArtifactStore) Option { return func(c *config) { c.store = st } }

// WithClock sets the clock for CheckedAt (not part of any id).
func WithClock(now func() time.Time) Option { return func(c *config) { c.now = now } }

// WithFacts supplies verified facts loaded by the wiring from knowledge/ in
// addition to the knowledge-fact artifacts of the bundle. Only trusted
// (deterministic or human), active facts attached to the item's change are
// used (OverrideInspection).
func WithFacts(facts ...domain.VerifiedFact) Option {
	return func(c *config) { c.facts = append(c.facts, facts...) }
}

// WithReportArtifact names the artifact (bundle or run) that holds the
// impact report JSON, so wrapped join verdicts cite it as an input.
func WithReportArtifact(id string) Option { return func(c *config) { c.reportArtifact = id } }

func newConfig(opts []Option) config {
	c := config{now: time.Now}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// base implements the identity half of investigate.Validator.
type base struct {
	name     string
	kind     domain.ValidationKind
	families []domain.ChangeItemFamily // nil: every family
	cfg      config
}

func (b base) Name() string                { return b.name }
func (b base) Kind() domain.ValidationKind { return b.kind }

func (b base) Applies(f domain.ChangeItemFamily) bool {
	if b.families == nil {
		return true
	}
	for _, x := range b.families {
		if x == f {
			return true
		}
	}
	return false
}

// draft is a validation record under construction.
type draft struct {
	kind          domain.ValidationKind // "" = the validator's kind
	rule          string                // deciding rule (provenance rule)
	outcome       domain.ValidationOutcomeKind
	property      string
	detail        string
	subjects      []string
	paths         []string
	rcs           []string
	inputs        []string
	outputs       []string
	evidence      []domain.EvidenceID
	pin           *domain.PinnedSetting
	impactFinding string
}

// output collects records and the evidence they cite that the bundle pool
// does not already hold.
type output struct {
	base   base
	in     investigate.ValidationInput
	pool   map[domain.EvidenceID]bool
	extra  domain.EvidenceSet
	recs   []domain.ValidationRecord
	ids    map[string]bool
	errors []error
}

func newOutput(b base, in investigate.ValidationInput) *output {
	pool := map[domain.EvidenceID]bool{}
	for _, e := range in.Bundle.Evidence {
		pool[e.ID] = true
	}
	return &output{base: b, in: in, pool: pool, ids: map[string]bool{}}
}

// cite records e (unless the bundle pool has it) and returns its id.
func (o *output) cite(e domain.Evidence) domain.EvidenceID {
	if !o.pool[e.ID] {
		o.extra.Add(e)
	}
	return e.ID
}

// known reports whether an evidence id resolves (bundle pool or cited here).
func (o *output) known(id domain.EvidenceID) bool { return o.pool[id] || o.extra.Has(id) }

func (o *output) add(d draft) domain.ValidationRecord {
	kind := d.kind
	if kind == "" {
		kind = o.base.kind
	}
	r := domain.ValidationRecord{
		Validator:       o.base.name,
		Kind:            kind,
		Level:           domain.LevelDeterministic,
		ChangeItem:      o.in.ChangeItem,
		Subjects:        uniq(d.subjects),
		Paths:           uniq(d.paths),
		RenderedChanges: uniq(d.rcs),
		Property:        d.property,
		Inputs:          uniq(d.inputs),
		Outputs:         uniq(d.outputs),
		Outcome:         d.outcome,
		Detail:          strings.TrimSpace(d.detail),
		Evidence:        uniqEvidence(d.evidence),
		Pin:             d.pin,
		ImpactFinding:   d.impactFinding,
		Provenance: domain.Provenance{
			Method:     domain.MethodComputed,
			Producer:   o.base.name,
			Rule:       d.rule,
			Confidence: domain.ConfidenceHigh,
		},
		CheckedAt: o.base.cfg.now().UTC().Truncate(time.Second),
	}
	if req := o.in.Request; req != nil {
		r.Request = req.ID
		if req.Hypothesis != "" {
			r.Answers = []string{req.Hypothesis}
		}
	}
	// an undecided record carries exactly what it was asked about
	r.EchoRequested(o.in.Subjects, o.in.Paths, o.in.RenderedChanges)
	r.ID = domain.ValidationRecordID(r.Validator, r.ChangeItem, r.Subjects, r.Paths, r.RenderedChanges, r.Inputs)
	if o.ids[r.ID] {
		o.errors = append(o.errors, fmt.Errorf("validate: %s produced two records with id %s", o.base.name, r.ID))
	}
	o.ids[r.ID] = true
	if err := r.Validate(); err != nil {
		o.errors = append(o.errors, fmt.Errorf("validate: %s produced an invalid record: %w", o.base.name, err))
	}
	o.recs = append(o.recs, r)
	return r
}

func (o *output) result() (*investigate.ValidationOutput, error) {
	if err := errors.Join(o.errors...); err != nil {
		return nil, err
	}
	return &investigate.ValidationOutput{Records: o.recs, Evidence: o.extra.List()}, nil
}

// notRun records an undecided outcome. The record carries the coverage the
// validator was asked about (add echoes the request's subjects, paths and
// rendered changes), so the classifier and the audit see what stayed
// undecided.
func (o *output) notRun(cov coverage, rule, property, detail string, inputs ...string) {
	o.add(draft{rule: rule, outcome: domain.ValidationNotRun, property: property, detail: detail,
		subjects: cov.subjects, paths: cov.paths, inputs: inputs})
}

// prepare checks the input and returns the change item. Invalid input is an
// error (permanent), never a record.
func prepare(ctx context.Context, in investigate.ValidationInput) (domain.ChangeItem, error) {
	if err := ctx.Err(); err != nil {
		return domain.ChangeItem{}, err
	}
	if in.Bundle == nil {
		return domain.ChangeItem{}, fmt.Errorf("validate: %w: no bundle", investigate.ErrRejected)
	}
	// Bundle.Item: record items and environment-scope items (DESIGN §1.3
	// amendment: scope:rendered-unexplained anchored on an environment
	// rendered change lives in EvidenceBundle.EnvironmentItems).
	it, ok := in.Bundle.Item(in.ChangeItem)
	if !ok {
		return domain.ChangeItem{}, fmt.Errorf("validate: change item %q: %w", in.ChangeItem, investigate.ErrNotFound)
	}
	return it, nil
}

// coverage is what a record speaks for: graph nodes and impact paths.
type coverage struct {
	subjects []string
	paths    []string
	// all: the coverage is every root and path of the item (nothing was
	// requested), so an item-wide result may claim it.
	all bool
}

// coverageOf resolves the requested subjects/paths, or every root node and
// path of the item when nothing was requested. Unknown ids are rejected.
func coverageOf(in investigate.ValidationInput, item domain.ChangeItem) (coverage, error) {
	if len(in.Subjects) > 0 || len(in.Paths) > 0 {
		if len(in.Paths) > 0 {
			if in.Traversal == nil {
				return coverage{}, fmt.Errorf("validate: %w: paths requested without a traversal", investigate.ErrRejected)
			}
			mine := map[string]bool{}
			for _, p := range in.Traversal.PathsOf(item.ID) {
				mine[p.ID] = true
			}
			for _, id := range in.Paths {
				if !mine[id] {
					return coverage{}, fmt.Errorf("validate: %w: %s is not an impact path of %s", investigate.ErrRejected, id, item.ID)
				}
			}
		}
		if len(in.Subjects) > 0 && in.Graph != nil {
			nodes := map[string]bool{}
			for _, n := range in.Graph.Nodes {
				nodes[n.ID] = true
			}
			for _, id := range in.Subjects {
				if !nodes[id] {
					return coverage{}, fmt.Errorf("validate: %w: subject %s is not a graph node", investigate.ErrRejected, id)
				}
			}
		}
		return coverage{subjects: uniq(in.Subjects), paths: uniq(in.Paths)}, nil
	}
	cov := coverage{all: true}
	if t := in.Traversal; t != nil {
		for _, r := range t.RootsOf(item.ID) {
			cov.subjects = append(cov.subjects, r.Node)
		}
		for _, p := range t.PathsOf(item.ID) {
			cov.paths = append(cov.paths, p.ID)
		}
	}
	cov.subjects, cov.paths = uniq(cov.subjects), uniq(cov.paths)
	return cov, nil
}

// restrict narrows a coverage to the given subject nodes: those subjects and
// the paths (of the coverage) rooted at one of them.
func restrict(in investigate.ValidationInput, cov coverage, nodes []string) coverage {
	keep := map[string]bool{}
	for _, n := range nodes {
		keep[n] = true
	}
	out := coverage{}
	for _, s := range cov.subjects {
		if keep[s] {
			out.subjects = append(out.subjects, s)
		}
	}
	if in.Traversal != nil {
		roots := map[string]string{}
		for _, p := range in.Traversal.Paths {
			if len(p.Nodes) > 0 {
				roots[p.ID] = p.Nodes[0]
			}
		}
		for _, p := range cov.paths {
			if keep[roots[p]] {
				out.paths = append(out.paths, p)
				out.subjects = append(out.subjects, roots[p])
			}
		}
	}
	out.subjects, out.paths = uniq(out.subjects), uniq(out.paths)
	return out
}

// nodesByKey indexes graph nodes by key (a Kubernetes object's key is its
// render object id).
func nodesByKey(g *domain.RelationshipGraph) map[string][]string {
	m := map[string][]string{}
	if g == nil {
		return m
	}
	for _, n := range g.Nodes {
		m[n.Key] = append(m[n.Key], n.ID)
	}
	return m
}

// scopeRenderedUnexplained is the scope rule of an item that is a rendered
// change nothing upstream explains (DESIGN.md §1.3 amendment): a validator
// passes it only by positively showing the change harmless, and fails it
// only with a concrete failure named.
const scopeRenderedUnexplained = "scope:rendered-unexplained"

// breaking reports whether an item is breaking in the sense of DESIGN.md §1.3
// (declared, computed-breaking or rendered-structural).
// Category and rendered-unexplained items never are: they are affected only
// through a validation that names a concrete failure.
func breaking(it domain.ChangeItem) bool {
	switch it.ScopeRule {
	case "scope:category", scopeRenderedUnexplained:
		return false
	case "scope:breaking", "scope:computed-breaking", "scope:rendered-structural":
		return true
	}
	return it.Breaking || it.ActionRequired
}

// envPairs returns the environment-scope render pairs of the product: a
// not-applicable pair (a target that does not deploy the product, e.g. an
// overlay of another application) is no render of it and is left out.
// complete reports whether there is at least one pair and every one is
// complete (status ok).
func envPairs(b *domain.EvidenceBundle) (pairs []domain.ManifestPair, complete bool) {
	for _, p := range b.Renders {
		if p.Status != domain.CollectionNotApplicable {
			pairs = append(pairs, p)
		}
	}
	complete = len(pairs) > 0
	for _, p := range pairs {
		if p.Status != domain.CollectionOK {
			complete = false
		}
	}
	return pairs, complete
}

// incompleteDetail names the environment render pairs that are not complete.
func incompleteDetail(pairs []domain.ManifestPair) string {
	if len(pairs) == 0 {
		return "the bundle has no environment render (renders disabled, or no render target of the product detected in the environment)"
	}
	var parts []string
	for _, p := range pairs {
		if p.Status != domain.CollectionOK {
			parts = append(parts, fmt.Sprintf("%s is %s (%s)", p.Target, p.Status, p.Reason))
		}
	}
	return "environment render incomplete — an incomplete render is an evidence gap, never \"no change\": " + strings.Join(parts, "; ")
}

var errNoStore = errors.New("no artifact store configured")

var timeZero time.Time

// read returns the verified bytes of a bundle artifact.
func (c config) read(ctx context.Context, b *domain.EvidenceBundle, id string) ([]byte, domain.ArtifactRef, error) {
	ref, ok := b.Manifest.Lookup(id)
	if !ok {
		return nil, ref, fmt.Errorf("artifact %s is not in the bundle manifest", id)
	}
	if c.store == nil {
		return nil, ref, errNoStore
	}
	rc, err := c.store.Open(ctx, ref.Digest)
	if err != nil {
		return nil, ref, fmt.Errorf("artifact %s (%s): %w", id, ref.Path, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, ref, fmt.Errorf("artifact %s (%s): %w", id, ref.Path, err)
	}
	if got := domain.Digest(data); got != ref.Digest {
		return nil, ref, fmt.Errorf("artifact %s (%s): digest %s, manifest says %s", id, ref.Path, got, ref.Digest)
	}
	return data, ref, nil
}

// artifactEvidence builds an evidence record citing (a locator in) an
// artifact; ContentDigest is the artifact digest, URI what a human opens.
func artifactEvidence(kind domain.EvidenceKind, a domain.ArtifactRef, locator, excerpt string, render *domain.RenderProvenance) domain.Evidence {
	e := domain.NewEvidence(kind, a.ID, a.URI, locator, excerpt, a.Digest, time.Time{})
	e.Render = render
	return e
}

// envRender is the render provenance of environment-scope evidence.
func envRender(object, path, after string) *domain.RenderProvenance {
	return &domain.RenderProvenance{Scope: domain.RenderEnvironment, Tool: "ri:render", Object: object, Path: path, After: after}
}

func uniq(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	m := map[string]bool{}
	var out []string
	for _, x := range xs {
		if x != "" && !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func uniqEvidence(xs []domain.EvidenceID) []domain.EvidenceID {
	if len(xs) == 0 {
		return nil
	}
	m := map[domain.EvidenceID]bool{}
	var out []domain.EvidenceID
	for _, x := range xs {
		if x != "" && !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
