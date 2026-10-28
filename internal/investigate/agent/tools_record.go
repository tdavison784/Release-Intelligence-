package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

const (
	maxLocator = 300
	maxTarget  = 2000
	maxDetail  = 500
)

// noteView is what the model receives when a concern is recorded as an
// investigation-level note.
type noteView struct {
	Note       string `json:"note"`
	RecordedAs string `json:"recordedAs"`
	Reason     string `json:"reason"`
	Dropped    string `json:"dropped,omitempty"`
}

func (s *Session) addNote(n domain.AgentNote, evs []domain.Evidence) {
	for _, x := range s.notes {
		if x.ID == n.ID {
			return
		}
	}
	s.notes = append(s.notes, n)
	for _, e := range evs {
		s.addRunEvidence(e)
	}
}

// recordView is what the model receives for a recorded hypothesis or
// assertion: the id to refer to it by, and the evidence its citations became.
type recordView struct {
	ID           string              `json:"id"`
	ChangeItem   string              `json:"changeItem"`
	Stance       string              `json:"stance,omitempty"`
	Citations    []domain.EvidenceID `json:"citations"`
	ProposedEdge string              `json:"proposedEdge,omitempty"`
	Existing     bool                `json:"existing,omitempty"` // the same record was already made in this call
	// Mitigation says what became of a structured mitigation (feature mitigation).
	Mitigation string `json:"mitigation,omitempty"`
}

// mitigationState tells the model what became of the mitigation it gave.
func mitigationState(h domain.Hypothesis, given bool) string {
	switch {
	case !given:
		return ""
	case h.Mitigation != nil:
		return "recorded: proposed rule " + h.Mitigation.ProposedRule.Digest() + " (a proposal for independent verification; it changes no class)"
	case h.MitigationRejected != "":
		return "not recorded (the hypothesis is kept as free text): " + h.MitigationRejected + " — to retry, record the hypothesis again with the corrected mitigation and a revised statement"
	}
	return "not recorded: this hypothesis already exists without a mitigation; record a hypothesis with a revised statement"
}

// requestView is what the model receives for a request.
type requestView struct {
	ID         string               `json:"id"`
	Status     domain.RequestStatus `json:"status"`
	Resolution string               `json:"resolution,omitempty"`
	Result     string               `json:"result,omitempty"`
}

// validationResult is request_validation's result: the request and, when a
// validator ran, the record it produced.
type validationResult struct {
	Request    *domain.ValidationRequest `json:"-"`
	Validation *domain.ValidationRecord  `json:"-"`
}

type validationView struct {
	Request    requestView              `json:"request"`
	Subjects   []SubjectResolution      `json:"subjects,omitempty"` // how named subjects were resolved (v2)
	Answers    []answerResult           `json:"answers,omitempty"`  // which assertions were linked (v3)
	Validation *domain.ValidationRecord `json:"validation,omitempty"`
}

// --- citations (DESIGN.md §4.4) ---------------------------------------------------------

// citations checks every citation — the artifact exists in the manifest or
// the run, the quote occurs in its bytes (whitespace-normalised) — and turns
// each into an evidence record (kind by origin, URI from the artifact,
// contentDigest = the artifact digest). Any failure rejects the whole call.
func (s *Session) citations(ctx context.Context, cs []investigate.Citation) ([]domain.Evidence, error) {
	if len(cs) == 0 {
		return nil, rejectf("at least one citation is required: an uncited claim is not recorded")
	}
	if len(cs) > MaxCitations {
		return nil, rejectf("%d citations (max %d)", len(cs), MaxCitations)
	}
	out := make([]domain.Evidence, 0, len(cs))
	for i, c := range cs {
		a, ok := s.artifact(c.Artifact)
		if !ok {
			return nil, rejectf("citation %d: unknown artifact %q", i+1, c.Artifact)
		}
		if trimmed(c.Quote) == "" {
			return nil, rejectf("citation %d: the quote is empty", i+1)
		}
		if len(c.Quote) > MaxQuote || len(c.Locator) > maxLocator {
			return nil, rejectf("citation %d: quote (max %d bytes) or locator (max %d) too long", i+1, MaxQuote, maxLocator)
		}
		text, err := s.textOf(ctx, a)
		if err != nil {
			return nil, err
		}
		if !investigate.QuoteOccurs(text, c.Quote) {
			return nil, rejectf("citation %d: the quote does not occur in artifact %s (%s); copy it verbatim from the artifact", i+1, a.ID, a.Path)
		}
		out = append(out, domain.NewEvidence(evidenceKind(a), a.ID, a.URI, c.Locator, c.Quote, a.Digest, time.Time{}))
	}
	return out, nil
}

// evidenceKind classifies citation evidence by the cited artifact.
func evidenceKind(a domain.ArtifactRef) domain.EvidenceKind {
	switch a.Origin {
	case domain.OriginEnvironment:
		return domain.EvidenceLocalFile
	case domain.OriginUpstream:
		switch a.Kind {
		case domain.ArtifactChartValues, domain.ArtifactCRDDefinition:
			return domain.EvidenceStructured
		}
		return domain.EvidenceDocument
	}
	switch a.Kind {
	case domain.ArtifactRenderedManifests:
		return domain.EvidenceRendered
	case domain.ArtifactRenderDiff:
		return domain.EvidenceRenderedDiff
	}
	return domain.EvidenceStructured
}

func evidenceIDs(es []domain.Evidence) []domain.EvidenceID {
	var out []domain.EvidenceID
	seen := map[domain.EvidenceID]bool{}
	for _, e := range es {
		if !seen[e.ID] {
			seen[e.ID] = true
			out = append(out, e.ID)
		}
	}
	return out
}

func (s *Session) statement(what, v string) (string, error) {
	v = trimmed(v)
	if v == "" {
		return "", rejectf("%s is required", what)
	}
	if len(v) > MaxStatement {
		return "", rejectf("%s is %d bytes (max %d)", what, len(v), MaxStatement)
	}
	return v, nil
}

// --- record_hypothesis ------------------------------------------------------------------

