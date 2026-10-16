package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/bundle"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/render"
)

var fixedNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

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

// fakeValidator counts calls and returns no records.
type fakeValidator struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeValidator) Name() string                         { return "validate.fake@v1" }
func (f *fakeValidator) Kind() domain.ValidationKind          { return domain.ValidationKind("static") }
func (f *fakeValidator) Applies(domain.ChangeItemFamily) bool { return true }
func (f *fakeValidator) Validate(context.Context, investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return &investigate.ValidationOutput{}, nil
}

// fakeClassifier yields one insufficient-evidence finding per in-scope item.
type fakeClassifier struct {
	mu    sync.Mutex
	calls int
	fail  int // transient failures before success
}

func (f *fakeClassifier) Classify(_ context.Context, in investigate.ClassifyInput) (*investigate.Classification, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	if n <= f.fail {
		return nil, fmt.Errorf("classifier unavailable (transient)")
	}
	var fs []domain.InvestigationFinding
	for _, it := range in.Bundle.Change.Items {
		if !it.InScope {
			continue
		}
		f := domain.InvestigationFinding{
			ChangeItem: it.ID, Class: domain.FindingInsufficientEvidence, Manifestation: it.Manifestation,
			Rule: "classify:test", Reasoning: "fake policy", Environment: in.Bundle.Snapshot.Environment,
			Evidence: it.Evidence, NextSteps: []string{"supply the values file"},
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

// fakeReviewer follows the ReviewApplier contract: only a human decides; a
// proxy decision is recorded and keeps the review pending.
type fakeReviewer struct{}

func (fakeReviewer) ApplyReview(inv *domain.Investigation, d investigate.ReviewDecision) error {
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
type fixture struct {
	Deps  Deps
	St    *store.Store
	Val   *fakeValidator
	Class *fakeClassifier
	Repo  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "values.yaml"), []byte("replicaCount: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := NewDirWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	val, cls := &fakeValidator{}, &fakeClassifier{}
	return &fixture{St: st, Val: val, Class: cls, Repo: repo, Deps: Deps{
		Artifacts: st, Records: st, Work: ws,
		Change:     bundle.NewChangeCollectorOver(upstream{}, st),
		Snapshot:   bundle.NewSnapshotBuilder(st),
		Assembler:  bundle.NewAssembler(st),
		Graph:      relations.NewBuilder(st),
		Exposure:   relations.NewExposure(),
		Traverser:  relations.NewTraverser(),
		Validators: func(*domain.ImpactReport) []investigate.Validator { return []investigate.Validator{val} },
		Classifier: cls, Reviewer: fakeReviewer{},
		Now: func() time.Time { return fixedNow },
	}}
}

func (f *fixture) request() investigate.Request {
	return investigate.Request{
		Product: "acme", From: "v1.0.0", To: "v2.0.0",
		Environment: investigate.EnvironmentInputs{Name: "shop-prod", Repo: f.Repo},
		Config: investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound,
			Validators: []string{"validate.fake@v1"}, Agent: investigate.AgentConfig{Driver: "scripted", PromptVersion: "investigate-agent/v1", Budget: investigate.DefaultAgentBudget}},
	}
}

func human(action investigate.ReviewAction) *investigate.ReviewDecision {
	return &investigate.ReviewDecision{Action: action, Reviewer: "alice", ReviewerKind: domain.ReviewerHuman, At: fixedNow, Note: "ok"}
}
