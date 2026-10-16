// Package run executes an investigation: the stages of DESIGN.md §5.1 over
// the ports of internal/investigate, and the in-process sequential runner
// (`ri investigate` without --temporal). The Temporal activities in
// internal/orchestrate call exactly the same Pipeline methods, so both paths
// produce the same run record. No go.temporal.io import here.
package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Deps are the collaborators of a Pipeline. Every field except Investigator
// and Report is required.
type Deps struct {
	Artifacts investigate.ArtifactStore
	Records   investigate.RecordStore
	Work      Workspace

	Change    investigate.ChangeCollector
	Snapshot  investigate.SnapshotBuilder
	Renderer  investigate.EnvironmentRenderer
	Assembler investigate.BundleAssembler
	Graph     investigate.GraphBuilder
	Exposure  investigate.ExposureMapper
	Traverser investigate.Traverser
	// Report computes the deterministic ImpactReport of the request (the
	// join validators and the exposure mapper read it). Optional: without
	// it, or when it errors, the run proceeds with no report and says so.
	Report func(ctx context.Context, req investigate.Request) (*domain.ImpactReport, error)
	// Validators builds the validators for a run, given the run's report
	// (the join validator wraps it).
	Validators func(report *domain.ImpactReport) []investigate.Validator
	// Investigator runs the agent; nil means no agent is available (the
	// stage records a not-run outcome).
	Investigator investigate.Investigator
	Classifier   investigate.Classifier
	Reviewer     investigate.ReviewApplier

	Now func() time.Time
}

// Pipeline runs single stages. It is safe for concurrent stages of
// different runs; stages of one run are sequential.
type Pipeline struct{ d Deps }

// New returns a Pipeline over deps.
func New(d Deps) *Pipeline {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Pipeline{d: d}
}

func (p *Pipeline) now() time.Time { return p.d.Now().UTC() }

// stageResult is what a stage body produces.
type stageResult struct {
	refs   Refs
	detail string
	refsIn []string // ids produced (audit refs)
	// create is the new run record (assemble-bundle only).
	create *domain.Investigation
	// apply mutates the existing record (every later stage).
	apply   func(*domain.Investigation) error
	outcome domain.PolicyOutcome
	review  domain.ReviewState
}