func (s *Session) recordHypothesis(ctx context.Context, in investigate.HypothesisInput) (*outcome, error) {
	if err := s.writable(); err != nil {
		return nil, err
	}
	it, err := s.item(in.ChangeItem)
	if err != nil {
		return nil, err
	}
	stmt, err := s.statement("statement", in.Statement)
	if err != nil {
		return nil, err
	}
	subjects, _, err := s.resolveSubjects(ctx, "subject", in.Subjects)
	if err != nil {
		return nil, err
	}
	evs, err := s.citations(ctx, in.Citations)
	if err != nil {
		return nil, err
	}
	if in.Mitigation != nil && !s.cfg.Prompt.Features.Mitigation {
		return nil, rejectf("this prompt version (%s) takes no structured mitigation", s.cfg.Prompt.Version)
	}
	id := domain.AgentRecordID(domain.HypothesisIDPrefix, it.ID, stmt, s.call.ID)
	for i := range s.hyps {
		if s.hyps[i].ID == id {
			h := s.hyps[i]
			return &outcome{value: &h, view: recordView{ID: id, ChangeItem: it.ID, Citations: h.Citations, Existing: true,
				Mitigation: mitigationState(h, in.Mitigation != nil)}}, nil
		}
	}
	cites := evidenceIDs(evs)
	h := domain.Hypothesis{ID: id, ChangeItem: it.ID, Statement: stmt, Subjects: subjects, Citations: cites, StatementKind: s.proseKind(),
		Provenance: s.aiProvenance(append(append([]domain.EvidenceID(nil), cites...), it.Evidence...))}
	if in.Mitigation != nil {
		h.Mitigation, h.MitigationRejected = s.mitigationSpec(ctx, it, *in.Mitigation, evs)
	}
	if err := invalid(func() error { c := h; c.Provenance = placeholder(c.Provenance); return c.Validate() }()); err != nil {
		return nil, err
	}
	return &outcome{value: &h, view: recordView{ID: id, ChangeItem: it.ID, Citations: cites, Mitigation: mitigationState(h, in.Mitigation != nil)}, commit: func() {
		s.hyps = append(s.hyps, h)
		s.cite(it.ID, evs)
		for _, e := range evs {
			s.addRunEvidence(e)
		}
	}}, nil
}

// placeholder fills the model fields the claude-cli driver reports only when
// the session ends, so a record can be validated when it is made; the real
// values replace them before the outcome is returned (Investigator).
func placeholder(p domain.Provenance) domain.Provenance {
	if p.Model == "" {
		p.Model = unreportedModelInfo
	}
	if p.ModelVersion == "" {
		p.ModelVersion = unreportedModelInfo
	}
	return p
}

func invalid(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("agent: invalid record: %w", err)
}

// --- record_assertion -------------------------------------------------------------------

func (s *Session) recordAssertion(ctx context.Context, in investigate.AssertionInput) (*outcome, error) {
	if err := s.writable(); err != nil {
		return nil, err
	}
	it, err := s.item(in.ChangeItem)
	if err != nil {
		return nil, err
	}
	switch in.Stance {
	case domain.StanceRaisesConcern, domain.StanceExplains, domain.StanceClaimsMitigation:
	default:
		return nil, rejectf("stance %q: want raises-concern, explains or claims-mitigation", in.Stance)
	}
	stmt, err := s.statement("statement", in.Statement)
	if err != nil {
		return nil, err
	}
	subjects, _, err := s.resolveSubjects(ctx, "subject", in.Subjects)
	if err != nil {
		return nil, err
	}
	paths := uniqSorted(in.Paths)
	for _, p := range paths {
		if _, ok := s.paths[p]; !ok {
			return nil, rejectf("unknown impact path %q", p)
		}
	}
	rcs, err := s.renderedIDs(in.RenderedChanges)
	if err != nil {
		return nil, err
	}
	if in.Hypothesis != "" && !s.hasHypothesis(in.Hypothesis) {
		return nil, rejectf("unknown hypothesis %q (record it with record_hypothesis first)", in.Hypothesis)
	}
	evs, err := s.citations(ctx, in.Citations)
	if err != nil {
		return nil, err
	}
	reason, err := s.noValidationReason(in.Stance == domain.StanceRaisesConcern, in.NoValidationReason)
	if err != nil {
		return nil, err
	}
	claimIDs, err := s.claimRefs(it.ID, in.Claims)
	if err != nil {
		return nil, err
	}
	if in.Stance == domain.StanceExplains {
		if err := s.depthExplains("record_assertion", it.ID, evs, nil); err != nil {
			return nil, err
		}
	}
	if in.Stance == domain.StanceRaisesConcern {
		if ok, why := s.itemEvidence(it.ID, evs); !ok {
			n, err := s.note(it.ID, stmt, why, evs)
			if err != nil {
				return nil, err
			}
			view := noteView{Note: n.ID, RecordedAs: "investigation-level note (AI), not an item concern", Reason: why}
			if in.ProposedEdge != nil {
				view.Dropped = "the proposed edge was not recorded: it needs an item concern"
			}
			return &outcome{value: (*domain.AgentAssertion)(nil), view: view, commit: func() { s.addNote(n, evs) }}, nil
		}
	}
	id := domain.AgentRecordID(domain.AssertionIDPrefix, it.ID, string(in.Stance)+"\x00"+stmt, s.call.ID)
	for i := range s.asserts {
		if s.asserts[i].ID == id {
			a := s.asserts[i]
			return &outcome{value: &a, view: recordView{ID: id, ChangeItem: it.ID, Stance: string(a.Stance), Citations: a.Citations, Existing: true}}, nil
		}
	}
	var edge *domain.GraphEdge
	var newNodes []domain.GraphNode
	if in.ProposedEdge != nil {
		if edge, newNodes, err = s.proposedEdge(*in.ProposedEdge, id); err != nil {
			return nil, err
		}
	}
	cites := evidenceIDs(evs)
	a := domain.AgentAssertion{ID: id, ChangeItem: it.ID, Stance: in.Stance, Statement: stmt, Subjects: subjects, Paths: paths,
		RenderedChanges: rcs, Citations: cites, ProposedEdge: edge, Hypothesis: in.Hypothesis, NoValidationReason: reason,
		StatementKind: s.proseKind(), Claims: claimIDs,
		Provenance: s.aiProvenance(append(append([]domain.EvidenceID(nil), cites...), it.Evidence...))}
	if err := invalid(func() error { c := a; c.Provenance = placeholder(c.Provenance); return c.Validate() }()); err != nil {
		return nil, err
	}
	view := recordView{ID: id, ChangeItem: it.ID, Stance: string(a.Stance), Citations: cites}
	if edge != nil {
		view.ProposedEdge = edge.ID
	}
	return &outcome{value: &a, view: view, commit: func() {
		s.asserts = append(s.asserts, a)
		s.cite(it.ID, evs)
		for _, e := range evs {
			s.addRunEvidence(e)
		}
		for _, n := range newNodes {
			s.nodes[n.ID] = n
			if _, ok := s.nodeKey[n.Key]; !ok {
				s.nodeKey[n.Key] = n.ID
			}
			s.propNodes = append(s.propNodes, n)
		}
		if edge != nil {
			if _, ok := s.edges[edge.ID]; !ok {
				s.edges[edge.ID] = *edge
			}
		}
	}}, nil
}

