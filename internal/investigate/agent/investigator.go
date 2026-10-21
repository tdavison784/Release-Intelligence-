package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// Investigator is the agent stage (investigate.Investigator): it serves a
// Session to a model through the configured driver, within the budget, and
// returns everything the session recorded.
type Investigator struct {
	st         investigate.ArtifactStore
	drivers    map[string]investigate.AgentDriver
	validators []investigate.Validator
	now        func() time.Time
	// rels are the graph builder's options for the session's consumer graph
	// (prompt feature traceConsumers).
	rels []relations.Option
}

// Option configures an Investigator.
type Option func(*Investigator)

// WithDriver registers a driver under a name (AgentConfig.Driver). The
// claude-cli driver is registered by default; exchange and replay need
// their directory / transcript and are registered by the caller.
func WithDriver(name string, d investigate.AgentDriver) Option {
	return func(i *Investigator) { i.drivers[name] = d }
}

// WithValidators makes validators available to request_validation.
func WithValidators(vs ...investigate.Validator) Option {
	return func(i *Investigator) { i.validators = append(i.validators, vs...) }
}

// WithRelations sets the relationship rules (relations.WithCatalog, …) of
// the consumer graph a session with feature traceConsumers builds: the same
// options the pipeline's graph builder takes, so the consumer graph is the
// stored graph plus consumer detail.
func WithRelations(opts ...relations.Option) Option {
	return func(i *Investigator) { i.rels = append(i.rels, opts...) }
}

// WithClock sets the clock that stamps generatedAt on agent records.
func WithClock(now func() time.Time) Option { return func(i *Investigator) { i.now = now } }

// New returns the investigation agent stage. st stores the transcript.
func New(st investigate.ArtifactStore, opts ...Option) investigate.Investigator {
	i := &Investigator{st: st, drivers: map[string]investigate.AgentDriver{DriverClaudeCLI: &ClaudeCLI{}}, now: time.Now}
	for _, o := range opts {
		o(i)
	}
	return i
}

// Budget returns b with every unset (zero) field taken from the default
// budget (DESIGN.md §4.3).
func Budget(b domain.AgentBudget) domain.AgentBudget {
	d := investigate.DefaultAgentBudget
	if b.MaxTurns <= 0 {
		b.MaxTurns = d.MaxTurns
	}
	if b.MaxToolCalls <= 0 {
		b.MaxToolCalls = d.MaxToolCalls
	}
	if b.MaxTokens <= 0 {
		b.MaxTokens = d.MaxTokens
	}
	if b.MaxReadBytes <= 0 {
		b.MaxReadBytes = d.MaxReadBytes
	}
	if b.MaxSeconds <= 0 {
		b.MaxSeconds = d.MaxSeconds
	}
	return b
}

// Prepare builds the session a driver will be given: the prompt, the
// rendered task, its digest and the tool surface. Investigate uses it; it is
// exported for `ri investigate mcp` and tests.
func Prepare(st investigate.ArtifactStore, in investigate.InvestigateInput, validators []investigate.Validator, now func() time.Time) (*Session, investigate.AgentSession, error) {
	return PrepareWith(st, in, validators, now, nil)
}