// Run executes one stage with audit events and memoised idempotency: a
// second call with the same key returns the first result without repeating
// any side effect (notably the non-idempotent agent stage).
func (p *Pipeline) Run(ctx context.Context, in StageInput) (StageOutput, error) {
	if in.Key == "" {
		in.Key = Key(in)
	}
	var cached StageOutput
	if ok, err := p.d.Work.Memo("out:"+in.Key, &cached); err != nil {
		return StageOutput{}, err
	} else if ok {
		return cached, nil
	}
	started := p.now()
	startEvent := "started"
	if in.Attempt > 1 {
		startEvent = "retried"
	}
	ev := func(event, detail string, refs []string) domain.AuditEvent {
		return domain.AuditEvent{At: p.now(), Stage: in.Stage, Event: event, Actor: "system", Refs: refs, Detail: detail}
	}
	startedEv := domain.AuditEvent{At: started, Stage: in.Stage, Event: startEvent, Actor: "system", Detail: phaseDetail(in)}
	hasRecord := in.Refs.Investigation != ""
	var journal []domain.AuditEvent // pre-record events of this stage
	if hasRecord {
		if _, err := p.update(ctx, in.Stage, in.Refs.Investigation, func(inv *domain.Investigation) error {
			inv.AppendAudit(startedEv)
			return nil
		}); err != nil {
			return StageOutput{}, err
		}
	} else if in.Stage != domain.StageAssemble {
		if _, err := p.W().Memo("events:"+in.Key, &journal); err != nil {
			return StageOutput{}, err
		}
		journal = append(journal, startedEv)
		if err := p.W().SetMemo("events:"+in.Key, journal); err != nil {
			return StageOutput{}, err
		}
	}

	res, err := p.exec(ctx, in)
	if err != nil {
		failed := ev("failed", Trunc(err.Error()), nil)
		if hasRecord {
			if _, uerr := p.update(ctx, in.Stage, in.Refs.Investigation, func(inv *domain.Investigation) error {
				inv.AppendAudit(failed)
				return nil
			}); uerr != nil {
				err = errors.Join(err, uerr)
			}
		} else if in.Stage != domain.StageAssemble {
			_ = p.W().SetMemo("events:"+in.Key, append(journal, failed))
		}
		return StageOutput{}, err
	}

	done := ev("completed", res.detail, res.refsIn)
	out := StageOutput{Stage: in.Stage, Key: in.Key, Refs: res.refs, Detail: res.detail, Outcome: res.outcome, Review: res.review}
	switch {
	case res.create != nil:
		inv := res.create
		round := ""
		if in.Round > 0 {
			round = fmt.Sprintf("round %d of %s", in.Round, in.Refs.Previous)
		}
		startedEv.Detail = strings.TrimSpace(round + " " + startedEv.Detail)
		if old, err := p.d.Records.GetInvestigation(ctx, inv.ID); err == nil {
			// The same bundle and configuration: attach to the existing record
			// (a resubmission, or an evidence round that found nothing new) and
			// re-run the later stages on it.
			inv = old
			done.Detail += "; bundle unchanged, existing record reused"
		} else if !errors.Is(err, investigate.ErrNotFound) {
			return StageOutput{}, err
		} else {
			for _, e := range in.Refs.Pending {
				inv.AppendAudit(e)
			}
		}
		inv.AppendAudit(startedEv)
		inv.AppendAudit(done)
		if err := p.d.Records.PutInvestigation(ctx, inv); err != nil {
			return StageOutput{}, investigate.PermanentError(in.Stage, err)
		}
		out.Refs.Investigation, out.Refs.Bundle = inv.ID, inv.Bundle
		out.Refs.Pending, out.Refs.Previous = nil, in.Refs.Previous
		out.Refs.Record, out.Digest = ContentDigest(inv), Digest(inv)
	case hasRecord:
		inv, err := p.update(ctx, in.Stage, in.Refs.Investigation, func(inv *domain.Investigation) error {
			if res.apply != nil {
				if err := res.apply(inv); err != nil {
					return err
				}
			}
			inv.AppendAudit(done)
			return nil
		})
		if err != nil {
			return StageOutput{}, err
		}
		out.Refs.Record, out.Digest = ContentDigest(inv), Digest(inv)
	default:
		out.Refs.Pending = append(append([]domain.AuditEvent(nil), in.Refs.Pending...), append(journal, done)...)
	}
	if err := p.W().SetMemo("out:"+in.Key, out); err != nil {
		return StageOutput{}, err
	}
	return out, nil
}

// W exposes the workspace.
func (p *Pipeline) W() Workspace { return p.d.Work }

func phaseDetail(in StageInput) string {
	d := ""
	if in.Phase != "" {
		d = in.Phase
	}
	if in.Round > 0 {
		d = strings.TrimSpace(d + fmt.Sprintf(" round %d", in.Round))
	}
	return d
}