func (s *Session) hasHypothesis(id string) bool {
	for _, h := range s.hyps {
		if h.ID == id {
			return true
		}
	}
	return false
}

// proposedEdge builds the inferred edge an assertion proposes. An end is a
// node id, a node key, or "<node kind>:<key>" — a placeholder node when the
// graph has no such node.
func (s *Session) proposedEdge(pe investigate.ProposedEdge, assertion string) (*domain.GraphEdge, []domain.GraphNode, error) {
	if s.cfg.Graph == nil {
		return nil, nil, rejectf("proposedEdge: this investigation has no relationship graph")
	}
	if !edgeKindKnown(pe.Kind) {
		return nil, nil, rejectf("proposedEdge: unknown edge kind %q", pe.Kind)
	}
	reason := trimmed(pe.Reason)
	if reason == "" {
		return nil, nil, rejectf("proposedEdge: a reason is required (a proposed edge is inferred)")
	}
	if len(pe.Detail) > maxDetail || len(reason) > MaxStatement {
		return nil, nil, rejectf("proposedEdge: detail (max %d bytes) or reason too long", maxDetail)
	}
	var created []domain.GraphNode
	end := func(what, ref string) (string, error) {
		if id, ok := s.node(ref); ok {
			return id, nil
		}
		kind, key, ok := strings.Cut(ref, ":")
		if ok && key != "" && nodeKindKnown(domain.NodeKind(kind)) {
			id := domain.NodeID(domain.NodeKind(kind), key)
			if _, exists := s.nodes[id]; exists {
				return id, nil
			}
			for _, n := range created {
				if n.ID == id {
					return id, nil
				}
			}
			created = append(created, domain.GraphNode{ID: id, Kind: domain.NodeKind(kind), Key: key, Placeholder: true})
			return id, nil
		}
		return "", rejectf("proposedEdge.%s %q: not a node id or key of the graph, nor <node kind>:<key>", what, ref)
	}
	from, err := end("from", pe.From)
	if err != nil {
		return nil, nil, err
	}
	to, err := end("to", pe.To)
	if err != nil {
		return nil, nil, err
	}
	if from == to {
		return nil, nil, rejectf("proposedEdge: from and to are the same node")
	}
	id := domain.EdgeID(pe.Kind, from, to, pe.Detail)
	for _, e := range s.cfg.Graph.Edges {
		if e.ID == id {
			return nil, nil, rejectf("proposedEdge: the graph already has this edge (%s, status %s)", id, e.Status)
		}
	}
	return &domain.GraphEdge{ID: id, Kind: pe.Kind, From: from, To: to, Status: domain.EdgeInferred, Rule: ProposedEdgeRule,
		Detail: pe.Detail, Reason: reason, Assertion: assertion}, created, nil
}

func nodeKindKnown(k domain.NodeKind) bool {
	for _, x := range domain.NodeKinds {
		if x == k {
			return true
		}
	}
	return false
}

// --- request_evidence -------------------------------------------------------------------

func (s *Session) requestEvidence(ctx context.Context, in investigate.EvidenceRequestInput) (*outcome, error) {
	if err := s.writable(); err != nil {
		return nil, err
	}
	if s.cfg.Prompt.Features.Upstream.On() {
		switch {
		case in.Kind == domain.RequestUpstreamFile || in.Kind == domain.RequestUpstreamSearch:
			return s.requestUpstream(ctx, in)
		case upstreamFieldsSet(in):
			return nil, rejectf("repo, ref, path, dir, pattern and glob are for kinds upstream-file and upstream-search")
		}
	}
	var inputs []domain.EvidenceID
	if in.ChangeItem != "" {
		it, err := s.item(in.ChangeItem)
		if err != nil {
			return nil, err
		}
		inputs = it.Evidence
	} else if inputs = s.taskEvidence(); len(inputs) == 0 {
		return nil, rejectf("changeItem is required")
	}
	target, reason := trimmed(in.Target), trimmed(in.Reason)
	if target == "" || reason == "" {
		return nil, rejectf("target and reason are required")
	}
	if len(target) > maxTarget || len(reason) > MaxStatement {
		return nil, rejectf("target (max %d bytes) or reason too long", maxTarget)
	}
	status, resolution := domain.RequestOpen, ""
	switch in.Kind {
	case domain.RequestLiveObservation:
		status, resolution = domain.RequestRejected, "live-cluster observation is disabled in milestone 1 (DESIGN.md §1.2)"
	case domain.RequestSource:
		if _, ok := s.cfg.Bundle.Change.SourceStatus(domain.ChangeSourceKind(target)); !ok {
			return nil, rejectf("target %q is not a change source kind (%s)", target, sourceKinds())
		}
	case domain.RequestArtifact, domain.RequestEnvironmentInput:
		for _, a := range s.allArtifacts() {
			if a.ID == target || a.Path == target || a.URI == target {
				return nil, rejectf("%s is already in the investigation as artifact %s: read it with read_artifact", target, a.ID)
			}
		}
	default:
		if s.cfg.Prompt.Features.Upstream.On() {
			return nil, rejectf("kind %q: want upstream-file, upstream-search, artifact, source, environment-input or live-observation", in.Kind)
		}
		return nil, rejectf("kind %q: want artifact, source, environment-input or live-observation", in.Kind)
	}
	id := domain.AgentRecordID(domain.EvidenceRequestIDPrefix, in.ChangeItem, string(in.Kind)+"\x00"+target, s.call.ID)
	for i := range s.evReqs {
		if s.evReqs[i].ID == id {
			r := s.evReqs[i]
			return &outcome{value: &r, view: requestView{ID: id, Status: r.Status, Resolution: r.Resolution}}, nil
		}
	}
	r := domain.EvidenceRequest{ID: id, ChangeItem: in.ChangeItem, Kind: in.Kind, Target: target, Reason: reason, Status: status,
		Resolution: resolution, Provenance: s.aiProvenance(inputs)}
	if err := invalid(func() error { c := r; c.Provenance = placeholder(c.Provenance); return c.Validate() }()); err != nil {
		return nil, err
	}
	view := requestView{ID: id, Status: status, Resolution: resolution}
	if status == domain.RequestOpen {
		view.Resolution = "recorded for the next collection round; nothing is fetched during this session"
	}
	return &outcome{value: &r, view: view, commit: func() { s.evReqs = append(s.evReqs, r) }}, nil
}

