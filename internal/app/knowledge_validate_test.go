package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/semvalidate"
)

var kvT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

type failingValidator struct{}

func (failingValidator) Name() string { return "test.render@v1" }
func (failingValidator) Validate(context.Context, knowledge.ValidationInput) ([]domain.ValidationResult, error) {
	return nil, errors.New("helm is not installed")
}

func kvEvidence(uri string) domain.Evidence {
	return domain.NewEvidence(domain.EvidenceStructured, "crds", uri, "", "crds", "sha256:"+uri, kvT0)
}

func kvRelease(tag, def string) *domain.Release {
	ev := kvEvidence("https://x/" + tag + "/crds.yaml")
	field := domain.CRDFieldSchema{Path: "spec.privateKey.rotationPolicy", Type: "string", Default: def}
	return &domain.Release{
		Product: "cert-manager", Version: domain.MustVersion(tag, tag[1:]), IngestedAt: kvT0,
		Evidence: []domain.Evidence{ev},
		Snapshots: []domain.Snapshot{{ArtifactID: "crds", Kind: domain.SnapshotCRDs, Evidence: []domain.EvidenceID{ev.ID},
			CRDs: &domain.CRDSnapshot{CRDs: []domain.CRDSummary{{Name: "certificates.cert-manager.io", Group: "cert-manager.io", Kind: "Certificate",
				Versions: []domain.CRDVersionInfo{{Name: "v1", Served: true, Storage: true, SchemaPaths: []string{field.Path}, Fields: []domain.CRDFieldSchema{field}}}}}}}},
	}
}

func kvCandidate(title string) domain.SemanticCandidate {
	e := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://x/notes.md", "L1", title, "sha256:doc", kvT0)
	a := domain.ChangeAnchor{Release: "v1.18.0", EvidenceKeys: []string{domain.EvidenceKey(e)}, StatementKeys: []string{domain.StatementKey(e)}, ChangeIDs: []string{"chg-" + title[:3]}}
	members := []domain.CandidateMember{{ChangeID: a.ChangeIDs[0], Anchor: &a}}
	return domain.SemanticCandidate{ID: domain.CandidateID("cert-manager", a.Release, members), Product: "cert-manager", Release: a.Release,
		Members: members, Grouping: "single", Category: domain.CategoryConfiguration, Title: title, Evidence: []domain.Evidence{e},
		Producer: "semantic.candidates@v1", CreatedAt: kvT0}
}

func kvProposal(c domain.SemanticCandidate, model, path string) domain.SemanticProposal {
	at := kvT0.Add(time.Minute)
	prov := domain.Provenance{Method: domain.MethodAI, Producer: "semantic.propose@v1", Confidence: domain.ConfidenceMedium, Model: model,
		ModelVersion: model + "-1", PromptVersion: "semantic/v1", PromptDigest: "sha256:" + model + path, CallID: "call-" + model,
		InputEvidence: []domain.EvidenceID{c.Evidence[0].ID}, GeneratedAt: &at}
	p := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskSemanticMapping, Provider: "anthropic", Provenance: prov,
		Citations: []domain.EvidenceID{c.Evidence[0].ID},
		Assertion: domain.SemanticAssertion{
			Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "cert-manager", Group: "cert-manager.io", Kind: "Certificate", Path: path},
			Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: strp(`"Never"`), After: strp(`"Always"`)},
		}}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	return p
}

func strp(s string) *string { return &s }