// PrepareWith is Prepare with the relationship rules of the consumer graph
// (WithRelations).
func PrepareWith(st investigate.ArtifactStore, in investigate.InvestigateInput, validators []investigate.Validator, now func() time.Time, rels []relations.Option) (*Session, investigate.AgentSession, error) {
	if in.Bundle == nil {
		return nil, investigate.AgentSession{}, investigate.PermanentError(domain.StageInvestigate, errors.New("agent: no bundle"))
	}
	if st == nil {
		return nil, investigate.AgentSession{}, investigate.PermanentError(domain.StageInvestigate, errors.New("agent: no artifact store"))
	}
	p, err := LoadPrompt(in.Config.PromptVersion)
	if err != nil {
		return nil, investigate.AgentSession{}, investigate.PermanentError(domain.StageInvestigate, err)
	}
	budget := Budget(in.Config.Budget)
	var agenda []domain.AgendaCoverage
	var budgetRule, task, digest string
	itemCap := 0
	switch {
	case p.Features.PerItem:
		// Per-item sessions: the session-level task and digest are set per
		// item (BeginItem); the run's digest is the digest of the item digests.
		agenda = ItemAgenda(in.Bundle, in.Validations)
		if itemCap = in.Config.MaxItems; itemCap <= 0 {
			itemCap = DefaultItemCap
		}
		budget, budgetRule = ItemBudgetFor(p.Features, in.Config.ItemBudget, itemCap, len(agenda))
	case p.Features.Agenda:
		agenda = Agenda(in.Bundle, in.Validations)
		budget, budgetRule = AgendaBudget(in.Config.Budget, len(agenda))
	}
	if !p.Features.PerItem {
		if task, err = p.renderTask(in, budget, agenda); err != nil {
			return nil, investigate.AgentSession{}, investigate.PermanentError(domain.StageInvestigate, err)
		}
		digest = p.Digest(task)
	}
	var graphs ConsumerGraphs
	if p.Features.TraceConsumers {
		b := in.Bundle
		graphs = func(ctx context.Context, side relations.Side) (*domain.RelationshipGraph, error) {
			return relations.BuildConsumerGraph(ctx, st, b, side, rels...)
		}
	}
	s, err := NewSession(SessionConfig{Bundle: in.Bundle, Store: st, Graph: in.Graph, Traversal: in.Traversal, Report: in.Report,
		Validations: in.Validations, ValidationArtifacts: in.ValidationArtifacts, ValidationEvidence: in.ValidationEvidence,
		Validators: validators, Budget: budget, Prompt: p, PromptDigest: digest, Model: in.Config.Model, Now: now,
		Agenda: agenda, BudgetRule: budgetRule, ItemCap: itemCap, ConsumerGraphs: graphs})
	if err != nil {
		return nil, investigate.AgentSession{}, investigate.PermanentError(domain.StageInvestigate, err)
	}
	return s, investigate.AgentSession{SystemPrompt: p.System, Task: task, PromptVersion: p.Version, PromptDigest: digest,
		Model: in.Config.Model, Budget: budget}, nil
}

// Investigate runs the agent stage. Budget exhaustion, a model that never
// submits, an exchange round awaiting its answer and driver failures are
// outcomes (AgentRun.Status with a reason); errors are infrastructure
// failures (no store, no CLI, an invalid configuration).
func (i *Investigator) Investigate(ctx context.Context, in investigate.InvestigateInput) (*investigate.AgentOutcome, error) {
	cfg := in.Config
	if !cfg.Enabled {
		return &investigate.AgentOutcome{Run: domain.AgentRun{Status: domain.AgentNotRun, Budget: Budget(cfg.Budget),
			Reason: "the agent stage is opt-in and was not enabled (--agent); environment configuration was not sent to any model"}}, nil
	}
	driver, ok := i.drivers[cfg.Driver]
	if !ok {
		var names []string
		for n := range i.drivers {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, investigate.PermanentError(domain.StageInvestigate, fmt.Errorf("agent: unknown driver %q (registered: %s)", cfg.Driver, strings.Join(names, ", ")))
	}
	s, as, err := PrepareWith(i.st, in, i.validators, i.now, i.rels)
	if err != nil {
		return nil, err
	}
	if s.cfg.Prompt.Features.PerItem {
		return i.investigateItems(ctx, in, driver, s, as)
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(as.Budget.MaxSeconds)*time.Second)
	defer cancel()
	dr, err := driver.Run(runCtx, as, s)
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			dr = &investigate.DriverResult{Stop: "budget:seconds", Model: cfg.Model}
			s.RecordEnvelope(cfg.Driver, *dr, nil)
		default:
			return nil, err
		}
	}
	return s.Outcome(ctx, i.st, cfg.Driver, *dr)
}

