// Package runtest is the test support of the run and orchestrate packages:
// a Deps over the REAL wave-1 packages (bundle, relations, store) with a
// fixed one-change upstream and in-memory fakes of the validator, classifier
// and review applier that the other lanes build. Not for production code.
package runtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/bundle"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/llm"
	"github.com/tdavison784/release-intelligence/internal/render"
	"github.com/tdavison784/release-intelligence/internal/synthesize"
)

var FixedNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

const notesURL = "https://github.com/acme/web/blob/main/notes.md"
const notesRaw = "https://raw.githubusercontent.com/acme/web/main/notes.md"
const notesBody = "# Notes\n## Breaking\n- --enable-thing is on by default\n"

// upstream is a fixed one-breaking-change upstream (the bundle lane's
// Upstream port), so the REAL collector, snapshot builder, assembler and
// relations packages run in these tests.
type upstream struct{}

func (upstream) Upgrade(context.Context, string, string, string) (*domain.UpgradeEdge, error) {
	e := domain.NewEvidence(domain.EvidenceDocument, "notes", notesURL+"#L3", "L3", "--enable-thing is on by default", domain.Digest([]byte(notesBody)), time.Time{})
	return &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "acme", Name: "Acme"},
		From:          domain.Version{Tag: "v1.0.0", Semver: "1.0.0"}, To: domain.Version{Tag: "v2.0.0", Semver: "2.0.0"},
		Sources: []domain.SourceStatus{{SourceID: "notes", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, State: domain.SourceOK}},
		Changes: []domain.Change{{ID: "c-breaking", Category: domain.CategoryConfiguration, Title: "flag on by default", Breaking: true,
			Subjects: []string{"--enable-thing"}, Evidence: []domain.EvidenceID{e.ID},
			Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.notes@v1", Rule: "marker:breaking", Confidence: domain.ConfidenceHigh}}},
		Evidence: []domain.Evidence{e},
	}, nil
}

func (upstream) RenderRelease(context.Context, *domain.UpgradeEdge, string) (*render.Pair, error) {
	return nil, nil
}

func (upstream) Fetch(_ context.Context, u string) ([]byte, string, error) {
	if u == notesRaw {
		return []byte(notesBody), "text/plain", nil
	}
	return nil, "", fmt.Errorf("fetch %s: 404", u)
}

// Validator counts calls and returns no records.
// Validator counts calls and returns no records.
type Validator struct {
	mu    sync.Mutex
	Calls int
}

func (f *Validator) Name() string                         { return "validate.fake@v1" }
func (f *Validator) Kind() domain.ValidationKind          { return domain.ValidationKind("static") }
func (f *Validator) Applies(domain.ChangeItemFamily) bool { return true }
func (f *Validator) Validate(context.Context, investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	f.mu.Lock()
	f.Calls++
	f.mu.Unlock()
	return &investigate.ValidationOutput{}, nil
}

// Classifier yields one insufficient-evidence finding per in-scope item.
// Classifier yields one insufficient-evidence finding per in-scope item.
type Classifier struct {
	mu    sync.Mutex
	Calls int
	Fail  int // transient failures before success
}

func (f *Classifier) Classify(_ context.Context, in investigate.ClassifyInput) (*investigate.Classification, error) {
	f.mu.Lock()
	f.Calls++
	n := f.Calls
	f.mu.Unlock()
	if n <= f.Fail {
		return nil, fmt.Errorf("classifier unavailable (transient)")
	}
	var fs []domain.InvestigationFinding
	items := append([]domain.ChangeItem(nil), in.Bundle.Change.Items...)
	items = append(items, in.Bundle.EnvironmentItems...)
	for _, it := range items {
		if !it.InScope {
			continue
		}
		var rcs []string
		for _, p := range in.Bundle.Renders {
			for _, c := range p.Changes {
				for _, x := range c.ChangeItems {
					if x == it.ID {
						rcs = append(rcs, c.ID)
					}
				}
			}
		}
		f := domain.InvestigationFinding{
			ChangeItem: it.ID, Class: domain.FindingInsufficientEvidence, Manifestation: it.Manifestation,
			Rule: "classify:test", Reasoning: "fake policy", Environment: in.Bundle.Snapshot.Environment,
			Evidence: it.Evidence, NextSteps: []string{"supply the values file"}, RenderedChanges: rcs,
			Gaps:       []domain.EvidenceGap{{Kind: domain.GapValidation, Subject: "none", Detail: "no decisive validation"}},
			Approval:   domain.ApprovalEligibility{RequiresHumanReview: true, Blockers: []string{"insufficient-evidence"}},
			Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "classify.policy@v1", Rule: "classify:test", Confidence: domain.ConfidenceHigh},
		}
		f.ID = domain.InvestigationFindingID(f.Environment, f.ChangeItem, f.Class, f.Rule)
		fs = append(fs, f)
	}
	return &investigate.Classification{Findings: fs, Decision: domain.PolicyDecision{
		Policy: "classify.policy@v1", Outcome: domain.PolicyHumanReview, Reasons: []string{"fake"}}}, nil
}

// Reviewer follows the ReviewApplier contract: only a human decides; a
// proxy decision is recorded and keeps the review pending.
type Reviewer struct{}