func put(t *testing.T, s knowledge.Store, e any) {
	t.Helper()
	rec, err := domain.NewRecord(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func TestValidateKnowledgeWritesIdempotentlyAndRecordsGaps(t *testing.T) {
	store := knowledge.NewFileStore(t.TempDir())
	good, other := kvCandidate("The default rotationPolicy is now Always"), kvCandidate("Something unrelated happened")
	put(t, store, good)
	put(t, store, other)
	pGood := kvProposal(good, "claude-sonnet-5-5", "spec.privateKey.rotationPolicy")
	pGhost := kvProposal(good, "claude-opus-5-5", "spec.privateKey.rotationMode")
	pOther := kvProposal(other, "claude-sonnet-5-5", "spec.privateKey.rotationPolicy")
	for _, p := range []domain.SemanticProposal{pGood, pGhost, pOther} {
		put(t, store, p)
	}
	from, to := kvRelease("v1.17.0", `"Never"`), kvRelease("v1.18.0", `"Always"`)
	opts := ValidateOptions{
		Validators: append(semvalidate.Validators(), failingValidator{}),
		Resolve: func(_ context.Context, c domain.SemanticCandidate) (*domain.Release, *domain.Release, *domain.UpgradeEdge, bool, error) {
			return from, to, nil, c.ID == good.ID, nil // `other` is in no requested edge
		},
	}
	rep, err := ValidateKnowledge(context.Background(), store, opts)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Candidates != 1 || rep.SkippedCandidates != 1 || rep.Proposals != 2 {
		t.Fatalf("counts: %+v", rep)
	}
	crd := rep.Outcomes[semvalidate.ProducerCRD]
	if crd[domain.AspectSubject].Confirmed != 1 || crd[domain.AspectSubject].Refuted != 1 || crd[domain.AspectChange].Confirmed != 1 || crd[domain.AspectChange].Refuted != 1 {
		t.Errorf("crd outcomes (one real path, one plausible but non-existent): %+v %+v", crd[domain.AspectSubject], crd[domain.AspectChange])
	}
	if rep.Unavailable["test.render@v1"] != 2 {
		t.Errorf("a failing validator must be recorded per proposal: %v", rep.Unavailable)
	}
	if u := rep.Outcomes["test.render@v1"][domain.AspectChange]; u == nil || u.Inconclusive != 2 {
		t.Errorf("unavailable is stored as inconclusive: %+v", rep.Outcomes["test.render@v1"])
	}
	if rep.NotApplicable[semvalidate.ProducerValues] != 2 {
		t.Errorf("the values validator has no opinion on a CRD field: %v", rep.NotApplicable)
	}
	if rep.Written == 0 || rep.Existing != 0 {
		t.Errorf("first run: %+v", rep)
	}
	snap, err := store.Load(context.Background(), knowledge.Query{Kinds: []domain.RecordKind{domain.RecordValidation}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Validations) != rep.Written {
		t.Errorf("stored %d validations, report says %d", len(snap.Validations), rep.Written)
	}
	// second run: nothing new
	rep2, err := ValidateKnowledge(context.Background(), store, opts)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Written != 0 || rep2.Existing != rep.Written {
		t.Errorf("not idempotent: first %+v, second written %d existing %d", rep.Written, rep2.Written, rep2.Existing)
	}
}

// Two models asserting the same thing share one result (its id does not
// include the proposal): stored and tallied once, not a conflict.
func TestValidateKnowledgeProposalsWithIdenticalAssertionsShareAResult(t *testing.T) {
	store := knowledge.NewFileStore(t.TempDir())
	c := kvCandidate("The default rotationPolicy is now Always")
	put(t, store, c)
	put(t, store, kvProposal(c, "claude-sonnet-5-5", "spec.privateKey.rotationPolicy"))
	put(t, store, kvProposal(c, "glm-5.3-flash", "spec.privateKey.rotationPolicy"))
	from, to := kvRelease("v1.17.0", `"Never"`), kvRelease("v1.18.0", `"Always"`)
	rep, err := ValidateKnowledge(context.Background(), store, ValidateOptions{
		Validators: semvalidate.Validators(),
		Resolve: func(context.Context, domain.SemanticCandidate) (*domain.Release, *domain.Release, *domain.UpgradeEdge, bool, error) {
			return from, to, nil, true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Proposals != 2 || rep.Shared == 0 || rep.Outcomes[semvalidate.ProducerCRD][domain.AspectChange].Confirmed != 1 {
		t.Errorf("%+v", rep)
	}
}
