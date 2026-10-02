package knowledge

// CONTRACT NOTE: the exported API in this file is the contract between the
// learning-loop lanes (DESIGN.md §9). Change it only through the `contract`
// owner.

import (
	"context"
	"errors"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// --- errors ---------------------------------------------------------------------

var (
	// ErrNotFound: no record with that id.
	ErrNotFound = errors.New("knowledge: not found")
	// ErrConflict: a record with the same id but different content exists and
	// the kind is immutable (candidates, proposals, validations, decisions), or
	// a mutable kind changed a field other than its mutable ones.
	ErrConflict = errors.New("knowledge: conflicting record")
	// ErrOrphan: a record references a candidate (or item) the store does not
	// hold; candidates are written first.
	ErrOrphan = errors.New("knowledge: referenced record missing")
)

// --- the store ---------------------------------------------------------------------

// Store is the durable knowledge store (default implementation: committed
// JSON files under knowledge/, DESIGN.md §8). Writes are idempotent by
// content-derived id. Every record is validated (KnowledgeRecord.Validate,
// and ValidateFactBasis for facts) before it is written; an invalid record is
// never stored.
type Store interface {
	// Put writes one record. Re-putting an identical record is a no-op.
	// Review items may change only Status; facts only Status, Anchors and
	// Candidates; every other kind is immutable (ErrConflict).
	Put(ctx context.Context, rec domain.KnowledgeRecord) error
	// Get returns one record by id (ErrNotFound).
	Get(ctx context.Context, id string) (domain.KnowledgeRecord, error)
	// Load returns the records matching q, grouped by kind.
	Load(ctx context.Context, q Query) (*Snapshot, error)
}

// Query filters a Load. Empty fields match everything.
type Query struct {
	Product  domain.ProductID
	Releases []string
	// Kinds restricts the record kinds loaded (empty = all).
	Kinds []domain.RecordKind
	// FactStatus restricts facts (empty = all statuses).
	FactStatus []domain.FactStatus
	// MinVerification keeps only facts at or above the level
	// (domain.VerificationLevel.AtLeast); "" keeps all.
	MinVerification domain.VerificationLevel
}

// Snapshot is an in-memory view of (part of) the store: what consumers read.
// The applicability lane takes Facts from it and hands them to impact.Build;
// the dashboard, routing, dataset export and metrics read the rest.
type Snapshot struct {
	Candidates  []domain.SemanticCandidate
	Proposals   []domain.SemanticProposal
	Validations []domain.ValidationResult
	ReviewItems []domain.ReviewItem
	Decisions   []domain.ReviewDecision
	Facts       []domain.VerifiedFact
}

// --- proposals (semantic lane) -------------------------------------------------------

// Proposer is ONE model behind ONE provider. It is the only seam between the
// loop and model providers: Claude, GLM via an Anthropic-compatible gateway,
// or a typed "System One" model are all Proposers returning the same typed
// answer. A Proposer never sees eval expectations and never receives Helm
// values (only key paths).
type Proposer interface {
	// Provider names who serves the model ("anthropic", "zai", …).
	Provider() string
	// Model is the requested model id; the answering model and version come
	// back in the proposal's provenance.
	Model() string
	// Propose answers one task for one candidate. The result must pass
	// SemanticProposal.ValidateAgainst(req.Candidate). A transport failure,
	// an undecodable answer or a schema violation is an error (recorded as a
	// ProposalFailure by the caller), never an empty proposal; an honest
	// abstention is a proposal with Undetermined aspects.
	Propose(ctx context.Context, req ProposalRequest) (*domain.SemanticProposal, error)
}

// ProposalRequest is one task for one candidate.
type ProposalRequest struct {
	Candidate domain.SemanticCandidate
	Task      domain.ProposalTask
	// Context is bounded, deterministic artifact context the prompt may show
	// (key paths and schema paths of the target release, never values or
	// environment data).
	Context ProposalContext
	// KnownFacts are candidate facts for the duplicate task (id + statement).
	KnownFacts []FactSummary
}

// ProposalContext is release-level artifact context for a prompt.
type ProposalContext struct {
	ValuesKeys  []string // flattened Helm values key paths of the target release
	SchemaPaths []string // CRD schema paths of the target release ("Certificate: spec.privateKey.rotationPolicy")
	GVKs        []string // group/version/kind served by the target release
	Images      []string // image repositories of the target release
	Truncated   bool     // the lists were capped; the prompt says so
}

// FactSummary identifies a known fact for duplicate detection.
type FactSummary struct {
	ID        string
	Subject   string // Subject.Key()
	Statement string
}

// ProposalFailure records a proposal attempt that produced no proposal.
type ProposalFailure struct {
	CandidateID  string
	Task         domain.ProposalTask
	Provider     string
	Model        string
	PromptDigest string
	Reason       string
	At           time.Time
}

// --- validation (validate lane) -------------------------------------------------------

// Validator proves aspects of an assertion from ingested artifacts only (no
// fetches at validation time): helm-values, crd-schema, compatibility, image,
// rendered-diff, canonical-applicability (DESIGN.md §2.3).
type Validator interface {
	// Name is the producer string, component@vN (ValidationResult.Validator).
	Name() string
	// Validate returns zero or more results (zero: not applicable to this
	// assertion's family). Every result must pass ValidationResult.Validate.
	Validate(ctx context.Context, in ValidationInput) ([]domain.ValidationResult, error)
}

// ValidationInput is one assertion to check against the release artifacts.
type ValidationInput struct {
	Candidate domain.SemanticCandidate
	// ProposalID names the proposal whose assertion is checked ("" when the
	// assertion is a reviewer's correction or a canonical construction).
	ProposalID string
	Assertion  domain.SemanticAssertion
	// From and To are the ingested endpoint releases (snapshots: values,
	// CRDs, images, compatibility); Edge is the edge the candidate came from.
	From, To *domain.Release
	Edge     *domain.UpgradeEdge
	Now      time.Time
}

// --- routing, the queue, decisions (knowledge lane; dashboard consumes) -----------------

// RouteResult is what routing concluded for a candidate (DESIGN.md §6):
// either a deterministic fact (every aspect confirmed) or review items for
// the open aspects (possibly several question types), never both.
type RouteResult struct {
	Fact        *domain.VerifiedFact
	ReviewItems []domain.ReviewItem
	// Agreement is the per-aspect agreement among distinct models that
	// routing based its signals on.
	Agreement []AspectAgreement
}

// AspectAgreement summarises how the proposals of distinct models compare on
// one aspect.
type AspectAgreement struct {
	Aspect domain.Aspect
	// Groups maps an aspect digest to the proposal ids asserting it; one
	// group with ≥2 distinct models = agreement, ≥2 groups = disagreement.
	Groups       map[string][]string
	Undetermined []string // proposal ids that abstained on the aspect
}

// Queue is the review queue: what the dashboard reads and writes.
type Queue interface {
	// Inbox lists review items matching f with the G7 counts.
	Inbox(ctx context.Context, f InboxFilter) (*Inbox, error)
	// Item returns everything the item page shows (G8).
	Item(ctx context.Context, id string) (*ReviewContext, error)
	// Decide records decisions — one per item; several when submitted as one
	// bulk action (same BatchID) — updates item status, and mints or updates
	// facts for accept/correct (and adds anchors for duplicate decisions).
	// All decisions of a call are validated before any is written.
	Decide(ctx context.Context, ds []domain.ReviewDecision) ([]DecisionOutcome, error)
}

// InboxFilter holds the G7 filters. Empty fields match everything.
type InboxFilter struct {
	Product      domain.ProductID
	Release      string
	SubjectType  domain.SubjectFamily
	QuestionType domain.QuestionType
	Severity     domain.ImpactSeverity
	Confidence   domain.Confidence
	Model        string
	Disagreement *bool
	Source       string // upstream evidence URI substring
	Status       []domain.ReviewStatus
	Reviewer     string
	Limit        int
}

// Inbox is the dashboard's landing view.
type Inbox struct {
	Counts InboxCounts
	Items  []InboxRow
}

// InboxCounts are the G7 counters.
type InboxCounts struct {
	Pending                int
	ModelDisagreement      int
	NeedsSemanticMapping   int
	ApplicabilityQuestions int
	NeedsMoreEvidence      int
	Deferred               int
}

// InboxRow is one line of the inbox.
type InboxRow struct {
	Item         domain.ReviewItem
	Title        string // candidate title
	Disagreement bool
	Models       []string
}

// ReviewContext is everything one review decision needs (G8).
type ReviewContext struct {
	Item        domain.ReviewItem
	Candidate   domain.SemanticCandidate // product/release, upstream statement(s), upstream evidence
	Proposals   []domain.SemanticProposal
	Validations []domain.ValidationResult
	Agreement   []AspectAgreement
	// Related are earlier decisions and facts about the same subject key or
	// the same candidate members.
	RelatedDecisions []domain.ReviewDecision
	RelatedFacts     []domain.VerifiedFact
	// SuggestedExposedClass pre-fills the consequence form
	// (ConsequenceKind.ExposedClass of the proposed kind).
	SuggestedExposedClass domain.ImpactClass
	// Environment is the optional illustration recorded on the item.
	Environment *domain.EnvironmentContext
}

// DecisionOutcome reports what one decision produced.
type DecisionOutcome struct {
	Decision domain.ReviewDecision
	// Fact is the minted or updated fact (accept/correct/duplicate), nil
	// when the decision verified only some aspects and others remain open.
	Fact *domain.VerifiedFact
	// OpenAspects lists aspects that still need verification for the
	// candidate's fact.
	OpenAspects []domain.Aspect
	// FollowUps are review items created because aspects remain open.
	FollowUps []domain.ReviewItem
}

// --- metrics (knowledge lane; DESIGN.md §7) ---------------------------------------------

// LoopMetrics are the loop's measured outcomes (G4, G15, G24).
type LoopMetrics struct {
	Agreement   []AgreementMetric
	Models      []ModelMetric
	Review      ReviewMetrics
	Facts       FactMetrics
	GeneratedAt time.Time
}

// AgreementMetric is agreement per aspect and task across distinct models.
type AgreementMetric struct {
	Aspect     domain.Aspect
	Task       domain.ProposalTask
	Candidates int     // candidates with ≥2 models answering the aspect
	Agreement  float64 // share of those where all asserting models agree
	// Pairwise maps "modelA|modelB" to their agreement rate.
	Pairwise map[string]float64
}

// ModelMetric is model-to-human accuracy for one model (G15).
type ModelMetric struct {
	Provider, Model      string
	Proposals            int
	AcceptedAsIs         int
	AcceptedCorrected    int
	Rejected             int
	InsufficientEvidence int
	Abstentions          int
	// FalsePositive/FalseNegative per aspect vs final facts.
	FalsePositive map[domain.Aspect]int
	FalseNegative map[domain.Aspect]int
	// GroundedCitations is the share of citations the accepted fact also uses.
	GroundedCitations float64
}

// ReviewMetrics is review cost (G24). Batch (bulk) and individual decisions
// are reported separately.
type ReviewMetrics struct {
	Items              int
	ItemsPerRelease    map[string]int
	ItemsPerProduct    map[string]int
	ItemsPerQuestion   map[domain.QuestionType]int
	Individual         DecisionStats
	Batch              DecisionStats
	RepeatPatternRate  float64
	AutoValidationRate float64
}

// DecisionStats are the decision rates and timing of one decision mode.
type DecisionStats struct {
	Decisions                   int
	Accept, Correct, Reject     int
	NeedMoreEvidence, Defer     int
	MedianDuration, P90Duration time.Duration
	Batches                     int // batch mode only: distinct BatchIDs
}

// FactMetrics count the knowledge itself.
type FactMetrics struct {
	Active         int
	ByLevel        map[domain.VerificationLevel]int
	ByFamily       map[domain.SubjectFamily]int
	Retracted      int
	AnchorsPerFact float64 // restatements remembered per fact
}