func sourceKinds() string {
	var out []string
	for _, k := range domain.ChangeSourceKinds {
		out = append(out, string(k))
	}
	return strings.Join(out, ", ")
}

// --- request_validation -----------------------------------------------------------------

func (s *Session) requestValidation(ctx context.Context, in investigate.ValidationRequestInput) (*outcome, error) {
	if err := s.writable(); err != nil {
		return nil, err
	}
	it, err := s.item(in.ChangeItem)
	if err != nil {
		return nil, err
	}
	if !it.InScope {
		return nil, rejectf("change item %s is out of scope (%s): it is not classified, so it is not validated", it.ID, it.ScopeRule)
	}
	known := false
	for _, k := range domain.ValidationKinds {
		known = known || k == in.Kind
	}
	if !known {
		return nil, rejectf("kind %q: want override-inspection, render-diff, schema-compat, static-reference or runtime", in.Kind)
	}
	subjects, resolved, err := s.resolveSubjects(ctx, "subject", in.Subjects)
	if err != nil {
		return nil, err
	}
	paths := uniqSorted(in.Paths)
	for _, p := range paths {
		pp, ok := s.paths[p]
		if !ok {
			return nil, rejectf("unknown impact path %q", p)
		}
		if pp.ChangeItem != it.ID {
			return nil, rejectf("impact path %s belongs to change item %s, not %s", p, pp.ChangeItem, it.ID)
		}
	}
	named := append([]string(nil), in.RenderedChanges...)
	for _, r := range resolved {
		if r.Rule == SubjectRenderedChange {
			named = append(named, r.Given)
		}
	}
	rcs, err := s.renderedIDs(named)
	if err != nil {
		return nil, err
	}
	inputs := uniqSorted(in.Inputs)
	for _, a := range inputs {
		if _, ok := s.artifact(a); !ok {
			return nil, rejectf("unknown input artifact %q", a)
		}
	}
	if in.Hypothesis != "" && !s.hasHypothesis(in.Hypothesis) {
		return nil, rejectf("unknown hypothesis %q (record it with record_hypothesis first)", in.Hypothesis)
	}
	answers, err := s.answerCandidates(it.ID, in.Answers, in.Hypothesis, subjects, paths, rcs, len(in.Subjects)+len(in.Paths)+len(in.RenderedChanges) == 0)
	if err != nil {
		return nil, err
	}
	id := domain.AgentRecordID(domain.ValidationRequestIDPrefix, it.ID, string(in.Kind)+"\x00"+strings.Join(subjects, ","), s.call.ID)
	for i := range s.valReqs {
		if s.valReqs[i].ID == id {
			r := s.valReqs[i]
			res := validationResult{Request: &r}
			if v, ok := s.valIDs[r.Result]; ok {
				res.Validation = &v
			}
			return &outcome{value: res, view: validationView{Request: requestView{ID: id, Status: r.Status, Resolution: r.Resolution, Result: r.Result}, Validation: res.Validation}}, nil
		}
	}
	evIn := append([]domain.EvidenceID(nil), it.Evidence...)
	for _, rc := range rcs {
		evIn = append(evIn, s.rcs[rc].Evidence...)
	}
	req := domain.ValidationRequest{ID: id, ChangeItem: it.ID, Kind: in.Kind, Subjects: subjects, Inputs: inputs,
		Hypothesis: in.Hypothesis, Answers: uniqSorted(in.Answers), Status: domain.RequestOpen, Provenance: s.aiProvenance(evIn)}

	var applicable []investigate.Validator
	var configured []string
	for _, v := range s.cfg.Validators {
		configured = append(configured, fmt.Sprintf("%s (%s)", v.Name(), v.Kind()))
		if v.Kind() == in.Kind && v.Applies(it.Family) {
			applicable = append(applicable, v)
		}
	}
	sort.Slice(applicable, func(i, j int) bool { return applicable[i].Name() < applicable[j].Name() })
	sort.Strings(configured)

	var (
		fresh    []domain.ValidationRecord
		existing *domain.ValidationRecord
		arts     []domain.ArtifactRef
		evs      []domain.Evidence
		problems []string
	)
	for _, v := range applicable {
		r := req
		out, err := v.Validate(ctx, investigate.ValidationInput{Bundle: s.cfg.Bundle, Graph: s.cfg.Graph, Traversal: s.cfg.Traversal,
			Report: s.cfg.Report, ChangeItem: it.ID, Subjects: subjects, Paths: paths, RenderedChanges: rcs, Request: &r})
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			problems = append(problems, fmt.Sprintf("validator %s failed: %v", v.Name(), err))
			continue
		}
		if out == nil {
			continue
		}
		for _, rec := range out.Records {
			if rec.Request == "" {
				rec.Request = id
			}
			switch {
			case rec.ChangeItem != it.ID || rec.Request != id:
				problems = append(problems, fmt.Sprintf("validator %s returned a record for another item or request (%s)", v.Name(), rec.ID))
				continue
			case rec.Provenance.Method == domain.MethodAI:
				problems = append(problems, fmt.Sprintf("validator %s returned an AI record (%s); a validation is never AI output", v.Name(), rec.ID))
				continue
			}
			if err := rec.Validate(); err != nil {
				problems = append(problems, fmt.Sprintf("validator %s returned an invalid record: %v", v.Name(), err))
				continue
			}
			if prev, ok := s.valIDs[rec.ID]; ok {
				if existing == nil {
					p := prev
					existing = &p
				}
				continue
			}
			dup := false
			for _, f := range fresh {
				dup = dup || f.ID == rec.ID
			}
			if !dup {
				fresh = append(fresh, rec)
			}
		}
		for _, a := range out.Artifacts {
			if err := a.Validate(); err != nil {
				problems = append(problems, fmt.Sprintf("validator %s returned an invalid artifact: %v", v.Name(), err))
				continue
			}
			arts = append(arts, a)
		}
		evs = append(evs, out.Evidence...)
	}
	res := validationResult{Request: &req}
	switch {
	case len(applicable) == 0:
		req.Status = domain.RequestUnavailable
		cfg := "none"
		if len(configured) > 0 {
			cfg = strings.Join(configured, ", ")
		}
		req.Resolution = fmt.Sprintf("no %s validator applies to family %s (configured: %s)", in.Kind, it.Family, cfg)
	case len(fresh) > 0:
		req.Status, req.Result = domain.RequestFulfilled, fresh[0].ID
		res.Validation = &fresh[0]
	case existing != nil:
		req.Status = domain.RequestRejected
		req.Resolution = fmt.Sprintf("already validated: %s by %s (outcome %s); the same validation is not recorded twice", existing.ID, existing.Validator, existing.Outcome)
		res.Validation = existing
	default:
		req.Status = domain.RequestUnavailable
		req.Resolution = "the validator produced no record"
		if len(problems) > 0 {
			req.Resolution = strings.Join(problems, "; ")
		}
	}
	if err := invalid(func() error { c := req; c.Provenance = placeholder(c.Provenance); return c.Validate() }()); err != nil {
		return nil, err
	}
	// Link the assertions the request answers into the record (guarded: the
	// record must cover what each assertion names). The agent's own record
	// carries the link; a link to an existing record (the deterministic
	// pass's) is reported for the runner to apply.
	var links []answerResult
	var link *investigate.AnswerLink
	if target := res.Validation; target != nil && len(answers) > 0 {
		var linked []string
		for _, a := range answers {
			r := answerResult{Assertion: a.ID, Linked: true}
			if why := answerGuard(a, *target, s.cfg.Traversal); why != "" {
				r.Linked, r.Reason = false, why
			} else {
				linked = append(linked, a.ID)
			}
			links = append(links, r)
		}
		if len(linked) > 0 {
			link = &investigate.AnswerLink{Validation: target.ID, Request: id, Assertions: uniqSorted(linked)}
			if len(fresh) > 0 && target.ID == fresh[0].ID {
				fresh[0].Answers = uniqSorted(append(fresh[0].Answers, linked...))
				v := fresh[0]
				res.Validation = &v
			}
		}
	} else if len(answers) > 0 {
		for _, a := range answers {
			links = append(links, answerResult{Assertion: a.ID, Reason: "no validation record was produced"})
		}
	}
	view := validationView{Request: requestView{ID: id, Status: req.Status, Resolution: req.Resolution, Result: req.Result}, Subjects: resolved,
		Answers: links, Validation: res.Validation}
	return &outcome{value: res, view: view, commit: func() {
		if link != nil {
			s.links = append(s.links, *link)
		}
		s.valReqs = append(s.valReqs, req)
		for _, v := range fresh {
			s.vals = append(s.vals, v)
			s.valIDs[v.ID] = v
		}
		for _, a := range arts {
			s.addRunArtifact(a)
		}
		for _, e := range evs {
			s.addRunEvidence(e)
		}
	}}, nil
}