// runStatus maps the driver's stop and the tool layer's budget state to the
// run status (DESIGN.md §4.3: exhaustion is an outcome; only a submitted run
// is complete).
func runStatus(dr investigate.DriverResult, s *Session) (domain.AgentRunStatus, string) {
	b := s.cfg.Budget
	turns := b.MaxTurns
	if s.Submitted() {
		turns++ // the closing message after an accepted submit_findings
	}
	switch {
	case strings.HasPrefix(dr.Stop, "pending:"):
		return domain.AgentFailed, "exchange-pending: the round request " + strings.TrimPrefix(dr.Stop, "pending:") + " awaits an answer; answer it and re-run the stage"
	case s.Exhausted() != "":
		return domain.AgentBudgetExhausted, s.Exhausted()
	case strings.HasPrefix(dr.Stop, "budget:"):
		return domain.AgentBudgetExhausted, dr.Stop
	case b.MaxTokens > 0 && dr.Tokens > b.MaxTokens:
		return domain.AgentBudgetExhausted, fmt.Sprintf("budget:tokens (%d reported, budget %d)", dr.Tokens, b.MaxTokens)
	case b.MaxTurns > 0 && dr.Turns > turns:
		return domain.AgentBudgetExhausted, fmt.Sprintf("budget:turns (%d reported, budget %d)", dr.Turns, b.MaxTurns)
	case dr.Stop == "submitted" && s.Submitted():
		return domain.AgentSubmitted, ""
	case dr.Stop == "submitted":
		return domain.AgentFailed, "the driver reported a submission the tool layer never accepted"
	case dr.Stop == "":
		return domain.AgentFailed, "the session ended without submit_findings"
	}
	return domain.AgentFailed, strings.TrimPrefix(dr.Stop, "error:")
}

// Outcome closes the session: it fills the model fields the driver reported
// into every record, stores the transcript as an agent-transcript artifact
// and returns the agent stage's outcome.
func (s *Session) Outcome(ctx context.Context, st investigate.ArtifactStore, driver string, dr investigate.DriverResult) (*investigate.AgentOutcome, error) {
	status, reason := runStatus(dr, s)
	return s.outcome(ctx, st, driver, dr, status, reason)
}

func (s *Session) outcome(ctx context.Context, st investigate.ArtifactStore, driver string, dr investigate.DriverResult, status domain.AgentRunStatus, reason string) (*investigate.AgentOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	model := dr.Model
	if model == "" {
		model = s.cfg.Model
	}
	if model == "" {
		model = unreportedModelInfo
	}
	version := dr.ModelVersion
	if version == "" {
		version = unreportedModelInfo
	}
	fill := func(p *domain.Provenance) {
		if c, ok := s.callInfo[p.CallID]; ok { // per-item sessions: the model of that session's call
			if p.Model == "" && c.Model != "" {
				p.Model = c.Model
			}
			if p.ModelVersion == "" && c.ModelVersion != "" {
				p.ModelVersion = c.ModelVersion
			}
			if p.Provider == "" {
				p.Provider = c.Provider
			}
		}
		if p.Model == "" {
			p.Model = model
		}
		if p.ModelVersion == "" {
			p.ModelVersion = version
		}
		if p.Provider == "" {
			p.Provider = dr.Provider
		}
	}
	out := &investigate.AgentOutcome{}
	for _, h := range s.hyps {
		fill(&h.Provenance)
		out.Hypotheses = append(out.Hypotheses, h)
	}
	for _, a := range s.asserts {
		fill(&a.Provenance)
		out.Assertions = append(out.Assertions, a)
	}
	for _, r := range s.evReqs {
		fill(&r.Provenance)
		out.EvidenceRequests = append(out.EvidenceRequests, r)
	}
	for _, r := range s.valReqs {
		fill(&r.Provenance)
		out.ValidationRequests = append(out.ValidationRequests, r)
	}
	out.Validations = append(out.Validations, s.vals...)
	for _, n := range s.notes {
		fill(&n.Provenance)
		out.Notes = append(out.Notes, n)
	}
	for _, c := range s.claims {
		fill(&c.Provenance)
		out.Claims = append(out.Claims, c)
	}
	droppedCites, droppedFacts := 0, 0
	for _, v := range s.verdicts {
		fill(&v.Provenance)
		out.Verdicts = append(out.Verdicts, v)
		droppedCites += v.DroppedCitations
		droppedFacts += v.DroppedVerifiedFacts
	}
	for _, a := range s.assessments {
		fill(&a.Provenance)
		out.Assessments = append(out.Assessments, a)
	}
	out.ProposedNodes = append(out.ProposedNodes, s.propNodes...)
	out.AnswerLinks = append(out.AnswerLinks, s.links...)
	for _, id := range s.runEvIDs {
		out.Evidence = append(out.Evidence, s.runEv[id])
	}
	sort.Slice(out.Evidence, func(i, j int) bool { return out.Evidence[i].ID < out.Evidence[j].ID })
	for _, err := range validateOutcome(out) {
		return nil, investigate.PermanentError(domain.StageInvestigate, fmt.Errorf("agent: outcome invariant: %w", err))
	}

	transcript := append([]byte(nil), s.transcript.Bytes()...)
	tag := domain.ShortHash(dr.CallID, s.cfg.PromptDigest)
	ref, err := st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactAgentTranscript, Path: "derived/agent/" + tag + "/transcript.jsonl",
		Origin: domain.OriginDerived, MediaType: "application/x-ndjson", URI: "ri:" + Producer, Producer: Producer,
		Tags: []string{"agent-transcript", driver, strings.ToLower(model)}}, transcript)
	if err != nil {
		return nil, investigate.TransientError(domain.StageInvestigate, fmt.Errorf("agent: store transcript: %w", err))
	}
	out.Artifacts = append([]domain.ArtifactRef{ref}, s.runArts...)
	out.Run = domain.AgentRun{Status: status, Reason: reason, Model: model, Provider: dr.Provider, PromptVersion: s.cfg.Prompt.Version,
		PromptDigest: s.cfg.PromptDigest, CallID: dr.CallID, Budget: s.cfg.Budget, Turns: dr.Turns, ToolCalls: s.toolCalls,
		Tokens: dr.Tokens, Transcript: ref.ID, Coverage: s.coverage(status, reason), BudgetRule: s.cfg.BudgetRule, ItemCap: s.cfg.ItemCap,
		Sessions: len(s.sessions), DroppedCitations: droppedCites, DroppedVerifiedFacts: droppedFacts}
	return out, nil
}