// update loads the run record, applies f and writes it back.
func (p *Pipeline) update(ctx context.Context, stage domain.Stage, id string, f func(*domain.Investigation) error) (*domain.Investigation, error) {
	inv, err := p.d.Records.GetInvestigation(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := f(inv); err != nil {
		return nil, err
	}
	if err := p.d.Records.PutInvestigation(ctx, inv); err != nil {
		return nil, investigate.PermanentError(stage, err)
	}
	return inv, nil
}

func (p *Pipeline) exec(ctx context.Context, in StageInput) (*stageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch in.Stage {
	case domain.StageCollectChange:
		return p.collect(ctx, in)
	case domain.StageBuildSnapshot:
		return p.snapshot(ctx, in)
	case domain.StageAssemble:
		return p.assemble(ctx, in)
	case domain.StageBuildGraph:
		return p.buildGraph(ctx, in)
	case domain.StageTraverse:
		return p.traverse(ctx, in)
	case domain.StageValidate:
		return p.validate(ctx, in)
	case domain.StageInvestigate:
		return p.investigate(ctx, in)
	case domain.StageClassify:
		return p.classify(ctx, in)
	}
	return nil, investigate.PermanentError(in.Stage, fmt.Errorf("no such stage %q", in.Stage))
}

// --- stages ---------------------------------------------------------------------------------

func (p *Pipeline) collect(ctx context.Context, in StageInput) (*stageResult, error) {
	r := in.Request
	res, err := p.d.Change.CollectChange(ctx, investigate.ChangeRequest{Product: r.Product, From: r.From, To: r.To, Render: r.Config.Render})
	if err != nil {
		return nil, err
	}
	d, err := p.W().Put(res)
	if err != nil {
		return nil, err
	}
	refs := in.Refs
	refs.Change = d
	return &stageResult{refs: refs, detail: res.Record.ID, refsIn: []string{res.Record.ID}}, nil
}

// withExtra applies the inputs a reviewer asked for to the environment
// inputs. `values:<path>`, `manifest:<path>`, `crd:<path>` and
// `image:<ref>` extend the matching list; anything else cannot be fulfilled
// automatically and is recorded (the round still re-runs, so files the
// reviewer changed in place are picked up by the new snapshot).
func withExtra(env investigate.EnvironmentInputs, extra []string) (investigate.EnvironmentInputs, []string) {
	var unfulfilled []string
	for _, x := range extra {
		kind, val, ok := strings.Cut(x, ":")
		switch {
		case !ok || val == "":
			unfulfilled = append(unfulfilled, x)
		case kind == "values":
			env.ValuesFiles = append(append([]string(nil), env.ValuesFiles...), val)
		case kind == "manifest":
			env.Manifests = append(append([]string(nil), env.Manifests...), val)
		case kind == "crd":
			env.CRDs = append(append([]string(nil), env.CRDs...), val)
		case kind == "image":
			env.Images = append(append([]string(nil), env.Images...), val)
		default:
			unfulfilled = append(unfulfilled, x)
		}
	}
	return env, unfulfilled
}

func (p *Pipeline) snapshot(ctx context.Context, in StageInput) (*stageResult, error) {
	envIn, unfulfilled := withExtra(in.Request.Environment, in.Extra)
	res, err := p.d.Snapshot.BuildSnapshot(ctx, envIn)
	if err != nil {
		return nil, err
	}
	d, err := p.W().Put(res)
	if err != nil {
		return nil, err
	}
	refs := in.Refs
	refs.Snapshot = d
	detail := res.Snapshot.ID
	if len(unfulfilled) > 0 {
		detail += "; not fulfillable automatically: " + strings.Join(unfulfilled, ", ")
	}
	return &stageResult{refs: refs, detail: detail, refsIn: []string{res.Snapshot.ID}}, nil
}

func (p *Pipeline) putJSON(ctx context.Context, kind domain.ArtifactKind, path string, v any) (domain.ArtifactRef, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return domain.ArtifactRef{}, err
	}
	return p.d.Artifacts.Put(ctx, investigate.ArtifactSpec{
		Kind: kind, Path: path, Origin: domain.OriginDerived, MediaType: "application/json",
		URI: "ri:" + Producer, Producer: Producer,
	}, raw)
}