// --- submit_findings --------------------------------------------------------------------

type submitView struct {
	Submitted  bool          `json:"submitted"`
	Assertions []string      `json:"assertions"`
	Note       string        `json:"note"`
	Agenda     *agendaView   `json:"agenda,omitempty"`   // prompt feature agenda
	Verdicts   []verdictView `json:"verdicts,omitempty"` // prompt feature verdict
	// Assessments: prompt feature assessment.
	Assessments []assessmentView `json:"assessments,omitempty"`
}

// verdictView is what the model receives for a recorded verdict: what
// resolved and what was dropped.
type verdictView struct {
	ID                   string              `json:"id"`
	ChangeItem           string              `json:"changeItem"`
	Class                domain.FindingClass `json:"class"`
	Citations            int                 `json:"citations"`
	DroppedCitations     int                 `json:"droppedCitations,omitempty"`
	DroppedVerifiedFacts int                 `json:"droppedVerifiedFacts,omitempty"`
	Dropped              []string            `json:"dropped,omitempty"`
}

type agendaView struct {
	Items        int            `json:"items"`
	Dispositions map[string]int `json:"dispositions"`
}

// stanceOf is the assertion stance an agenda disposition is recorded with
// (examined-no-concern is an explains assertion: never a mitigation).
func stanceOf(d domain.CoverageDisposition) (domain.AssertionStance, bool) {
	switch d {
	case domain.CoverRaisesConcern:
		return domain.StanceRaisesConcern, true
	case domain.CoverClaimsMitigation:
		return domain.StanceClaimsMitigation, true
	case domain.CoverExplains, domain.CoverExaminedNoConcern:
		return domain.StanceExplains, true
	}
	return "", false
}

