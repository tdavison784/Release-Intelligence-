package investigate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// --- requests and configuration ----------------------------------------------------------

// Request is one investigation request: a dependency upgrade against one
// environment. It is the workflow input.
type Request struct {
	Product     string            `json:"product"` // products/<id>.yaml id
	From        string            `json:"from"`    // version tag as the user gives it
	To          string            `json:"to"`
	Environment EnvironmentInputs `json:"environment"`
	Config      Config            `json:"config"`
}

// EnvironmentInputs names the environment inputs (the env.Inputs the
// snapshot builder loads, plus the environment's name and the opt-in live
// observation). Paths are as supplied by the user.
type EnvironmentInputs struct {
	Name              string   `json:"name"` // e.g. "prod-eu"
	Repo              string   `json:"repo,omitempty"`
	ValuesFiles       []string `json:"valuesFiles,omitempty"`
	Manifests         []string `json:"manifests,omitempty"`
	CRDs              []string `json:"crds,omitempty"`
	Images            []string `json:"images,omitempty"`
	Inventory         string   `json:"inventory,omitempty"`
	KubernetesVersion string   `json:"kubernetesVersion,omitempty"`
	// Live enables the read-only live-cluster observation (off by default;
	// DESIGN.md §1.2).
	Live *LiveOptions `json:"live,omitempty"`
}

// LiveOptions scopes a read-only live observation.
type LiveOptions struct {
	Context    string   `json:"context"`
	Namespaces []string `json:"namespaces,omitempty"`
	Kinds      []string `json:"kinds,omitempty"` // GVKs to list; empty = the graph builder's default set
}

// Config is everything besides the inputs that determines the outcome. Its
// digest is part of the investigation id, so a configuration change is a
// new investigation, never a silent re-run.
type Config struct {
	Policy     string                `json:"policy"` // classification policy version, e.g. "classify.policy@v1"
	Bound      domain.TraversalBound `json:"bound"`
	Validators []string              `json:"validators"` // enabled validators, component@vN, sorted
	Agent      AgentConfig           `json:"agent"`
	// Render enables environment-scope helm/kustomize renders in the bundle.
	Render      bool   `json:"render"`
	KubeVersion string `json:"kubeVersion,omitempty"` // render --kube-version
	// AutoApproval enacts automated approval for eligible investigations. It
	// stays false until the pre-registered gates allow it (DESIGN.md §3.4,
	// eval/investigations/gates.yaml); eligibility is computed either way.
	AutoApproval bool `json:"autoApproval,omitempty"`
}

// Digest pins the configuration (canonical JSON).
func (c Config) Digest() string { return digestJSON(c) }

// AgentConfig configures the investigation agent.
type AgentConfig struct {
	Enabled       bool               `json:"enabled"`
	Driver        string             `json:"driver"` // "claude-cli" | "exchange" | "replay" | "scripted"
	Model         string             `json:"model,omitempty"`
	PromptVersion string             `json:"promptVersion"` // e.g. "investigate-agent/v1"
	Budget        domain.AgentBudget `json:"budget"`
}

// DefaultAgentBudget is the first-milestone budget (DESIGN.md §4.3).
var DefaultAgentBudget = domain.AgentBudget{MaxTurns: 40, MaxToolCalls: 80, MaxTokens: 600_000, MaxReadBytes: 4 << 20, MaxSeconds: 1200}

// ID returns the investigation id the request will produce for a bundle.
func (r Request) ID(bundleID string) string {
	return domain.InvestigationID(bundleID, r.Config.Digest())
}

// ChangeKey is the idempotency key of change-evidence collection: the same
// product, endpoints and catalog definition collect the same record.
func ChangeKey(product, from, to, definitionDigest string) string {
	return "change:" + domain.ShortHash(product, from, to, definitionDigest)
}

// SnapshotKey is the idempotency key of snapshot building. It covers the
// inputs as named; the snapshot itself is identified by the digests of what
// was read (EnvironmentSnapshot.ID), so a changed file yields a new snapshot.
func SnapshotKey(in EnvironmentInputs) string { return "snapshot:" + domain.ShortHash(digestJSON(in)) }

// --- artifact and record storage ---------------------------------------------------------