func validateOutcome(out *investigate.AgentOutcome) []error {
	var errs []error
	for _, h := range out.Hypotheses {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, a := range out.Assertions {
		if err := a.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, r := range out.EvidenceRequests {
		if err := r.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, r := range out.ValidationRequests {
		if err := r.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, c := range out.Claims {
		if err := c.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, v := range out.Verdicts {
		if err := v.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// MergeGraph returns the graph plus the agent's proposed relationships: every
// proposed edge (always inferred) and the placeholder nodes its new ends
// need. Traversing it can only add unresolved paths (DESIGN.md §5.1).
func MergeGraph(g *domain.RelationshipGraph, out *investigate.AgentOutcome) *domain.RelationshipGraph {
	if g == nil {
		return nil
	}
	m := *g
	m.Nodes = append([]domain.GraphNode(nil), g.Nodes...)
	m.Edges = append([]domain.GraphEdge(nil), g.Edges...)
	if out == nil {
		return &m
	}
	nodes := map[string]bool{}
	for _, n := range m.Nodes {
		nodes[n.ID] = true
	}
	edges := map[string]bool{}
	for _, e := range m.Edges {
		edges[e.ID] = true
	}
	added := false
	for _, n := range out.ProposedNodes {
		if !nodes[n.ID] {
			nodes[n.ID] = true
			m.Nodes = append(m.Nodes, n)
			added = true
		}
	}
	for _, a := range out.Assertions {
		if e := a.ProposedEdge; e != nil && !edges[e.ID] && nodes[e.From] && nodes[e.To] {
			edges[e.ID] = true
			m.Edges = append(m.Edges, *e)
			added = true
		}
	}
	if !added {
		return &m
	}
	sort.Slice(m.Nodes, func(i, j int) bool { return m.Nodes[i].ID < m.Nodes[j].ID })
	sort.Slice(m.Edges, func(i, j int) bool { return m.Edges[i].ID < m.Edges[j].ID })
	m.Builder = g.Builder + "+" + Producer
	m.ID = m.DerivedID()
	return &m
}

// coverage reports, for every agenda item in agenda order, how the agent
// closed it; an item without a closing entry is not-examined, saying how the
// run ended. Caller holds s.mu.
func (s *Session) coverage(status domain.AgentRunStatus, reason string) []domain.AgendaCoverage {
	if len(s.cfg.Agenda) == 0 {
		return nil
	}
	records := map[string][]string{}
	for _, h := range s.hyps {
		records[h.ChangeItem] = append(records[h.ChangeItem], h.ID)
	}
	for _, a := range s.asserts {
		records[a.ChangeItem] = append(records[a.ChangeItem], a.ID)
	}
	out := make([]domain.AgendaCoverage, 0, len(s.cfg.Agenda))
	for _, e := range s.cfg.Agenda {
		c := e
		st := s.itemStats[e.ChangeItem]
		switch sum, ok := s.closing[e.ChangeItem]; {
		case ok:
			c.Disposition, c.Summary = sum.Disposition, sum.Summary
			c.Assertion, c.Citations = s.closingIDs[e.ChangeItem], s.closingEv[e.ChangeItem]
			c.SummaryKind = s.proseKind()
		case s.cfg.Prompt.Features.PerItem && st == nil:
			c.Disposition = domain.CoverNotExamined
			c.Summary = fmt.Sprintf("beyond cap %d: no session (closed by the runner, not by the model)", s.cfg.ItemCap)
		case st != nil:
			c.Disposition = domain.CoverNotExamined
			c.Summary = "no closing entry: the item's session ended " + string(st.Status)
			if st.Reason != "" {
				c.Summary += " (" + st.Reason + ")"
			}
		default:
			c.Disposition = domain.CoverNotExamined
			c.Summary = "no closing entry: the run ended " + string(status)
			if reason != "" {
				c.Summary += " (" + reason + ")"
			}
		}
		if st != nil {
			c.CallID, c.SessionStatus, c.SessionReason = st.CallID, st.Status, st.Reason
			c.Turns, c.ToolCalls, c.Tokens = st.Turns, st.ToolCalls, st.Tokens
		}
		c.Dossier = s.dossiers[e.ChangeItem]
		for _, v := range s.verdicts {
			if v.ChangeItem == e.ChangeItem {
				c.Verdict = v.ID
			}
		}
		c.Records = uniqSorted(records[e.ChangeItem])
		c.Examined, c.Inspections = s.examined[e.ChangeItem], s.inspections[e.ChangeItem]
		for _, r := range s.valReqs {
			if r.ChangeItem == e.ChangeItem {
				c.ValidationRequests++
			}
		}
		out = append(out, c)
	}
	return out
}

// ApplyAnswerLinks adds the outcome's answer links to the validation records
// they name (record Answers, deduplicated and sorted) and returns the
// records. The agent's own records already carry their links; the runner
// applies this to the deterministic pass's records before classify, so a
// concern answered by an existing validation is lifted (DESIGN.md §3.1 R2.6).
func ApplyAnswerLinks(records []domain.ValidationRecord, links []investigate.AnswerLink) []domain.ValidationRecord {
	add := map[string][]string{}
	for _, l := range links {
		add[l.Validation] = append(add[l.Validation], l.Assertions...)
	}
	out := make([]domain.ValidationRecord, len(records))
	for i, r := range records {
		if extra, ok := add[r.ID]; ok {
			r.Answers = uniqSorted(append(append([]string(nil), r.Answers...), extra...))
		}
		out[i] = r
	}
	return out
}

// investigateItems runs one short, focused session per agenda item in
// ItemAgenda order, up to the item cap (DESIGN-facing: docs/INVESTIGATE.md
// "Per-item sessions"). Items beyond the cap get no session; the runner (this
// function, never the model) closes them not-examined.
func (i *Investigator) investigateItems(ctx context.Context, in investigate.InvestigateInput, driver investigate.AgentDriver, s *Session, base investigate.AgentSession) (*investigate.AgentOutcome, error) {
	p, agenda, cap := s.cfg.Prompt, s.cfg.Agenda, s.cfg.ItemCap
	n := len(agenda)
	if n > cap {
		n = cap
	}
	var digests []string
	agg := investigate.DriverResult{Provider: ""}
	started := time.Now() // wall time is a cost metric, never part of a record's identity
	consumers, err := s.LoadConsumers(ctx)
	if err != nil {
		return nil, investigate.TransientError(domain.StageInvestigate, err)
	}
	for k := 0; k < n; k++ {
		e := agenda[k]
		var dossier *Dossier
		var dossierBytes []byte
		var dossierRef domain.ArtifactRef
		if p.Features.Dossier {
			var err error
			if dossier, dossierBytes, dossierRef, err = itemDossier(ctx, i.st, in, consumers, e.ChangeItem); err != nil {
				return nil, err
			}
		}
		task, err := p.renderItemTask(in, e, k+1, n, cap, base.Budget, dossierBytes, dossierRef.ID)
		if err != nil {
			return nil, investigate.PermanentError(domain.StageInvestigate, err)
		}
		digest := p.Digest(task)
		digests = append(digests, digest)
		s.BeginItem(e.ChangeItem, base.Budget, digest)
		if dossier != nil {
			s.StartFrom(dossier, dossierRef)
		}
		as := base
		as.Task, as.PromptDigest, as.ChangeItem = task, digest, e.ChangeItem
		runCtx, cancel := context.WithTimeout(ctx, time.Duration(base.Budget.MaxSeconds)*time.Second)
		dr, err := driver.Run(runCtx, as, s)
		cancel()
		if err != nil {
			switch {
			case ctx.Err() != nil:
				return nil, ctx.Err()
			case errors.Is(runCtx.Err(), context.DeadlineExceeded):
				dr = &investigate.DriverResult{Stop: "budget:seconds", Model: in.Config.Model}
				s.RecordEnvelope(in.Config.Driver, *dr, nil)
			default:
				return nil, err
			}
		}
		s.EndItem(*dr)
		if agg.Model == "" {
			agg.Model, agg.Provider, agg.CallID = dr.Model, dr.Provider, dr.CallID
		}
		if dr.ModelVersion != "" && !contains(strings.Split(agg.ModelVersion, ","), dr.ModelVersion) {
			agg.ModelVersion = strings.Trim(agg.ModelVersion+","+dr.ModelVersion, ",")
		}
		agg.Turns += dr.Turns
		agg.Tokens += dr.Tokens
	}
	s.mu.Lock()
	s.cfg.PromptDigest = domain.Digest([]byte(strings.Join(digests, "\n")))
	s.mu.Unlock()
	status, reason := s.aggregateStatus()
	out, err := s.outcome(ctx, i.st, in.Config.Driver, agg, status, reason)
	if err != nil {
		return nil, err
	}
	out.Run.WallMillis = time.Since(started).Milliseconds()
	return out, nil
}

// itemDossier builds and stores one item's dossier from the deterministic
// pass's view of the run (the same input every replay sees).
func itemDossier(ctx context.Context, st investigate.ArtifactStore, in investigate.InvestigateInput, consumers *domain.RelationshipGraph, item string) (*Dossier, []byte, domain.ArtifactRef, error) {
	d, err := BuildDossier(ctx, DossierInput{Bundle: in.Bundle, Graph: in.Graph, Traversal: in.Traversal, Report: in.Report,
		Validations: in.Validations, Read: StoreReader(st), Consumers: consumers}, item)
	if err != nil {
		return nil, nil, domain.ArtifactRef{}, investigate.TransientError(domain.StageInvestigate, fmt.Errorf("agent: dossier of %s: %w", item, err))
	}
	data, err := d.Encode()
	if err != nil {
		return nil, nil, domain.ArtifactRef{}, investigate.PermanentError(domain.StageInvestigate, err)
	}
	ref, err := StoreDossier(ctx, st, d)
	if err != nil {
		return nil, nil, domain.ArtifactRef{}, investigate.TransientError(domain.StageInvestigate, fmt.Errorf("agent: store dossier of %s: %w", item, err))
	}
	return d, data, ref, nil
}

// aggregateStatus is the worst per-item session status (failed >
// budget-exhausted > submitted), with the reasons of the non-submitted ones.
func (s *Session) aggregateStatus() (domain.AgentRunStatus, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rank := map[domain.AgentRunStatus]int{domain.AgentSubmitted: 0, domain.AgentBudgetExhausted: 1, domain.AgentFailed: 2}
	status := domain.AgentSubmitted
	var reasons []string
	for _, item := range s.sessions {
		st := s.itemStats[item]
		if st == nil {
			continue
		}
		if rank[st.Status] > rank[status] {
			status = st.Status
		}
		if st.Status != domain.AgentSubmitted {
			reasons = append(reasons, item+": "+st.Reason)
		}
	}
	if len(s.sessions) == 0 {
		return domain.AgentSubmitted, ""
	}
	return status, strings.Join(reasons, "; ")
}