func (s *Session) submitFindings(ctx context.Context, in investigate.SubmitInput) (*outcome, error) {
	if err := s.writable(); err != nil {
		return nil, err
	}
	agenda := s.cfg.Prompt.Features.Agenda
	perItem := s.cfg.Prompt.Features.PerItem
	if perItem && (len(in.Items) != 1 || in.Items[0].ChangeItem != s.focus) {
		return nil, rejectf("this session closes exactly one item, %s: submit_findings needs one entry for it", s.focus)
	}
	var notes []domain.AgentNote
	var verdicts []domain.AgentVerdict
	// pendingAssessment is an entry's assessment, built after every other
	// check of the call passed (the completion check rejects only once).
	type pendingAssessment struct {
		at        string
		it        domain.ChangeItem
		in        investigate.AssessmentInput
		verdict   *domain.AgentVerdict
		behaviour behaviourDeps
	}
	var pendingAssessments []pendingAssessment
	seen := map[string]bool{}
	var made []domain.AgentAssertion
	var evs []domain.Evidence
	ids := []string{}
	closing := map[string]investigate.ItemSummary{}
	closingIDs := map[string]string{}
	closingEv := map[string][]domain.EvidenceID{}
	counts := map[string]int{}
	type explainsEntry struct {
		i    int
		item string
	}
	var explainsEntries []explainsEntry
	entryEv := map[string][]domain.Evidence{} // item -> the entry's checked citations
	for i, sum := range in.Items {
		it, err := s.item(sum.ChangeItem)
		if err != nil {
			return nil, fmt.Errorf("items[%d]: %w", i, err)
		}
		if !it.InScope {
			return nil, rejectf("items[%d]: change item %s is out of scope (%s)", i, it.ID, it.ScopeRule)
		}
		if seen[it.ID] {
			return nil, rejectf("items[%d]: change item %s is summarised twice", i, it.ID)
		}
		seen[it.ID] = true
		disp := sum.Disposition
		switch {
		case !agenda && disp != "":
			return nil, rejectf("items[%d]: this prompt version (%s) has no agenda; dispositions are not accepted", i, s.cfg.Prompt.Version)
		case !agenda:
			disp = domain.CoverExplains
		case disp == "":
			return nil, rejectf("items[%d]: disposition is required: raises-concern, claims-mitigation, explains, examined-no-concern or not-examined", i)
		}
		stance, closes := stanceOf(disp)
		if !closes && disp != domain.CoverNotExamined {
			return nil, rejectf("items[%d]: unknown disposition %q: want raises-concern, claims-mitigation, explains, examined-no-concern or not-examined", i, disp)
		}
		what := fmt.Sprintf("items[%d].summary", i)
		if !closes {
			what = fmt.Sprintf("items[%d].summary (the reason it was not examined)", i)
		}
		stmt, err := s.statement(what, sum.Summary)
		if err != nil {
			return nil, err
		}
		counts[string(disp)]++
		sum.Disposition, sum.Summary = disp, stmt
		closing[it.ID] = sum
		if s.cfg.Prompt.Features.Verdict {
			switch {
			case sum.Verdict == nil && disp != domain.CoverNotExamined:
				return nil, rejectf("items[%d]: verdict is required: your provisional class (affected, not-affected, potentially-affected, insufficient-evidence), mechanism, environmentReasoning, citations, verifiedFacts, assumptions, mitigations, uncertainty, proposedValidations, wouldOverturn", i)
			case sum.Verdict != nil:
				// A verdict stating no citations takes the entry's, checked here
				// exactly as below (an unchecked one rejects the call).
				var inherit []domain.Evidence
				if s.cfg.Prompt.Features.VerdictInheritsCitations && len(sum.Verdict.Citations) == 0 && len(sum.Citations) > 0 {
					if inherit, err = s.citations(ctx, sum.Citations); err != nil {
						return nil, fmt.Errorf("items[%d]: %w", i, err)
					}
				}
				v, vevs, err := s.verdictOf(ctx, it, *sum.Verdict, inherit)
				if err != nil {
					return nil, fmt.Errorf("items[%d]: %w", i, err)
				}
				verdicts = append(verdicts, v)
				evs = append(evs, vevs...)
			}
		} else if sum.Verdict != nil {
			return nil, rejectf("items[%d]: this prompt version (%s) takes no verdict", i, s.cfg.Prompt.Version)
		}
		switch {
		case !s.cfg.Prompt.Features.Assessment && sum.Assessment != nil:
			return nil, rejectf("items[%d]: this prompt version (%s) takes no assessment", i, s.cfg.Prompt.Version)
		case s.cfg.Prompt.Features.ScopedVerdict && sum.Assessment == nil && disp != domain.CoverNotExamined:
			return nil, rejectf("items[%d]: assessment is required: headline, scope, preconditions, whatChanged, mechanism, consumers, effectiveConfiguration, verified (each with citations), inferred, unknown, likelyImpact, recommendedTests", i)
		case s.cfg.Prompt.Features.Assessment && sum.Assessment == nil && disp != domain.CoverNotExamined:
			return nil, rejectf("items[%d]: assessment is required: whatChanged, mechanism, consumers, effectiveConfiguration, verified (each with citations), inferred, unknown, likelyImpact, recommendedTests", i)
		case sum.Assessment != nil:
			pa := pendingAssessment{at: fmt.Sprintf("items[%d].assessment", i), it: it, in: *sum.Assessment}
			if len(verdicts) > 0 && verdicts[len(verdicts)-1].ChangeItem == it.ID {
				pa.verdict = &verdicts[len(verdicts)-1]
			}
			if err := s.scopedChecks(pa.at, pa.in, pa.verdict); err != nil {
				return nil, err
			}
			if pa.behaviour, err = s.behaviourOf(ctx, pa.at, it, pa.in.Behaviour); err != nil {
				return nil, err
			}
			if err := s.fetchFirst(pa.at, it, pa.behaviour.list); err != nil {
				return nil, err
			}
			if err := behaviourVerdictCheck(pa.at, pa.verdict, pa.behaviour.list); err != nil {
				return nil, err
			}
			pendingAssessments = append(pendingAssessments, pa)
		}
		if !closes {
			if len(sum.Citations) > 0 { // optional; checked when given
				cs, err := s.citations(ctx, sum.Citations)
				if err != nil {
					return nil, fmt.Errorf("items[%d]: %w", i, err)
				}
				closingEv[it.ID] = evidenceIDs(cs)
				evs = append(evs, cs...)
				entryEv[it.ID] = cs
			}
			continue
		}
		cs, err := s.citations(ctx, sum.Citations)
		if err != nil {
			return nil, fmt.Errorf("items[%d]: %w", i, err)
		}
		reason, err := s.noValidationReason(disp == domain.CoverRaisesConcern, sum.NoValidationReason)
		if err != nil {
			return nil, fmt.Errorf("items[%d]: %w", i, err)
		}
		entryClaims, err := s.claimRefs(it.ID, sum.Claims)
		if err != nil {
			return nil, fmt.Errorf("items[%d]: %w", i, err)
		}
		if disp == domain.CoverRaisesConcern {
			if ok, why := s.itemEvidence(it.ID, cs); !ok {
				n, err := s.note(it.ID, stmt, why, cs)
				if err != nil {
					return nil, err
				}
				notes = append(notes, n)
				counts[string(disp)]--
				counts[string(domain.CoverNotExamined)]++
				sum.Disposition, sum.Summary = domain.CoverNotExamined, "concern recorded as investigation-level note "+n.ID+" ("+why+"): "+stmt
				closing[it.ID] = sum
				closingEv[it.ID] = evidenceIDs(cs)
				evs = append(evs, cs...)
				continue
			}
		}
		entryEv[it.ID] = cs
		if disp == domain.CoverExplains || disp == domain.CoverExaminedNoConcern {
			explainsEntries = append(explainsEntries, explainsEntry{i: i, item: it.ID})
		}
		id := domain.AgentRecordID(domain.AssertionIDPrefix, it.ID, string(stance)+"\x00"+stmt, s.call.ID)
		ids = append(ids, id)
		cites := evidenceIDs(cs)
		closingIDs[it.ID], closingEv[it.ID] = id, cites
		dup := false
		for _, a := range s.asserts {
			dup = dup || a.ID == id
		}
		for _, a := range made {
			dup = dup || a.ID == id
		}
		if dup {
			continue
		}
		a := domain.AgentAssertion{ID: id, ChangeItem: it.ID, Stance: stance, Statement: stmt, Citations: cites, NoValidationReason: reason,
			StatementKind: s.proseKind(), Claims: entryClaims,
			Provenance: s.aiProvenance(append(append([]domain.EvidenceID(nil), cites...), it.Evidence...))}
		if err := invalid(func() error { c := a; c.Provenance = placeholder(c.Provenance); return c.Validate() }()); err != nil {
			return nil, err
		}
		made = append(made, a)
		evs = append(evs, cs...)
	}
	// Depth rules: D1 over the explains / examined-no-concern entries (the
	// other entries of this call count as other items' citations), D2 over
	// every raises-concern.
	pending := map[string]map[string]bool{}
	for item, cs := range entryEv {
		for _, e := range cs {
			k := citeKey(e)
			if pending[k] == nil {
				pending[k] = map[string]bool{}
			}
			pending[k][item] = true
		}
	}
	for _, x := range explainsEntries {
		if err := s.depthExplains(fmt.Sprintf("items[%d]", x.i), x.item, entryEv[x.item], pending); err != nil {
			return nil, err
		}
	}
	if err := s.depthConcerns(made); err != nil {
		return nil, err
	}
	// Completion check (feature assessment): the consumers of the item's
	// exposed objects no tool of this session returned. A not-affected
	// verdict is rejected once while there are any; every assessment records
	// them as investigation gaps.
	var assessments []domain.AgentAssessment
	for _, pa := range pendingAssessments {
		untraced, err := s.untracedConsumers(ctx, pa.it.ID)
		if err != nil {
			return nil, err
		}
		if s.cfg.Prompt.Features.ScopedVerdict {
			if err := s.scopedCompletion(pa.at, pa.it.ID, pa.in, pa.verdict, untraced); err != nil {
				return nil, err
			}
		} else if pa.verdict != nil && pa.verdict.Class == domain.FindingNotAffected && len(untraced) > 0 && !s.rejectedNA[pa.it.ID] {
			s.rejectedNA[pa.it.ID] = true // recorded although the call is rejected: the check rejects once
			return nil, rejectf("%s: a not-affected verdict needs the consumers of the item's exposed objects examined; %d were never retrieved in this session: %s. Trace them (trace_consumers changeItem=%s) or read them (get_rendered_object, read_artifact), then submit again. A second submission is accepted, with each untraced consumer recorded as an investigation gap.",
				pa.at, len(untraced), listKeys(untraced, 20), pa.it.ID)
		}
		if err := s.behaviourCompletion(pa.at, pa.it, pa.verdict, pa.behaviour.list); err != nil {
			return nil, err
		}
		verdict := ""
		if pa.verdict != nil {
			verdict = pa.verdict.ID
		}
		a, aevs, err := s.assessmentOf(ctx, pa.it, pa.in, verdict, untraced, pa.behaviour, pa.at)
		if err != nil {
			return nil, err
		}
		assessments = append(assessments, a)
		evs = append(evs, aevs...)
	}
	view := submitView{Submitted: true, Assertions: ids, Note: "submitted: the run is over; the deterministic policy classifies every item. Stop now."}
	for _, a := range assessments {
		view.Assessments = append(view.Assessments, assessmentView{ID: a.ID, ChangeItem: a.ChangeItem, Verified: len(a.Verified), UntracedConsumers: a.UntracedConsumers,
			UnavailableEvidence: a.UnavailableEvidence})
	}
	for _, v := range verdicts {
		view.Verdicts = append(view.Verdicts, verdictView{ID: v.ID, ChangeItem: v.ChangeItem, Class: v.Class, Citations: len(v.Citations),
			DroppedCitations: v.DroppedCitations, DroppedVerifiedFacts: v.DroppedVerifiedFacts, Dropped: v.Dropped})
	}
	if agenda && !perItem {
		var missing []string
		for _, e := range s.cfg.Agenda {
			if !seen[e.ChangeItem] {
				missing = append(missing, e.ChangeItem)
			}
		}
		if len(missing) > 0 {
			return nil, rejectf("submit_findings must close every agenda item; %d of %d have no entry: %s (close an item you did not examine with disposition not-examined and the reason)",
				len(missing), len(s.cfg.Agenda), strings.Join(missing, ", "))
		}
		view.Agenda = &agendaView{Items: len(s.cfg.Agenda), Dispositions: counts}
	}
	return &outcome{value: nil, view: view, commit: func() {
		s.asserts = append(s.asserts, made...)
		for _, e := range evs {
			s.addRunEvidence(e)
		}
		for k, v := range closing {
			s.closing[k] = v
		}
		for k, v := range closingIDs {
			s.closingIDs[k] = v
		}
		for k, v := range closingEv {
			s.closingEv[k] = v
		}
		for item, cs := range entryEv {
			s.cite(item, cs)
		}
		for _, n := range notes {
			s.addNote(n, nil)
		}
		s.verdicts = append(s.verdicts, verdicts...)
		s.assessments = append(s.assessments, assessments...)
		s.submitted = true
	}}, nil
}