func (Reviewer) ApplyReview(inv *domain.Investigation, d investigate.ReviewDecision) error {
	if d.Reviewer == "" {
		return fmt.Errorf("%w: reviewer required", investigate.ErrRejected)
	}
	at := d.At
	if d.ReviewerKind == domain.ReviewerProxy {
		inv.AppendAudit(domain.AuditEvent{At: at, Stage: domain.StageHumanReview, Event: "decided", Actor: "proxy:" + d.Reviewer, Detail: string(d.Action) + " (proxy; not binding)"})
		return nil
	}
	switch d.Action {
	case investigate.ReviewApprove:
		inv.Review.State = domain.ReviewApproved
	case investigate.ReviewReject:
		inv.Review.State = domain.ReviewRejected
	case investigate.ReviewRequestMoreEvidence:
		inv.Review.State = domain.ReviewMoreEvidenceWanted
	default:
		return fmt.Errorf("%w: unknown action %q", investigate.ErrRejected, d.Action)
	}
	inv.Review.Reviewer, inv.Review.ReviewerKind, inv.Review.DecidedAt, inv.Review.Note = d.Reviewer, d.ReviewerKind, &at, d.Note
	inv.AppendAudit(domain.AuditEvent{At: at, Stage: domain.StageHumanReview, Event: "decided", Actor: "human:" + d.Reviewer, Detail: string(d.Action)})
	return inv.Validate()
}

// fixture bundles a Deps over real stores and wave-1 packages.
// Fixture is a Deps over real stores and wave-1 packages.
type Fixture struct {
	Deps  run.Deps
	St    *store.Store
	Val   *Validator
	Class *Classifier
	Repo  string
}

func New(t *testing.T) *Fixture {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "values.yaml"), []byte("replicaCount: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := run.NewDirWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	val, cls := &Validator{}, &Classifier{}
	return &Fixture{St: st, Val: val, Class: cls, Repo: repo, Deps: run.Deps{
		Artifacts: st, Records: st, Work: ws,
		Change:     bundle.NewChangeCollectorOver(upstream{}, st),
		Snapshot:   bundle.NewSnapshotBuilder(st),
		Assembler:  bundle.NewAssembler(st),
		Graph:      relations.NewBuilder(st),
		Exposure:   relations.NewExposure(),
		Traverser:  relations.NewTraverser(),
		Validators: func(*domain.ImpactReport) []investigate.Validator { return []investigate.Validator{val} },
		Classifier: cls, Reviewer: Reviewer{},
		Now: func() time.Time { return FixedNow },
	}}
}

// Request is a valid request over the fixture's environment repo.
func (f *Fixture) Request() investigate.Request {
	return investigate.Request{
		Product: "acme", From: "v1.0.0", To: "v2.0.0",
		Environment: investigate.EnvironmentInputs{Name: "shop-prod", Repo: f.Repo},
		Config: investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound,
			Validators: []string{"validate.fake@v1"}, Agent: investigate.AgentConfig{Driver: "scripted", PromptVersion: "investigate-agent/v1", Budget: investigate.DefaultAgentBudget}},
	}
}

// Human is a human review decision.
func Human(action investigate.ReviewAction) *investigate.ReviewDecision {
	return &investigate.ReviewDecision{Action: action, Reviewer: "alice", ReviewerKind: domain.ReviewerHuman, At: FixedNow, Note: "ok"}
}

// Synth is the synthesis port over internal/synthesize with a model client
// (normally a cached llm.Fake answering ValidationRequiredAnswer).
type Synth struct{ Client llm.Client }

func (s Synth) Synthesize(ctx context.Context, in run.SynthesisInput) (*domain.UpgradeJudgement, error) {
	return (&synthesize.Synthesizer{Client: s.Client}).Synthesize(ctx, synthesize.Input{
		Investigation: in.Investigation, Bundle: in.Bundle, Report: in.Report, Edge: in.Edge})
}

// ValidationRequiredAnswer answers a synthesis request with a
// validation-required judgement that passes the floor on any input: it
// requires the first candidate test (or comments on the first finding).
func ValidationRequiredAnswer(req llm.Request) (string, error) {
	msg := req.Messages[0].Content
	i := strings.Index(msg, "\n{")
	if i < 0 {
		return "", fmt.Errorf("no input document in the request")
	}
	var p synthesize.Precomputed
	if err := json.NewDecoder(strings.NewReader(msg[i+1:])).Decode(&p); err != nil {
		return "", err
	}
	var basis []string
	if len(p.Tests) > 0 {
		basis = append(basis, p.Tests[0].ID)
	} else if len(p.Findings) > 0 {
		basis = append(basis, p.Findings[0].ID)
	}
	a := synthesize.Answer{Recommendation: domain.RecommendValidationRequired,
		Headline: "Validation required before " + p.Upgrade.To.String() + " can be cleared in " + p.Upgrade.Environment + ".",
		Scope:    "the repository", Preconditions: []string{}, Rationale: "the classification leaves items undecided",
		Changes:       []synthesize.AnswerChange{{Kind: domain.StepValidate, Summary: "run the required tests", ChangeItems: []string{}}},
		Validation:    []synthesize.AnswerTest{{Test: "run the candidate test", Expected: "it passes", Basis: basis, ChangeItems: []string{}}},
		ResidualRisks: []synthesize.AnswerRisk{}, FindingComments: []synthesize.AnswerComment{}, InteractionComments: []synthesize.AnswerComment{},
		Verified: []synthesize.AnswerVerified{}, Inferred: []string{}, Unknown: []string{"the undecided items"}}
	b, err := json.Marshal(a)
	return string(b), err
}

// SynthClient is a cached fake model answering ValidationRequiredAnswer,
// with a fixed clock (replays are cache hits).
func SynthClient(t *testing.T) (*llm.Cache, *llm.Fake) {
	t.Helper()
	fake := &llm.Fake{Model: "claude-opus-5-5", Respond: ValidationRequiredAnswer}
	return &llm.Cache{Dir: t.TempDir(), Inner: fake, Clock: func() time.Time { return FixedNow }}, fake
}