func (p *Pipeline) assemble(ctx context.Context, in StageInput) (*stageResult, error) {
	var change investigate.ChangeResult
	var snap investigate.SnapshotResult
	if err := p.W().Get(in.Refs.Change, &change); err != nil {
		return nil, investigate.PermanentError(in.Stage, err)
	}
	if err := p.W().Get(in.Refs.Snapshot, &snap); err != nil {
		return nil, investigate.PermanentError(in.Stage, err)
	}
	var renders *investigate.RenderResult
	if in.Request.Config.Render {
		var err error
		if renders, err = p.d.Renderer.RenderEnvironment(ctx, &change, &snap, in.Request.Config); err != nil {
			return nil, err
		}
	}
	beat(ctx, "rendered")
	b, err := p.d.Assembler.Assemble(ctx, &change, &snap, renders)
	if err != nil {
		return nil, err
	}
	// The bundle is fully determined by its inputs (no wall clock inside),
	// so storing a re-assembled identical bundle is a no-op.
	if err := p.d.Records.PutBundle(ctx, b); err != nil {
		return nil, investigate.PermanentError(in.Stage, err)
	}
	inv := &domain.Investigation{
		SchemaVersion: domain.InvestigationSchemaVersion,
		ID:            in.Request.ID(b.ID),
		Bundle:        b.ID,
		Snapshot:      b.Snapshot.ID,
		ChangeRecord:  b.Change.ID,
		Environment:   in.Request.Environment.Name,
		Config:        in.Request.Config.Digest(),
		Status:        domain.InvestigationRunning,
		Review:        domain.HumanReview{State: domain.ReviewNotRequired},
		CreatedAt:     p.now(),
	}
	detail := b.ID
	if p.d.Report != nil {
		rep, rerr := p.d.Report(ctx, in.Request)
		switch {
		case rerr != nil:
			detail += "; no impact report: " + rerr.Error()
		case rep != nil:
			ref, err := p.putJSON(ctx, domain.ArtifactImpactReport, "derived/impact/report.json", rep)
			if err != nil {
				return nil, err
			}
			inv.ImpactReport = ref.ID
			inv.Artifacts = append(inv.Artifacts, ref)
		}
	}
	return &stageResult{refs: in.Refs, create: inv, detail: detail, refsIn: []string{b.ID, inv.ID}}, nil
}

// runCtx is the loaded state a later stage works on.
type runCtx struct {
	inv    *domain.Investigation
	bundle *domain.EvidenceBundle
	graph  *domain.RelationshipGraph
	report *domain.ImpactReport
}