// --- answer links (prompt feature answerLinks) ------------------------------------------

type answerResult struct {
	Assertion string `json:"assertion"`
	Linked    bool   `json:"linked"`
	Reason    string `json:"reason,omitempty"`
}

// answerCandidates are the assertions a validation request answers: those it
// names in Answers (checked: they exist, are about the item, and are
// raises-concern or claims-mitigation), plus such assertions recorded with
// the request's hypothesis. A named assertion whose subjects, paths or
// rendered changes the request does not cover is rejected before anything
// runs; an implicit one that does not fit is simply not linked.
func (s *Session) answerCandidates(item string, named []string, hypothesis string, subjects, paths, rcs []string, itemWide bool) ([]domain.AgentAssertion, error) {
	if !s.cfg.Prompt.Features.AnswerLinks {
		if len(named) > 0 {
			return nil, rejectf("this prompt version (%s) does not link answers", s.cfg.Prompt.Version)
		}
		return nil, nil
	}
	byID := map[string]domain.AgentAssertion{}
	for _, a := range s.asserts {
		byID[a.ID] = a
	}
	var out []domain.AgentAssertion
	seen := map[string]bool{}
	for _, id := range uniqSorted(named) {
		a, ok := byID[id]
		switch {
		case !ok:
			return nil, rejectf("answers: unknown assertion %q (record it with record_assertion first)", id)
		case a.ChangeItem != item:
			return nil, rejectf("answers: assertion %s is about change item %s, not %s", id, a.ChangeItem, item)
		case a.Stance != domain.StanceRaisesConcern && a.Stance != domain.StanceClaimsMitigation:
			return nil, rejectf("answers: assertion %s is %s; only raises-concern and claims-mitigation assertions are answered", id, a.Stance)
		}
		if why := coverageGap(a, subjects, paths, rcs, itemWide); why != "" {
			return nil, rejectf("answers: assertion %s: %s — request the validation for what the assertion names", id, why)
		}
		seen[id] = true
		out = append(out, a)
	}
	if hypothesis != "" {
		for _, a := range s.asserts {
			if a.Hypothesis == hypothesis && a.ChangeItem == item && !seen[a.ID] &&
				(a.Stance == domain.StanceRaisesConcern || a.Stance == domain.StanceClaimsMitigation) {
				seen[a.ID] = true
				out = append(out, a)
			}
		}
	}
	return out, nil
}