// ArtifactSpec describes bytes about to be stored; the store computes the
// digest, size and id.
type ArtifactSpec struct {
	Kind        domain.ArtifactKind   `json:"kind"`
	Path        string                `json:"path"` // logical path, "<origin prefix>..."
	Origin      domain.ArtifactOrigin `json:"origin"`
	MediaType   string                `json:"mediaType"`
	URI         string                `json:"uri"`
	Producer    string                `json:"producer"`
	Redacted    bool                  `json:"redacted,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	ChangeItems []string              `json:"changeItems,omitempty"`
}

// ArtifactStore is the content-addressed artifact store
// (<state>/investigations/cas/sha256/<hex>). Put is idempotent: the same
// bytes are stored once, and the same spec + bytes yield the same ref.
type ArtifactStore interface {
	Put(ctx context.Context, spec ArtifactSpec, content []byte) (domain.ArtifactRef, error)
	// Open returns the bytes of a digest; ErrNotFound when absent. The
	// store verifies the digest of what it returns.
	Open(ctx context.Context, digest string) (io.ReadCloser, error)
}

// RecordStore persists bundles and investigation records
// (<state>/investigations/{bundles,runs}/<id>.json). Writes validate first
// and are atomic; a bundle is immutable (writing a different bundle under an
// existing id is an error); an investigation is rewritten as it progresses,
// but its audit trail may only grow.
type RecordStore interface {
	PutBundle(ctx context.Context, b *domain.EvidenceBundle) error
	GetBundle(ctx context.Context, id string) (*domain.EvidenceBundle, error)
	PutInvestigation(ctx context.Context, inv *domain.Investigation) error
	GetInvestigation(ctx context.Context, id string) (*domain.Investigation, error)
}

// --- stage ports (one orchestration activity each; DESIGN.md §5) -------------------------

// ChangeRequest asks for the change evidence of one upgrade.
type ChangeRequest struct {
	Product string `json:"product"`
	From    string `json:"from"`
	To      string `json:"to"`
	Render  bool   `json:"render"` // also produce release-scope renders
}

// ChangeResult is the collected change evidence: the record, the artifacts
// it references (already in the ArtifactStore) and the upstream evidence
// pool its ids resolve in.
type ChangeResult struct {
	Record    domain.ChangeEvidenceRecord `json:"record"`
	Artifacts []domain.ArtifactRef        `json:"artifacts"`
	Evidence  []domain.Evidence           `json:"evidence"`
}

// ChangeCollector collects change evidence (MISSION §A1; lane bundle). It
// builds on app.Upgrade (the UpgradeEdge) and adds what the edge lacks; a
// source it cannot collect is recorded with a non-ok status, not an error.
type ChangeCollector interface {
	CollectChange(ctx context.Context, req ChangeRequest) (*ChangeResult, error)
}

// SnapshotResult is the collected environment evidence.
type SnapshotResult struct {
	Snapshot  domain.EnvironmentSnapshot `json:"snapshot"`
	Artifacts []domain.ArtifactRef       `json:"artifacts"`
	Evidence  []domain.Evidence          `json:"evidence"`
}

// SnapshotBuilder builds the environment snapshot (MISSION §A2; lane
// bundle). It builds on env.Load; an input it cannot read is recorded with a
// non-ok status, not an error.
type SnapshotBuilder interface {
	BuildSnapshot(ctx context.Context, in EnvironmentInputs) (*SnapshotResult, error)
}

// RenderResult is the environment-scope render evidence: one ManifestPair
// per render target the environment deploys the product with, its rendered
// changes correlated to the change items, the stored renders/diffs, and the
// rendered-diff evidence the changes cite.
type RenderResult struct {
	Pairs     []domain.ManifestPair `json:"pairs"`
	Artifacts []domain.ArtifactRef  `json:"artifacts"`
	Evidence  []domain.Evidence     `json:"evidence"`
	// Notes say what target detection skipped and why (a values file not
	// naming the product, renders disabled, no deployment found), so "no
	// pairs" is never silent.
	// CONTRACT-CHANGE(renderdiff): additive; the port had no place for them.
	Notes []string `json:"notes,omitempty"`
	// Items are the environment-scope change items anchored on rendered
	// changes nothing else explains (DESIGN §1.3 amendment:
	// scope:rendered-unexplained, scope:routine-self-artifact); the assembler
	// puts them into EvidenceBundle.EnvironmentItems. Sorted by id.
	// CONTRACT-CHANGE(renderdiff): additive.
	Items []domain.ChangeItem `json:"items,omitempty"`
}

// EnvironmentRenderer renders the product's from and to releases with the
// environment's own configuration and diffs them (lane renderdiff; the
// CENTERPIECE evidence). It builds on internal/render (DetectTargets,
// RenderPair, Diff, Correlate, chart coverage): a target it cannot render
// completely is a pair with a non-ok status and the render failure reason,
// never an empty diff.
type EnvironmentRenderer interface {
	RenderEnvironment(ctx context.Context, change *ChangeResult, snap *SnapshotResult, cfg Config) (*RenderResult, error)
}

// BundleAssembler joins change evidence, environment evidence and the
// environment renders into the immutable bundle (lane bundle).
type BundleAssembler interface {
	Assemble(ctx context.Context, change *ChangeResult, snap *SnapshotResult, renders *RenderResult) (*domain.EvidenceBundle, error)
}

// GraphBuilder builds the lightweight relationship graph of a bundle
// deterministically from the rendered objects and environment files
// (MISSION §A3, secondary context; lane relations). Stored as a graph artifact.
type GraphBuilder interface {
	BuildGraph(ctx context.Context, b *domain.EvidenceBundle) (*domain.RelationshipGraph, error)
}

// ExposureMapper maps the in-scope change items to their exposure roots:
// first the objects of the environment rendered changes correlated to each
// item, then the graph nodes the deterministic join's matches name (lane
// relations). The report is the domain.ImpactReport of the same upgrade and
// environment.
type ExposureMapper interface {
	Exposure(ctx context.Context, b *domain.EvidenceBundle, g *domain.RelationshipGraph, report *domain.ImpactReport) ([]domain.ExposureRoot, error)
}

// Traverser runs the bounded, deterministic DFS (lane relations). Same
// graph, roots and bound ⇒ byte-identical result.
type Traverser interface {
	Traverse(ctx context.Context, g *domain.RelationshipGraph, roots []domain.ExposureRoot, bound domain.TraversalBound) (*domain.TraversalResult, error)
}

// ValidationInput is what a validator gets: the bundle, the graph and
// traversal, the change item and the subjects/paths to check, optionally
// the agent request that asked for it.
type ValidationInput struct {
	Bundle          *domain.EvidenceBundle    `json:"bundle"`
	Graph           *domain.RelationshipGraph `json:"graph"`
	Traversal       *domain.TraversalResult   `json:"traversal"`
	Report          *domain.ImpactReport      `json:"report,omitempty"`
	ChangeItem      string                    `json:"changeItem"`
	Subjects        []string                  `json:"subjects,omitempty"`
	Paths           []string                  `json:"paths,omitempty"`
	RenderedChanges []string                  `json:"renderedChanges,omitempty"`
	Request         *domain.ValidationRequest `json:"request,omitempty"`
}

// ValidationOutput is what a validator produced. Outputs are already in the
// ArtifactStore; Evidence are records its ValidationRecords cite.
type ValidationOutput struct {
	Records   []domain.ValidationRecord `json:"records"`
	Artifacts []domain.ArtifactRef      `json:"artifacts,omitempty"`
	Evidence  []domain.Evidence         `json:"evidence,omitempty"`
}

// Validator is one deterministic (or observed) validator (MISSION §A5;
// lane validate). It never calls a model. When it cannot decide it returns
// a not-run or error record with the reason — never a pass.
type Validator interface {
	Name() string // component@vN
	Kind() domain.ValidationKind
	// Applies reports whether the validator can test this change family.
	Applies(family domain.ChangeItemFamily) bool
	Validate(ctx context.Context, in ValidationInput) (*ValidationOutput, error)
}

// InvestigateInput is what the agent stage gets.
type InvestigateInput struct {
	Bundle    *domain.EvidenceBundle    `json:"bundle"`
	Graph     *domain.RelationshipGraph `json:"graph"`
	Traversal *domain.TraversalResult   `json:"traversal"`
	Report    *domain.ImpactReport      `json:"report,omitempty"`
	// Validations already run (the deterministic pass before the agent).
	Validations []domain.ValidationRecord `json:"validations,omitempty"`
	Config      AgentConfig               `json:"config"`
}

// AgentOutcome is what the agent stage recorded. Validations requested by
// the agent and run during the session are included (produced by
// validators, not the agent).
type AgentOutcome struct {
	Run                domain.AgentRun            `json:"run"`
	Hypotheses         []domain.Hypothesis        `json:"hypotheses,omitempty"`
	Assertions         []domain.AgentAssertion    `json:"assertions,omitempty"`
	EvidenceRequests   []domain.EvidenceRequest   `json:"evidenceRequests,omitempty"`
	ValidationRequests []domain.ValidationRequest `json:"validationRequests,omitempty"`
	Validations        []domain.ValidationRecord  `json:"validations,omitempty"`
	Artifacts          []domain.ArtifactRef       `json:"artifacts,omitempty"`
	Evidence           []domain.Evidence          `json:"evidence,omitempty"`
	// ProposedNodes are the placeholder nodes the agent's proposed edges
	// need (an end the graph does not have, named by kind and key), so the
	// re-traversal over the graph plus the proposed edges (DESIGN.md §5.1)
	// can build them; agent.MergeGraph does.
	// CONTRACT-CHANGE(agent): additive; a node id alone cannot be turned
	// back into the kind and key a placeholder node needs.
	ProposedNodes []domain.GraphNode `json:"proposedNodes,omitempty"`
}

// Investigator runs the agent stage (MISSION §A4; lane agent): it serves
// Tools to a model through an AgentDriver within the budget. A budget
// exhaustion or driver failure is an outcome (AgentRun.Status), not an
// error; errors are for infrastructure failures.
type Investigator interface {
	Investigate(ctx context.Context, in InvestigateInput) (*AgentOutcome, error)
}

// ClassifyInput is everything the policy reads.
type ClassifyInput struct {
	Bundle    *domain.EvidenceBundle  `json:"bundle"`
	Traversal *domain.TraversalResult `json:"traversal"`
	Report    *domain.ImpactReport    `json:"report,omitempty"`
	Agent     *AgentOutcome           `json:"agent,omitempty"`
	// Validations from every validator run (deterministic pass and agent requests).
	Validations []domain.ValidationRecord `json:"validations"`
	Config      Config                    `json:"config"`
}

// Classification is the policy's output.
type Classification struct {
	Findings []domain.InvestigationFinding `json:"findings"`
	Decision domain.PolicyDecision         `json:"decision"`
}

// Classifier is the deterministic §3 policy (lane classify). Pure: same
// input ⇒ identical output; it never reads model confidence or agreement.
type Classifier interface {
	Classify(ctx context.Context, in ClassifyInput) (*Classification, error)
}

// ReviewAction is a human review decision (the orchestration signal).
type ReviewAction string

const (
	ReviewApprove             ReviewAction = "approve"
	ReviewReject              ReviewAction = "reject"
	ReviewRequestMoreEvidence ReviewAction = "request-more-evidence"
)

// ReviewDecision is the payload of the human-review signal.
type ReviewDecision struct {
	Action       ReviewAction        `json:"action"`
	Reviewer     string              `json:"reviewer"`
	ReviewerKind domain.ReviewerKind `json:"reviewerKind"`
	Note         string              `json:"note,omitempty"`
	At           time.Time           `json:"at"`
	// ProxyProvenance is required for a proxy reviewer, whose decision is
	// recorded but can never approve.
	ProxyProvenance *domain.Provenance `json:"proxyProvenance,omitempty"`
	// Requests names what more evidence is wanted (request-more-evidence).
	Requests []string `json:"requests,omitempty"`
}

// ReviewApplier applies a review decision to the run record (lane classify):
// sets Review, appends the audit event, and validates. Pure.
type ReviewApplier interface {
	ApplyReview(inv *domain.Investigation, d ReviewDecision) error
}

// --- the agent's tool surface (DESIGN.md §4) ----------------------------------------------

// Tool names as exposed over MCP (`ri investigate mcp`). The diff tools come
// first: the rendered diff is the agent's primary evidence.
const (
	ToolGetChangeItem     = "get_change_item"
	ToolListRenders       = "list_renders"
	ToolQueryDiff         = "query_diff"
	ToolGetRenderedObject = "get_rendered_object"
	ToolSearchManifest    = "search_manifest"
	ToolReadArtifact      = "read_artifact"
	ToolExpandGraph       = "expand_graph"
	ToolListPaths         = "list_paths"
	ToolRequestEvidence   = "request_evidence"
	ToolRequestValidation = "request_validation"
	ToolRecordHypothesis  = "record_hypothesis"
	ToolRecordAssertion   = "record_assertion"
	ToolSubmitFindings    = "submit_findings"
)

// ToolNames lists every tool in a stable order.
var ToolNames = []string{
	ToolGetChangeItem, ToolListRenders, ToolQueryDiff, ToolGetRenderedObject,
	ToolSearchManifest, ToolReadArtifact, ToolExpandGraph, ToolListPaths, ToolRequestEvidence,
	ToolRequestValidation, ToolRecordHypothesis, ToolRecordAssertion, ToolSubmitFindings,
}

// ChangeItemDetail is everything upstream says about one change item, next
// to what the renders show for it — so the agent can name the setting that
// decides the item (for a controller-behaviour item: the flag, values key,
// field or annotation that pins the old behaviour).
type ChangeItemDetail struct {
	Item domain.ChangeItem `json:"item"`
	// Change is the referenced UpgradeEdge change (nil for compatibility and
	// rendered-anchored items): title, detail (the upstream text), category,
	// subjects, references.
	Change *domain.Change `json:"change,omitempty"`
	// Compatibility is the referenced compatibility change, for platform items.
	Compatibility *domain.CompatibilityChange `json:"compatibility,omitempty"`
	// Evidence are the upstream records (URI, locator, verbatim excerpt) of
	// the item; Sources the artifacts holding the full upstream documents
	// (read them with read_artifact).
	Evidence       []domain.Evidence    `json:"evidence"`
	Sources        []domain.ArtifactRef `json:"sources,omitempty"`
	MigrationNotes []domain.Evidence    `json:"migrationNotes,omitempty"`
	// RenderedChanges are the release- and environment-scope rendered
	// changes correlated to the item.
	RenderedChanges []domain.RenderedChange `json:"renderedChanges,omitempty"`
	// ImpactFindings are the deterministic join's findings for the change.
	ImpactFindings []domain.ImpactFindingRef `json:"impactFindings,omitempty"`
}

// RenderSummary describes one render pair of the bundle.
type RenderSummary struct {
	Scope        domain.RenderScope      `json:"scope"`
	Target       string                  `json:"target"`
	Status       domain.CollectionStatus `json:"status"`
	Reason       string                  `json:"reason,omitempty"`
	Changes      int                     `json:"changes"`
	Uncorrelated int                     `json:"uncorrelated"`
	ByClass      map[string]int          `json:"byClass,omitempty"`
}

// DiffQuery filters rendered changes. Every set field must match; list
// fields match any element.
type DiffQuery struct {
	Scope        domain.RenderScope `json:"scope,omitempty"` // default environment
	Target       string             `json:"target,omitempty"`
	ChangeItem   string             `json:"changeItem,omitempty"`
	Uncorrelated bool               `json:"uncorrelated,omitempty"`
	Classes      []string           `json:"classes,omitempty"`    // render change classes
	Kinds        []string           `json:"kinds,omitempty"`      // object kinds
	Namespaces   []string           `json:"namespaces,omitempty"` // "-" = cluster-scoped
	PathPrefix   string             `json:"pathPrefix,omitempty"`
	Limit        int                `json:"limit,omitempty"` // default 50, max 200
	// After continues a bounded result: the id of the last rendered change
	// of the previous page (results are in a fixed order, so pages never
	// overlap or skip).
	// CONTRACT-CHANGE(renderdiff): additive; "bounded result sizes with
	// continuation" (brief) needs a cursor.
	After string `json:"after,omitempty"`
}

// ObjectRequest reads one rendered object (or a field of it) from one side
// of a render pair. Sensitive values (Secret data) are redacted.
type ObjectRequest struct {
	Target string `json:"target"`
	Side   string `json:"side"`           // "from" | "to"
	Object string `json:"object"`         // <group>/<version>/<kind>/<namespace>/<name>
	Path   string `json:"path,omitempty"` // field path; "" = whole object
}

// ObjectResult is the requested object or field as YAML, with the render
// artifact it came from (cite it with Citation{Artifact, Locator: path, Quote}).
type ObjectResult struct {
	Artifact domain.ArtifactRef `json:"artifact"`
	YAML     string             `json:"yaml"`
	Found    bool               `json:"found"` // false: the object/path is absent on that side (a fact, not an error)
}

// SearchQuery searches the manifest (bundle + run artifacts). Terms match
// tags, paths and kinds; Grep, when set, is an RE2 pattern matched against
// artifact text (bounded; redacted artifacts are searched in redacted form).
type SearchQuery struct {
	Terms      []string              `json:"terms,omitempty"`
	Kinds      []domain.ArtifactKind `json:"kinds,omitempty"`
	ChangeItem string                `json:"changeItem,omitempty"`
	Grep       string                `json:"grep,omitempty"`
	Limit      int                   `json:"limit,omitempty"` // default 20, max 100
}

// SearchHit is one result: the ref and, for grep, matching lines.
type SearchHit struct {
	Artifact domain.ArtifactRef `json:"artifact"`
	Lines    []LineMatch        `json:"lines,omitempty"`
}

// LineMatch is one matching line.
type LineMatch struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// ReadRequest reads a window of an artifact's text. Every byte returned
// counts against AgentBudget.MaxReadBytes.
type ReadRequest struct {
	Artifact string `json:"artifact"`           // artifact id
	FromLine int    `json:"fromLine"`           // 1-based, inclusive
	ToLine   int    `json:"toLine"`             // inclusive; window max 400 lines / 64 KiB
	YAMLPath string `json:"yamlPath,omitempty"` // alternative to lines: one YAML document node
}

// ReadResult is the returned window.
type ReadResult struct {
	Artifact   domain.ArtifactRef `json:"artifact"`
	FromLine   int                `json:"fromLine"`
	ToLine     int                `json:"toLine"`
	TotalLines int                `json:"totalLines"`
	Text       string             `json:"text"`
	Truncated  bool               `json:"truncated,omitempty"`
}

// ExpandRequest returns the neighbourhood of a node.
type ExpandRequest struct {
	Node      string            `json:"node"`
	Depth     int               `json:"depth"`     // max 3
	Direction string            `json:"direction"` // "out" (consumers) | "in" (providers) | "both"
	EdgeKinds []domain.EdgeKind `json:"edgeKinds,omitempty"`
}

// Subgraph is a node neighbourhood.
type Subgraph struct {
	Nodes []domain.GraphNode `json:"nodes"`
	Edges []domain.GraphEdge `json:"edges"`
}

// PathQuery lists impact paths.
type PathQuery struct {
	ChangeItem string `json:"changeItem,omitempty"`
	Unresolved bool   `json:"unresolved,omitempty"` // only paths with an inferred/missing edge
}

// Citation is how the agent cites: an artifact, a locator and a verbatim
// quote. The tool layer resolves the artifact, checks the quote occurs in
// its bytes (whitespace-normalised) and turns it into a domain.Evidence; an
// unknown artifact or a quote that does not occur rejects the whole call.
type Citation struct {
	Artifact string `json:"artifact"`
	Locator  string `json:"locator,omitempty"` // "L10-L12" or a YAML path
	Quote    string `json:"quote"`
}

// HypothesisInput records a hypothesis.
type HypothesisInput struct {
	ChangeItem string     `json:"changeItem"`
	Statement  string     `json:"statement"`
	Subjects   []string   `json:"subjects,omitempty"`
	Citations  []Citation `json:"citations"`
}

// AssertionInput records an assertion. ProposedEdge, when set, adds an
// inferred edge to the run's graph view (never established).
type AssertionInput struct {
	ChangeItem string                 `json:"changeItem"`
	Stance     domain.AssertionStance `json:"stance"`
	Statement  string                 `json:"statement"`
	Subjects   []string               `json:"subjects,omitempty"`
	Paths      []string               `json:"paths,omitempty"`
	// RenderedChanges cites rendered changes by id (in addition to the
	// quote citations, which still need at least one entry).
	RenderedChanges []string      `json:"renderedChanges,omitempty"`
	Citations       []Citation    `json:"citations"`
	Hypothesis      string        `json:"hypothesis,omitempty"`
	ProposedEdge    *ProposedEdge `json:"proposedEdge,omitempty"`
}

// ProposedEdge is a relationship the agent believes exists.
type ProposedEdge struct {
	Kind   domain.EdgeKind `json:"kind"`
	From   string          `json:"from"` // node id, or "<kind>:<key>" for a node not yet in the graph
	To     string          `json:"to"`
	Detail string          `json:"detail,omitempty"`
	Reason string          `json:"reason"`
}

// EvidenceRequestInput asks for more evidence.
type EvidenceRequestInput struct {
	ChangeItem string                     `json:"changeItem,omitempty"`
	Kind       domain.EvidenceRequestKind `json:"kind"`
	Target     string                     `json:"target"`
	Reason     string                     `json:"reason"`
}

// ValidationRequestInput asks a deterministic validator to test something.
type ValidationRequestInput struct {
	ChangeItem      string                `json:"changeItem"`
	Kind            domain.ValidationKind `json:"kind"`
	Subjects        []string              `json:"subjects,omitempty"`
	Paths           []string              `json:"paths,omitempty"`
	RenderedChanges []string              `json:"renderedChanges,omitempty"`
	Inputs          []string              `json:"inputs,omitempty"`
	Hypothesis      string                `json:"hypothesis,omitempty"`
}

// ItemSummary is the agent's closing summary for one change item. It is
// recorded as an `explains` assertion; it never sets a class.
type ItemSummary struct {
	ChangeItem string     `json:"changeItem"`
	Summary    string     `json:"summary"`
	Citations  []Citation `json:"citations"`
}

// SubmitInput ends the agent run.
type SubmitInput struct {
	Items []ItemSummary `json:"items"`
}

// Tools is the agent's tool surface, scoped to one investigation (one
// bundle, its renders and diffs, its graph and traversal, the run's
// records). Every call is recorded in the transcript with its arguments and
// result digest, counted against the budget, and validated: unknown ids,
// uncited claims or quotes that do not occur are rejected with ErrRejected
// and nothing is recorded.
type Tools interface {
	GetChangeItem(ctx context.Context, id string) (*ChangeItemDetail, error)
	ListRenders(ctx context.Context) ([]RenderSummary, error)
	QueryDiff(ctx context.Context, q DiffQuery) ([]domain.RenderedChange, error)
	GetRenderedObject(ctx context.Context, r ObjectRequest) (*ObjectResult, error)
	SearchManifest(ctx context.Context, q SearchQuery) ([]SearchHit, error)
	ReadArtifact(ctx context.Context, r ReadRequest) (*ReadResult, error)
	ExpandGraph(ctx context.Context, r ExpandRequest) (*Subgraph, error)
	ListPaths(ctx context.Context, q PathQuery) ([]domain.ImpactPath, error)
	RequestEvidence(ctx context.Context, in EvidenceRequestInput) (*domain.EvidenceRequest, error)
	// RequestValidation runs the validator synchronously when it applies and
	// the budget allows; the request carries the status and the result id.
	RequestValidation(ctx context.Context, in ValidationRequestInput) (*domain.ValidationRequest, *domain.ValidationRecord, error)
	RecordHypothesis(ctx context.Context, in HypothesisInput) (*domain.Hypothesis, error)
	RecordAssertion(ctx context.Context, in AssertionInput) (*domain.AgentAssertion, error)
	SubmitFindings(ctx context.Context, in SubmitInput) error
}

// AgentSession is what a driver needs to run one agent session.
type AgentSession struct {
	SystemPrompt  string             `json:"systemPrompt"`
	Task          string             `json:"task"` // the initial user message (change items, environment, roots)
	PromptVersion string             `json:"promptVersion"`
	PromptDigest  string             `json:"promptDigest"` // digest of system prompt + task + tool schemas
	Model         string             `json:"model"`
	Budget        domain.AgentBudget `json:"budget"`
	// MCPConfig is the path of the MCP config that starts
	// `ri investigate mcp` for this investigation (claude-cli driver).
	MCPConfig string `json:"mcpConfig,omitempty"`
}

// DriverResult is what the driver reports about the model side.
type DriverResult struct {
	Model        string `json:"model"`        // as reported by the provider envelope
	ModelVersion string `json:"modelVersion"` // as reported, never assumed
	Provider     string `json:"provider"`
	CallID       string `json:"callId"` // session id from the envelope
	Turns        int    `json:"turns"`
	Tokens       int    `json:"tokens"`
	// Stop says why the session ended: "submitted", "budget:<which>", "error:<detail>".
	Stop string `json:"stop"`
}

// AgentDriver runs a model against the Tools (lane agent). Drivers:
// claude-cli (`claude -p --mcp-config … --output-format json`), exchange
// (the -llm-exchange file protocol, one tool round per exchange file),
// replay (re-issues a recorded transcript's tool calls; used for
// reproducibility and tests) and scripted (tests).
type AgentDriver interface {
	Run(ctx context.Context, s AgentSession, tools Tools) (*DriverResult, error)
}

// --- errors (retry classes; DESIGN.md §5.2) -----------------------------------------------

// ErrNotFound reports an unknown id or digest.
var ErrNotFound = errors.New("investigate: not found")

// ErrRejected reports a tool call the surface refused (unknown id, uncited
// claim, quote not in the artifact, out of scope). It is returned to the
// model as a tool error, never retried by the orchestrator.
var ErrRejected = errors.New("investigate: rejected")

// ErrBudgetExhausted reports a spent agent budget.
var ErrBudgetExhausted = errors.New("investigate: budget exhausted")

// ErrorClass tells the orchestrator whether retrying can help.
type ErrorClass string

const (
	// Transient: a network/registry/rate-limit/timeout failure; retry with backoff.
	Transient ErrorClass = "transient"
	// Permanent: invalid input, an invariant violation, a missing binary,
	// an unknown product; retrying cannot help.
	Permanent ErrorClass = "permanent"
)

// StageError wraps a stage failure with its retry class.
type StageError struct {
	Stage domain.Stage
	Class ErrorClass
	Err   error
}

func (e *StageError) Error() string { return fmt.Sprintf("%s (%s): %v", e.Stage, e.Class, e.Err) }
func (e *StageError) Unwrap() error { return e.Err }

// TransientError marks err as retryable.
func TransientError(stage domain.Stage, err error) error {
	return &StageError{Stage: stage, Class: Transient, Err: err}
}

// PermanentError marks err as not retryable.
func PermanentError(stage domain.Stage, err error) error {
	return &StageError{Stage: stage, Class: Permanent, Err: err}
}

// ClassOf returns the retry class of an error. Unclassified errors are
// transient (the orchestrator's retry policy bounds the attempts), except
// context cancellation, ErrNotFound and ErrRejected, which are permanent.
func ClassOf(err error) ErrorClass {
	var se *StageError
	switch {
	case errors.As(err, &se):
		return se.Class
	case errors.Is(err, context.Canceled), errors.Is(err, ErrNotFound), errors.Is(err, ErrRejected):
		return Permanent
	}
	return Transient
}

// --- helpers ------------------------------------------------------------------------------

func digestJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("investigate: digest: %v", err))
	}
	return domain.Digest(b)
}

// normalizeQuote collapses whitespace runs, the comparison the tool layer
// uses to check a citation's quote occurs in an artifact.
func normalizeQuote(s string) string { return strings.Join(strings.Fields(s), " ") }

// QuoteOccurs reports whether quote occurs in text, whitespace-normalised.
// Exported so every Tools implementation checks citations the same way.
func QuoteOccurs(text, quote string) bool {
	q := normalizeQuote(quote)
	return q != "" && strings.Contains(normalizeQuote(text), q)
}