func (p *Pipeline) readArtifact(ctx context.Context, ref domain.ArtifactRef, v any) error {
	rc, err := p.d.Artifacts.Open(ctx, ref.Digest)
	if err != nil {
		return err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	return json.NewDecoder(bytes.NewReader(raw)).Decode(v)
}

func artifactByID(inv *domain.Investigation, id string) (domain.ArtifactRef, bool) {
	for _, a := range inv.Artifacts {
		if a.ID == id {
			return a, true
		}
	}
	return domain.ArtifactRef{}, false
}

func (p *Pipeline) load(ctx context.Context, id string, needGraph bool) (*runCtx, error) {
	inv, err := p.d.Records.GetInvestigation(ctx, id)
	if err != nil {
		return nil, err
	}
	b, err := p.d.Records.GetBundle(ctx, inv.Bundle)
	if err != nil {
		return nil, err
	}
	rc := &runCtx{inv: inv, bundle: b}
	if inv.ImpactReport != "" {
		ref, ok := artifactByID(inv, inv.ImpactReport)
		if !ok {
			return nil, investigate.PermanentError(domain.StageAssemble, fmt.Errorf("impact report artifact %s missing from the run", inv.ImpactReport))
		}
		rc.report = &domain.ImpactReport{}
		if err := p.readArtifact(ctx, ref, rc.report); err != nil {
			return nil, err
		}
	}
	if needGraph {
		ref, ok := artifactByID(inv, inv.Graph)
		if !ok {
			return nil, investigate.PermanentError(domain.StageBuildGraph, fmt.Errorf("the run has no graph artifact"))
		}
		rc.graph = &domain.RelationshipGraph{}
		if err := p.readArtifact(ctx, ref, rc.graph); err != nil {
			return nil, err
		}
	}
	return rc, nil
}

func (p *Pipeline) storeGraph(ctx context.Context, g *domain.RelationshipGraph) (domain.ArtifactRef, error) {
	return p.putJSON(ctx, domain.ArtifactGraph, "derived/graph/"+g.ID+".json", g)
}

func (p *Pipeline) buildGraph(ctx context.Context, in StageInput) (*stageResult, error) {
	rc, err := p.load(ctx, in.Refs.Investigation, false)
	if err != nil {
		return nil, err
	}
	g, err := p.d.Graph.BuildGraph(ctx, rc.bundle)
	if err != nil {
		return nil, err
	}
	ref, err := p.storeGraph(ctx, g)
	if err != nil {
		return nil, err
	}
	return &stageResult{refs: in.Refs, detail: g.ID, refsIn: []string{g.ID, ref.ID}, apply: func(inv *domain.Investigation) error {
		inv.Graph = ref.ID
		inv.Artifacts = mergeArtifacts(inv.Artifacts, ref)
		return nil
	}}, nil
}

func (p *Pipeline) traverse(ctx context.Context, in StageInput) (*stageResult, error) {
	rc, err := p.load(ctx, in.Refs.Investigation, true)
	if err != nil {
		return nil, err
	}
	g, graphRef, detail := rc.graph, rc.inv.Graph, ""
	var newRef *domain.ArtifactRef
	if in.Phase == PhaseFinal {
		merged, notes := MergeProposed(g, rc.inv.Assertions)
		if merged.ID != g.ID {
			ref, err := p.storeGraph(ctx, merged)
			if err != nil {
				return nil, err
			}
			g, graphRef, newRef = merged, ref.ID, &ref
		}
		detail = strings.Join(notes, "; ")
	}
	roots, err := p.d.Exposure.Exposure(ctx, rc.bundle, g, rc.report)
	if err != nil {
		return nil, err
	}
	tr, err := p.d.Traverser.Traverse(ctx, g, roots, in.Request.Config.Bound)
	if err != nil {
		return nil, err
	}
	detail = strings.TrimSpace(fmt.Sprintf("%s complete=%t paths=%d %s", tr.ID, tr.Complete, len(tr.Paths), detail))
	return &stageResult{refs: in.Refs, detail: detail, refsIn: []string{tr.ID}, apply: func(inv *domain.Investigation) error {
		inv.Graph, inv.Traversal = graphRef, tr
		if newRef != nil {
			inv.Artifacts = mergeArtifacts(inv.Artifacts, *newRef)
		}
		return nil
	}}, nil
}

func (p *Pipeline) validate(ctx context.Context, in StageInput) (*stageResult, error) {
	rc, err := p.load(ctx, in.Refs.Investigation, true)
	if err != nil {
		return nil, err
	}
	var vals []investigate.Validator
	if p.d.Validators != nil {
		vals = p.d.Validators(rc.report)
	}
	enabled := map[string]bool{}
	for _, n := range in.Request.Config.Validators {
		enabled[n] = true
	}
	var records []domain.ValidationRecord
	var arts []domain.ArtifactRef
	var evid []domain.Evidence
	ran := 0
	items := append(append([]domain.ChangeItem(nil), rc.bundle.Change.Items...), rc.bundle.EnvironmentItems...)
	for _, it := range items {
		if !it.InScope {
			continue
		}
		for _, v := range vals {
			if (len(enabled) > 0 && !enabled[v.Name()]) || !v.Applies(it.Family) {
				continue
			}
			beat(ctx, v.Name()+" "+it.ID)
			out, err := v.Validate(ctx, investigate.ValidationInput{
				Bundle: rc.bundle, Graph: rc.graph, Traversal: rc.inv.Traversal, Report: rc.report, ChangeItem: it.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("validator %s on %s: %w", v.Name(), it.ID, err)
			}
			ran++
			if out != nil {
				records = append(records, out.Records...)
				arts = append(arts, out.Artifacts...)
				evid = append(evid, out.Evidence...)
			}
		}
	}
	var ids []string
	for _, r := range records {
		ids = append(ids, r.ID)
	}
	return &stageResult{refs: in.Refs, detail: fmt.Sprintf("%d validator runs, %d records", ran, len(records)), refsIn: ids,
		apply: func(inv *domain.Investigation) error {
			inv.Validations = mergeValidations(inv.Validations, records)
			inv.Artifacts = mergeArtifacts(inv.Artifacts, arts...)
			inv.Evidence = mergeEvidence(inv.Evidence, evid)
			return nil
		}}, nil
}

func (p *Pipeline) investigate(ctx context.Context, in StageInput) (*stageResult, error) {
	cfg := in.Request.Config.Agent
	if !cfg.Enabled || p.d.Investigator == nil {
		reason := "agent disabled (--agent not given)"
		if cfg.Enabled {
			reason = "no investigator is configured"
		}
		run := &domain.AgentRun{Status: domain.AgentNotRun, Reason: reason, Budget: cfg.Budget}
		return &stageResult{refs: in.Refs, detail: reason, apply: func(inv *domain.Investigation) error {
			inv.Agent = run
			return nil
		}}, nil
	}
	rc, err := p.load(ctx, in.Refs.Investigation, true)
	if err != nil {
		return nil, err
	}
	beat(ctx, "agent")
	out, err := p.d.Investigator.Investigate(ctx, investigate.InvestigateInput{
		Bundle: rc.bundle, Graph: rc.graph, Traversal: rc.inv.Traversal, Report: rc.report,
		Validations: rc.inv.Validations, Config: cfg,
	})
	if err != nil {
		return nil, err
	}
	actor := "agent:" + out.Run.Model
	return &stageResult{refs: in.Refs, detail: fmt.Sprintf("agent %s: %s", out.Run.Status, out.Run.Reason), refsIn: []string{out.Run.CallID},
		apply: func(inv *domain.Investigation) error {
			run := out.Run
			inv.Agent = &run
			inv.Hypotheses = out.Hypotheses
			inv.Assertions = out.Assertions
			inv.EvidenceRequests = out.EvidenceRequests
			inv.ValidationRequests = out.ValidationRequests
			inv.Validations = mergeValidations(inv.Validations, out.Validations)
			inv.Artifacts = mergeArtifacts(inv.Artifacts, out.Artifacts...)
			inv.Evidence = mergeEvidence(inv.Evidence, out.Evidence)
			// §5.4: one `requested` event per agent request, actor agent:<model>.
			for _, r := range out.EvidenceRequests {
				inv.AppendAudit(domain.AuditEvent{At: p.now(), Stage: domain.StageInvestigate, Event: "requested", Actor: actor, Refs: []string{r.ID}, Detail: "evidence: " + string(r.Kind) + " " + r.Target})
			}
			for _, r := range out.ValidationRequests {
				inv.AppendAudit(domain.AuditEvent{At: p.now(), Stage: domain.StageInvestigate, Event: "requested", Actor: actor, Refs: []string{r.ID}, Detail: "validation: " + string(r.Kind)})
			}
			return nil
		}}, nil
}

func agentOutcome(inv *domain.Investigation) *investigate.AgentOutcome {
	if inv.Agent == nil {
		return nil
	}
	return &investigate.AgentOutcome{
		Run: *inv.Agent, Hypotheses: inv.Hypotheses, Assertions: inv.Assertions,
		EvidenceRequests: inv.EvidenceRequests, ValidationRequests: inv.ValidationRequests,
	}
}

func (p *Pipeline) classify(ctx context.Context, in StageInput) (*stageResult, error) {
	rc, err := p.load(ctx, in.Refs.Investigation, false)
	if err != nil {
		return nil, err
	}
	cls, err := p.d.Classifier.Classify(ctx, investigate.ClassifyInput{
		Bundle: rc.bundle, Traversal: rc.inv.Traversal, Report: rc.report, Agent: agentOutcome(rc.inv),
		Validations: rc.inv.Validations, Config: in.Request.Config,
	})
	if err != nil {
		return nil, err
	}
	// Automated approval is enacted only when the policy says it is enabled
	// (never in milestone 1): an eligible run otherwise still waits for a human.
	auto := cls.Decision.Outcome == domain.PolicyAutoApproveEligible && cls.Decision.AutoApprovalEnabled && in.Request.Config.AutoApproval
	review, status := domain.ReviewAwaiting, domain.InvestigationAwaiting
	if auto {
		review, status = domain.ReviewNotRequired, domain.InvestigationCompleted
	}
	var ids []string
	for _, f := range cls.Findings {
		ids = append(ids, f.ID)
	}
	return &stageResult{refs: in.Refs, outcome: cls.Decision.Outcome, review: review,
		detail: fmt.Sprintf("%d findings, decision %s", len(cls.Findings), cls.Decision.Outcome), refsIn: ids,
		apply: func(inv *domain.Investigation) error {
			inv.Findings = cls.Findings
			d := cls.Decision
			inv.Decision = &d
			inv.Review = domain.HumanReview{State: review}
			inv.Status = status
			if err := inv.ValidateAgainst(rc.bundle); err != nil {
				return investigate.PermanentError(domain.StageClassify, err)
			}
			return nil
		}}, nil
}

// --- review and finalisation ----------------------------------------------------------------

// ApplyReview applies a human-review decision to the run record through the
// ReviewApplier (pure: it sets Review, appends the `decided` event and
// validates). A decision the applier rejects is a permanent error and leaves
// the record untouched. A proxy decision is recorded and leaves the review
// pending. Idempotent per decision.
func (p *Pipeline) ApplyReview(ctx context.Context, in ReviewInput) (ReviewOutput, error) {
	key := ReviewKey(in)
	var cached ReviewOutput
	if ok, err := p.W().Memo("out:"+key, &cached); err != nil {
		return ReviewOutput{}, err
	} else if ok {
		return cached, nil
	}
	var out ReviewOutput
	_, err := p.update(ctx, domain.StageHumanReview, in.Investigation, func(inv *domain.Investigation) error {
		if err := p.d.Reviewer.ApplyReview(inv, in.Decision); err != nil {
			return investigate.PermanentError(domain.StageHumanReview, fmt.Errorf("%w: %v", investigate.ErrRejected, err))
		}
		switch inv.Review.State {
		case domain.ReviewApproved, domain.ReviewRejected, domain.ReviewMoreEvidenceWanted:
			inv.Status = domain.InvestigationCompleted
		}
		out = ReviewOutput{State: inv.Review.State, Status: inv.Status, Digest: Digest(inv)}
		return nil
	})
	if err != nil {
		return ReviewOutput{}, err
	}
	if err := p.W().SetMemo("out:"+key, out); err != nil {
		return ReviewOutput{}, err
	}
	return out, nil
}

// Finalize ends a run without a decision (review timeout, cancellation,
// exhausted evidence rounds): it appends the event and sets the status. It
// never approves anything.
func (p *Pipeline) Finalize(ctx context.Context, in FinalizeInput) (ReviewOutput, error) {
	var out ReviewOutput
	_, err := p.update(ctx, in.Stage, in.Investigation, func(inv *domain.Investigation) error {
		inv.Status = in.Status
		inv.AppendAudit(domain.AuditEvent{At: p.now(), Stage: in.Stage, Event: in.Event, Actor: "system", Detail: in.Detail})
		out = ReviewOutput{State: inv.Review.State, Status: inv.Status, Digest: Digest(inv)}
		return nil
	})
	return out, err
}

// --- merging helpers ------------------------------------------------------------------------

func mergeArtifacts(have []domain.ArtifactRef, add ...domain.ArtifactRef) []domain.ArtifactRef {
	seen := map[string]bool{}
	for _, a := range have {
		seen[a.ID] = true
	}
	for _, a := range add {
		if !seen[a.ID] {
			seen[a.ID] = true
			have = append(have, a)
		}
	}
	return have
}

func mergeEvidence(have, add []domain.Evidence) []domain.Evidence {
	seen := map[domain.EvidenceID]bool{}
	for _, e := range have {
		seen[e.ID] = true
	}
	for _, e := range add {
		if !seen[e.ID] {
			seen[e.ID] = true
			have = append(have, e)
		}
	}
	return have
}

func mergeValidations(have, add []domain.ValidationRecord) []domain.ValidationRecord {
	seen := map[string]bool{}
	for _, v := range have {
		seen[v.ID] = true
	}
	for _, v := range add {
		if !seen[v.ID] {
			seen[v.ID] = true
			have = append(have, v)
		}
	}
	sort.SliceStable(have, func(i, j int) bool { return have[i].ID < have[j].ID })
	return have
}

// Trunc bounds an error text stored in an audit detail (validation errors
// can list hundreds of ids).
func Trunc(s string) string {
	const max = 600
	if len(s) <= max {
		return s
	}
	return s[:max] + "… (" + fmt.Sprint(len(s)-max) + " more bytes)"
}