// coverageGap says what of the assertion a set of subjects/paths/rendered
// changes does not cover ("" = covered). An assertion naming none of them
// is about the whole item and needs an item-wide request.
func coverageGap(a domain.AgentAssertion, subjects, paths, rcs []string, itemWide bool) string {
	if len(a.Subjects)+len(a.Paths)+len(a.RenderedChanges) == 0 {
		if !itemWide {
			return "it names no subject, path or rendered change, so only an item-wide validation (no subjects, paths or rendered changes requested) answers it"
		}
		return ""
	}
	for _, x := range a.Subjects {
		if !contains(subjects, x) {
			return "subject " + x + " is not covered"
		}
	}
	for _, x := range a.Paths {
		if !contains(paths, x) {
			return "impact path " + x + " is not covered"
		}
	}
	for _, x := range a.RenderedChanges {
		if !contains(rcs, x) {
			return "rendered change " + x + " is not covered"
		}
	}
	return ""
}

// answerGuard checks the produced record against the assertion ("" = the
// record may answer it): same item, and the record covers everything the
// assertion names — an assertion naming nothing needs a record covering
// every root and path of the item.
func answerGuard(a domain.AgentAssertion, v domain.ValidationRecord, t *domain.TraversalResult) string {
	if a.ChangeItem != v.ChangeItem {
		return "the validation is about another change item"
	}
	if len(a.Subjects)+len(a.Paths)+len(a.RenderedChanges) > 0 {
		return coverageGap(a, v.Subjects, v.Paths, v.RenderedChanges, false)
	}
	if t != nil {
		for _, r := range t.RootsOf(v.ChangeItem) {
			if !contains(v.Subjects, r.Node) {
				return "the validation does not cover root " + r.Node + " of the item (an item-wide assertion needs an item-wide validation)"
			}
		}
		for _, p := range t.PathsOf(v.ChangeItem) {
			if !contains(v.Paths, p.ID) {
				return "the validation does not cover impact path " + p.ID + " of the item"
			}
		}
	}
	return ""
}
