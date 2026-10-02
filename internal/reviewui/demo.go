package reviewui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// DemoQueue is an in-memory knowledge.Queue with realistic fixture items. It
// stands in for the knowledge lane's file-backed queue (`ri review serve
// -demo`, and the tests of this package). It enforces the same rules the real
// queue must: every decision of a Decide call is validated before any is
// written, only open items can be decided, and nothing is stored for an
// invalid call.
type DemoQueue struct {
	mu          sync.Mutex
	candidates  map[string]domain.SemanticCandidate
	proposals   map[string]domain.SemanticProposal
	validations map[string]domain.ValidationResult
	items       map[string]domain.ReviewItem
	order       []string
	decisions   []domain.ReviewDecision
	facts       []domain.VerifiedFact
	envs        map[string]*domain.EnvironmentContext
	renders     map[string]*RenderedDelta
}

var _ knowledge.Queue = (*DemoQueue)(nil)

// Decisions returns every decision recorded so far (oldest first).
func (q *DemoQueue) Decisions() []domain.ReviewDecision {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]domain.ReviewDecision(nil), q.decisions...)
}

// ItemRecord returns the stored review item.
func (q *DemoQueue) ItemRecord(id string) (domain.ReviewItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	it, ok := q.items[id]
	return it, ok
}

// ItemIDs lists item ids in fixture order.
func (q *DemoQueue) ItemIDs() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.order...)
}

// ItemIDByTitle returns the id of the item whose candidate title contains s.
func (q *DemoQueue) ItemIDByTitle(s string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, id := range q.order {
		if strings.Contains(q.candidates[q.items[id].CandidateID].Title, s) {
			return id
		}
	}
	return ""
}

// Agreement computes the per-aspect agreement among distinct models of the
// given proposals, the way routing does (DESIGN.md §6): proposals are grouped
// by aspect digest, a model counts once. Exported for fakes of knowledge.Queue.
func Agreement(props []domain.SemanticProposal) []knowledge.AspectAgreement {
	var out []knowledge.AspectAgreement
	for _, a := range domain.Aspects {
		ag := knowledge.AspectAgreement{Aspect: a, Groups: map[string][]string{}}
		seen := false
		for _, p := range props {
			switch {
			case p.Assertion.Has(a):
				d := p.Assertion.AspectDigest(a)
				ag.Groups[d] = append(ag.Groups[d], p.ID)
				seen = true
			default:
				for _, u := range p.Undetermined {
					if u == a {
						ag.Undetermined = append(ag.Undetermined, p.ID)
						seen = true
					}
				}
			}
		}
		if seen {
			out = append(out, ag)
		}
	}
	return out
}

// --- the knowledge.Queue implementation ----------------------------------------------

// Inbox implements knowledge.Queue.
func (q *DemoQueue) Inbox(_ context.Context, f knowledge.InboxFilter) (*knowledge.Inbox, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	in := &knowledge.Inbox{}
	for _, id := range q.order {
		it := q.items[id]
		row := q.row(it)
		switch it.Status {
		case domain.ReviewPending:
			in.Counts.Pending++
			if row.Disagreement {
				in.Counts.ModelDisagreement++
			}
			switch it.QuestionType {
			case domain.QuestionSemanticMapping:
				in.Counts.NeedsSemanticMapping++
			case domain.QuestionApplicability:
				in.Counts.ApplicabilityQuestions++
			}
		case domain.ReviewNeedsEvidence:
			in.Counts.NeedsMoreEvidence++
		case domain.ReviewDeferred:
			in.Counts.Deferred++
		}
		if q.matches(it, row, f) {
			in.Items = append(in.Items, row)
		}
	}
	rank := map[domain.ReviewPriority]int{domain.PriorityHigh: 0, domain.PriorityNormal: 1, domain.PriorityLow: 2}
	sort.SliceStable(in.Items, func(i, j int) bool {
		return rank[in.Items[i].Item.Routing.Priority] < rank[in.Items[j].Item.Routing.Priority]
	})
	if f.Limit > 0 && len(in.Items) > f.Limit {
		in.Items = in.Items[:f.Limit]
	}
	return in, nil
}

func (q *DemoQueue) row(it domain.ReviewItem) knowledge.InboxRow {
	props := q.proposalsOf(it)
	row := knowledge.InboxRow{Item: it, Title: q.candidates[it.CandidateID].Title}
	seen := map[string]bool{}
	calls := map[string]bool{}
	for _, p := range props {
		calls[p.Provenance.CallID] = true
		if !seen[p.Provenance.Model] {
			seen[p.Provenance.Model] = true
			row.Models = append(row.Models, p.Provenance.Model)
		}
	}
	row.Calls = len(calls)
	for _, a := range Agreement(props) {
		if len(a.Groups) >= 2 {
			row.Disagreement = true
		}
	}
	return row
}

func (q *DemoQueue) proposalsOf(it domain.ReviewItem) []domain.SemanticProposal {
	var out []domain.SemanticProposal
	for _, id := range it.Proposals {
		if p, ok := q.proposals[id]; ok {
			out = append(out, p)
		}
	}
	return out
}

func (q *DemoQueue) matches(it domain.ReviewItem, row knowledge.InboxRow, f knowledge.InboxFilter) bool {
	if f.Product != "" && it.Product != f.Product {
		return false
	}
	if f.Release != "" && it.Release != f.Release {
		return false
	}
	if f.QuestionType != "" && it.QuestionType != f.QuestionType {
		return false
	}
	if f.SubjectType != "" && (it.Proposed.Subject == nil || it.Proposed.Subject.Family != f.SubjectType) {
		return false
	}
	if f.Severity != "" && (it.Proposed.Consequence == nil || it.Proposed.Consequence.Severity != f.Severity) {
		return false
	}
	if f.Confidence != "" {
		ok := false
		for _, p := range q.proposalsOf(it) {
			ok = ok || p.Provenance.Confidence == f.Confidence
		}
		if !ok {
			return false
		}
	}
	if f.Model != "" {
		ok := false
		for _, m := range row.Models {
			ok = ok || strings.Contains(m, f.Model)
		}
		if !ok {
			return false
		}
	}
	if f.Disagreement != nil && *f.Disagreement != row.Disagreement {
		return false
	}
	if f.Source != "" {
		ok := false
		for _, e := range q.candidates[it.CandidateID].Evidence {
			ok = ok || strings.Contains(e.URI, f.Source)
		}
		if !ok {
			return false
		}
	}
	if len(f.Status) > 0 {
		ok := false
		for _, s := range f.Status {
			ok = ok || s == it.Status
		}
		if !ok {
			return false
		}
	}
	if f.Reviewer != "" {
		ok := false
		for _, d := range q.decisions {
			ok = ok || (d.ReviewItemID == it.ID && d.Reviewer == f.Reviewer)
		}
		if !ok {
			return false
		}
	}
	return true
}

// Item implements knowledge.Queue.
func (q *DemoQueue) Item(_ context.Context, id string) (*knowledge.ReviewContext, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	it, ok := q.items[id]
	if !ok {
		return nil, knowledge.ErrNotFound
	}
	rc := &knowledge.ReviewContext{Item: it, Candidate: q.candidates[it.CandidateID], Environment: it.Context}
	rc.Proposals = q.proposalsOf(it)
	for _, vid := range it.Validations {
		if v, ok := q.validations[vid]; ok {
			rc.Validations = append(rc.Validations, v)
		}
	}
	rc.Agreement = Agreement(rc.Proposals)
	for _, d := range q.decisions {
		other := q.items[d.ReviewItemID]
		if d.ReviewItemID == id || (other.Proposed.Subject != nil && it.Proposed.Subject != nil && other.Proposed.Subject.Key() == it.Proposed.Subject.Key()) {
			rc.RelatedDecisions = append(rc.RelatedDecisions, d)
		}
	}
	for _, f := range q.facts {
		if it.Proposed.Subject != nil && f.Assertion.Subject != nil && f.Assertion.Subject.Key() == it.Proposed.Subject.Key() {
			rc.RelatedFacts = append(rc.RelatedFacts, f)
		}
	}
	if c := it.Proposed.Consequence; c != nil {
		rc.SuggestedExposedClass = c.Kind.ExposedClass()
	}
	return rc, nil
}

// Decide implements knowledge.Queue: all decisions are validated first; an
// error writes nothing.
func (q *DemoQueue) Decide(_ context.Context, ds []domain.ReviewDecision) ([]knowledge.DecisionOutcome, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(ds) == 0 {
		return nil, fmt.Errorf("no decisions")
	}
	seen := map[string]bool{}
	for _, d := range ds {
		if err := d.Validate(); err != nil {
			return nil, err
		}
		it, ok := q.items[d.ReviewItemID]
		switch {
		case !ok:
			return nil, fmt.Errorf("decision %s: %w: %s", d.ID, knowledge.ErrNotFound, d.ReviewItemID)
		case it.Status == domain.ReviewDecided || it.Status == domain.ReviewSuperseded:
			return nil, fmt.Errorf("decision %s: item %s is already %s", d.ID, it.ID, it.Status)
		case seen[d.ReviewItemID]:
			return nil, fmt.Errorf("two decisions for item %s in one call", it.ID)
		}
		seen[d.ReviewItemID] = true
	}
	var out []knowledge.DecisionOutcome
	for _, d := range ds {
		it := q.items[d.ReviewItemID]
		switch d.Action {
		case domain.ActionAccept, domain.ActionCorrect, domain.ActionReject:
			it.Status = domain.ReviewDecided
		case domain.ActionNeedMoreEvidence:
			it.Status = domain.ReviewNeedsEvidence
		case domain.ActionDefer:
			it.Status = domain.ReviewDeferred
		}
		q.items[it.ID] = it
		q.decisions = append(q.decisions, d)
		out = append(out, knowledge.DecisionOutcome{Decision: d})
	}
	return out, nil
}

// --- fixtures ---------------------------------------------------------------------------

// DemoEpoch is the fixed time of the fixtures (they are deterministic).
var DemoEpoch = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

type demoBuilder struct {
	q     *DemoQueue
	calls *int
}

func str(s string) *string { return &s }

// NewDemoQueue returns a queue filled with realistic fixtures: a multi-model
// disagreement on the consequence of rotationPolicy, an applicability
// question, a semantic mapping the models disagree on, an item that needs
// more evidence, a deferred one, a decided one with a fact, and a batch of
// routine items that models agree on (the bulk-review demonstration).
func NewDemoQueue() *DemoQueue {
	q := &DemoQueue{
		candidates:  map[string]domain.SemanticCandidate{},
		proposals:   map[string]domain.SemanticProposal{},
		validations: map[string]domain.ValidationResult{},
		items:       map[string]domain.ReviewItem{},
		envs:        map[string]*domain.EnvironmentContext{},
		renders:     map[string]*RenderedDelta{},
	}
	n := 0
	b := demoBuilder{q, &n}
	b.rotationPolicy()
	b.http01()
	b.mapping()
	b.insufficient()
	b.deferred()
	b.decided()
	b.consensusAction()
	b.routine()
	return q
}

func (b demoBuilder) candidate(product domain.ProductID, release, title, text string, evs ...domain.Evidence) domain.SemanticCandidate {
	st := domain.StatementKey(evs[0])
	a := domain.ChangeAnchor{Release: release, EvidenceKeys: []string{domain.EvidenceKey(evs[0])}, StatementKeys: []string{st}, ChangeIDs: []string{"chg-" + domain.ShortHash(title)}}
	members := []domain.CandidateMember{{ChangeID: a.ChangeIDs[0], Anchor: &a}}
	c := domain.SemanticCandidate{
		ID: domain.CandidateID(product, release, members), Product: product, Release: release,
		Members: members, Grouping: "single", Category: domain.CategoryConfiguration,
		Title: title, Text: text, Evidence: evs, Producer: "semantic.candidates@v1", CreatedAt: DemoEpoch,
	}
	b.q.candidates[c.ID] = c
	return c
}

func (b demoBuilder) proposal(c domain.SemanticCandidate, task domain.ProposalTask, provider, model string, conf domain.Confidence,
	a domain.SemanticAssertion, undetermined []domain.Aspect, reason string, class domain.ImpactClass) domain.SemanticProposal {
	at := DemoEpoch.Add(10 * time.Minute)
	var input []domain.EvidenceID
	for _, e := range c.Evidence {
		input = append(input, e.ID)
	}
	*b.calls++
	p := domain.SemanticProposal{
		CandidateID: c.ID, Task: task, Provider: provider, Assertion: a, Undetermined: undetermined,
		UndeterminedReason: reason, SuggestedClass: class,
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "semantic.propose@v1", Confidence: conf,
			Model: model, ModelVersion: model + "-2026-09", PromptVersion: "semantic/v1", PromptDigest: "sha256:" + domain.ShortHash(model, c.ID),
			InputEvidence: input, GeneratedAt: &at,
			CallID: fmt.Sprintf("call-%s-%03d", model, *b.calls)}, // each proposal is its own stateless call
	}
	if !a.Empty() {
		p.Citations = []domain.EvidenceID{c.Evidence[0].ID}
	}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	b.q.proposals[p.ID] = p
	return p
}

func (b demoBuilder) validation(c domain.SemanticCandidate, validator string, p *domain.SemanticProposal, a domain.SemanticAssertion, checks []domain.AspectCheck, evs ...domain.Evidence) domain.ValidationResult {
	v := domain.ValidationResult{CandidateID: c.ID, Validator: validator, Assertion: a, Checks: checks, Evidence: evs, CheckedAt: DemoEpoch.Add(12 * time.Minute)}
	if p != nil {
		v.ProposalID = p.ID
	}
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	b.q.validations[v.ID] = v
	return v
}

func (b demoBuilder) item(c domain.SemanticCandidate, qt domain.QuestionType, question string, proposed domain.SemanticAssertion,
	props []domain.SemanticProposal, vals []domain.ValidationResult, rt domain.Routing, status domain.ReviewStatus, env *domain.EnvironmentContext) domain.ReviewItem {
	it := domain.ReviewItem{CandidateID: c.ID, Product: c.Product, Release: c.Release, QuestionType: qt, Question: question,
		Proposed: proposed, Routing: rt, Context: env, Status: status, CreatedAt: DemoEpoch.Add(15 * time.Minute)}
	for _, p := range props {
		it.Proposals = append(it.Proposals, p.ID)
	}
	for _, v := range vals {
		it.Validations = append(it.Validations, v.ID)
	}
	it.ID = domain.ReviewItemID(it.CandidateID, it.QuestionType, it.Proposed)
	b.q.items[it.ID] = it
	b.q.order = append(b.q.order, it.ID)
	return it
}

func certs(of ...domain.Condition) domain.Condition {
	return domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate", Of: of}
}

func rotation(kind domain.ConsequenceKind, sev domain.ImpactSeverity, stmt string) domain.SemanticAssertion {
	return domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "cert-manager", Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"Never"`), After: str(`"Always"`)},
		Applicability: &domain.Applicability{
			Exposure: certs(domain.Condition{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: domain.StateUnset}),
			Overlap:  ptr(certs(domain.Condition{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: domain.StateSet})),
		},
		Consequence: &domain.Consequence{Kind: kind, ExposedClass: kind.ExposedClass(), Statement: stmt, Severity: sev},
		Statement:   "Certificate.spec.privateKey.rotationPolicy default Never → Always",
	}
}

func ptr[T any](v T) *T { return &v }

func (b demoBuilder) rotationPolicy() {
	up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://github.com/cert-manager/website/blob/master/content/docs/releases/upgrading/upgrading-1.17-1.18.md", "L21-L30",
		"The default value of Certificate.spec.privateKey.rotationPolicy is now Always. Existing Certificates that do not set the field will have their private key rotated on every renewal.", "sha256:upg118", DemoEpoch)
	crd := domain.NewEvidence(domain.EvidenceStructured, "crds", "https://github.com/cert-manager/cert-manager/blob/v1.18.0/deploy/crds/cert-manager.io_certificates.yaml", "L612",
		"rotationPolicy: description: … Defaults to `Always`.", "sha256:crd118", DemoEpoch)
	c := b.candidate("cert-manager", "v1.18.0", "The default privateKey.rotationPolicy is now Always",
		"Upgrading 1.17 → 1.18: rotationPolicy defaults to Always.", up, crd)
	full := rotation(domain.ConsequenceBehaviorChange, domain.SeverityHigh, "Private keys are regenerated on every renewal; pinned keys (TLS pinning, HPKP-style allow lists) break.")
	p1 := b.proposal(c, domain.TaskFull, "anthropic", "claude-opus-5-5", domain.ConfidenceMedium, full, nil, "", domain.ImpactReviewRequired)
	alt := rotation(domain.ConsequenceSettingIgnored, domain.SeverityCritical, "Pinned keys stop matching after renewal and clients refuse the certificate.")
	p2 := b.proposal(c, domain.TaskFull, "anthropic", "claude-sonnet-5-5", domain.ConfidenceMedium, alt, nil, "", domain.ImpactReviewRequired)
	p2.Assertion.Consequence.Kind = domain.ConsequenceWorkloadFailure
	p2.Assertion.Consequence.ExposedClass = domain.ConsequenceWorkloadFailure.ExposedClass()
	b.q.proposals[p2.ID] = p2
	hasOverlapOnly := rotation(domain.ConsequenceBehaviorChange, domain.SeverityMedium, "Keys rotate on renewal.")
	hasOverlapOnly.Applicability = &domain.Applicability{Exposure: certs(domain.Condition{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: domain.StateUnset})}
	p3 := b.proposal(c, domain.TaskFull, "zai", "glm-5.3-flash", domain.ConfidenceLow, hasOverlapOnly, nil, "", domain.ImpactReviewRequired)
	v := b.validation(c, "semvalidate.crd@v1", &p1, full, []domain.AspectCheck{
		{Aspect: domain.AspectSubject, Outcome: domain.OutcomeConfirmed, Rule: "crd-schema:field", Detail: "Certificate.spec.privateKey.rotationPolicy exists in v1.18.0 CRD"},
		{Aspect: domain.AspectChange, Outcome: domain.OutcomeConfirmed, Rule: "crd-schema:default", Detail: "default Never (v1.17.4) → Always (v1.18.0)"},
		{Aspect: domain.AspectApplicability, Outcome: domain.OutcomeInconclusive, Rule: "canonical:crd-field/default-changed", Detail: "stated condition differs from the canonical one (overlap branch)"},
	}, crd)
	b.item(c, domain.QuestionConsequence, "If a Certificate with rotationPolicy unset does nothing during the upgrade, what happens — and is that review or action?",
		full, []domain.SemanticProposal{p1, p2, p3}, []domain.ValidationResult{v},
		domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityHigh, Signals: []domain.RoutingSignal{domain.SignalModelsDisagree, domain.SignalHighImpact, domain.SignalValidationConfirmed}},
		domain.ReviewPending, &domain.EnvironmentContext{Label: "illustration: platform-prod (values digest 41c9)", Digest: "sha256:41c9"})
}

func (b demoBuilder) http01() {
	up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://github.com/cert-manager/cert-manager/releases/tag/v1.18.0", "## Breaking changes",
		"ingress-nginx ≥ 1.12 is required for HTTP01 solvers using ingressClassName; older controllers ignore the solver ingress.", "sha256:rel118", DemoEpoch)
	c := b.candidate("cert-manager", "v1.18.0", "HTTP01 solver needs ingress-nginx >= 1.12",
		"Breaking: HTTP01 solver ingress handling.", up)
	a := domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectProductRelationship, Product: "cert-manager", Name: "ingress-nginx"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindRequirementChanged, After: str(">=1.12.0")},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
			{Op: domain.OpProductVersion, Name: "ingress-nginx", State: domain.StateOutOfRange, Range: ">=1.12.0"},
			{Op: domain.OpValuesKey, Path: "ingressShim.defaultIssuerKind", State: domain.StateSet},
		}}},
		Statement: "cert-manager 1.18 requires ingress-nginx >= 1.12 for HTTP01",
	}
	p1 := b.proposal(c, domain.TaskRelationship, "anthropic", "claude-opus-5-5", domain.ConfidenceMedium, a, nil, "", "")
	p2 := b.proposal(c, domain.TaskRelationship, "anthropic", "claude-sonnet-5-5", domain.ConfidenceMedium, a, nil, "", "")
	v := b.validation(c, "semvalidate.compat@v1", &p1, a, []domain.AspectCheck{
		{Aspect: domain.AspectSubject, Outcome: domain.OutcomeConfirmed, Rule: "compatibility:row", Detail: "compatibility table lists ingress-nginx"},
		{Aspect: domain.AspectChange, Outcome: domain.OutcomeConfirmed, Rule: "compatibility:range", Detail: ">=1.12.0 matches the table row"},
	}, domain.NewEvidence(domain.EvidenceStructured, "compat", "https://cert-manager.io/docs/releases/#compatibility", "table", "ingress-nginx 1.12+", "sha256:compat", DemoEpoch))
	it := b.item(c, domain.QuestionRelationship, "Does cert-manager v1.18 require ingress-nginx ≥ 1.12 for HTTP01 — and which environments are exposed?",
		a, []domain.SemanticProposal{p1, p2}, []domain.ValidationResult{v},
		domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal, Signals: []domain.RoutingSignal{domain.SignalModelsAgree, domain.SignalValidationConfirmed}},
		domain.ReviewPending, nil)
	b.q.renders[it.ID] = &RenderedDelta{
		Relation: RelationNotVisible, Renderer: "helm", Version: "v3.17.2",
		FromChart: "sha256:9a1c…e04", ToChart: "sha256:51bd…77f",
		Explanation: "The requirement is about a controller outside this chart; a chart render cannot show it. Absence is not refutation.",
		Deltas: []RenderedChange{{
			Object: "rbac.authorization.k8s.io/v1 ClusterRole/cert-manager-controller-challenges", ChangeClass: "rbac-permission-added",
			Path: "rules[3].resources", Source: "[ingresses]", Target: "[ingresses, httproutes]", Template: "templates/rbac.yaml",
		}},
	}
}

func (b demoBuilder) mapping() {
	up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://github.com/argoproj/argo-cd/blob/v3.1.0/docs/operator-manual/upgrading/3.0-3.1.md", "L40-L44",
		"The `server.rbac.policy.default` setting moved to `configs.rbac.policy.default`.", "sha256:argo31", DemoEpoch)
	c := b.candidate("argo-cd", "v3.1.0", "RBAC default policy setting moved",
		"server.rbac.policy.default → configs.rbac.policy.default", up)
	mk := func(fam domain.SubjectFamily, path string) domain.SemanticAssertion {
		return domain.SemanticAssertion{
			Subject:   &domain.Subject{Family: fam, Product: "argo-cd", Path: path},
			Change:    &domain.ChangeSpec{Type: domain.ChangeKindRenamed, ReplacedBy: &domain.Subject{Family: fam, Product: "argo-cd", Path: "configs.rbac.policy.default"}},
			Statement: "RBAC default policy value moved",
		}
	}
	hv := mk(domain.SubjectHelmValue, "server.rbac.policy.default")
	ck := mk(domain.SubjectConfigKey, "server.rbac.policy.default")
	p1 := b.proposal(c, domain.TaskSemanticMapping, "anthropic", "claude-opus-5-5", domain.ConfidenceMedium, hv, nil, "", "")
	p2 := b.proposal(c, domain.TaskSemanticMapping, "anthropic", "claude-sonnet-5-5", domain.ConfidenceMedium, ck, nil, "", "")
	p3 := b.proposal(c, domain.TaskSemanticMapping, "zai", "glm-5.3-flash", domain.ConfidenceLow, hv, nil, "", "")
	v := b.validation(c, "semvalidate.helm@v1", &p1, hv, []domain.AspectCheck{
		{Aspect: domain.AspectSubject, Outcome: domain.OutcomeConfirmed, Rule: "helm-values:key", Detail: "server.rbac.policy.default is a values.yaml key in 3.0.x"},
		{Aspect: domain.AspectChange, Outcome: domain.OutcomeRefuted, Rule: "helm-values:renamed", Detail: "configs.rbac.policy.default is absent from the 3.1.0 values.yaml"},
	}, domain.NewEvidence(domain.EvidenceStructured, "values", "https://github.com/argoproj/argo-helm/blob/argo-cd-9.0.0/charts/argo-cd/values.yaml", "L88", "rbac: policy.default", "sha256:argovalues", DemoEpoch))
	b.item(c, domain.QuestionSemanticMapping, "Is `server.rbac.policy.default` a Helm value, or a key inside the argocd-rbac-cm ConfigMap — and was it renamed?",
		hv, []domain.SemanticProposal{p1, p2, p3}, []domain.ValidationResult{v},
		domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal, Signals: []domain.RoutingSignal{domain.SignalModelsDisagree, domain.SignalValidationRefuted}},
		domain.ReviewPending, nil)
}

func (b demoBuilder) insufficient() {
	up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://github.com/istio/istio/releases/tag/1.27.0", "## Upgrade notes",
		"Some behaviours of the ambient data plane changed. See the migration guide.", "sha256:istio127", DemoEpoch)
	c := b.candidate("istio", "1.27.0", "Ambient data plane behaviour changed", "Vague note.", up)
	p1 := b.proposal(c, domain.TaskFull, "anthropic", "claude-opus-5-5", domain.ConfidenceLow, domain.SemanticAssertion{}, domain.Aspects,
		"the note names no concrete field, flag or API", "")
	a := domain.SemanticAssertion{Statement: "Is the cited text enough to decide?"}
	b.item(c, domain.QuestionEvidenceSufficiency, "Is the cited release note enough to say what changed? (It only points at a migration guide.)",
		a, []domain.SemanticProposal{p1}, nil,
		domain.Routing{Route: domain.RouteMissingEvidence, Priority: domain.PriorityLow, Signals: []domain.RoutingSignal{domain.SignalAllUndetermined}},
		domain.ReviewNeedsEvidence, nil)
}

func (b demoBuilder) deferred() {
	up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://github.com/prometheus-community/helm-charts/releases/tag/kube-prometheus-stack-77.0.0", "## Breaking",
		"The default retention changed from 10d to 15d.", "sha256:kps77", DemoEpoch)
	c := b.candidate("kube-prometheus-stack", "77.0.0", "Default retention changed 10d → 15d", "Default changed.", up)
	a := domain.SemanticAssertion{
		Subject:     &domain.Subject{Family: domain.SubjectHelmValue, Product: "kube-prometheus-stack", Path: "prometheus.prometheusSpec.retention"},
		Change:      &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"10d"`), After: str(`"15d"`)},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired, Statement: "Disk usage grows ~50%.", Severity: domain.SeverityLow},
	}
	p1 := b.proposal(c, domain.TaskConsequence, "anthropic", "claude-haiku-4-5", domain.ConfidenceMedium, domain.SemanticAssertion{Consequence: a.Consequence}, nil, "", domain.ImpactReviewRequired)
	b.item(c, domain.QuestionClassification, "Is a larger default retention a review item or informational?",
		domain.SemanticAssertion{Consequence: a.Consequence}, []domain.SemanticProposal{p1}, nil,
		domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityLow, Signals: []domain.RoutingSignal{domain.SignalSingleModel}},
		domain.ReviewDeferred, nil)
}

func (b demoBuilder) decided() {
	up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://github.com/cert-manager/cert-manager/releases/tag/v1.18.0", "## Changes",
		"The `--enable-certificate-owner-ref` flag no longer defaults to false.", "sha256:flag118", DemoEpoch)
	c := b.candidate("cert-manager", "v1.18.0", "Owner-reference flag default flipped", "Flag default.", up)
	a := domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCLIFlag, Product: "cert-manager", Name: "enable-certificate-owner-ref", Component: "controller"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str("false"), After: str("true")},
	}
	p1 := b.proposal(c, domain.TaskSemanticMapping, "anthropic", "claude-opus-5-5", domain.ConfidenceMedium, a, nil, "", "")
	it := b.item(c, domain.QuestionSemanticMapping, "Is this a controller CLI flag default change false → true?", a,
		[]domain.SemanticProposal{p1}, nil,
		domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal, Signals: []domain.RoutingSignal{domain.SignalSingleModel}}, domain.ReviewPending, nil)
	orig := it.Proposed
	d := domain.ReviewDecision{ReviewItemID: it.ID, Action: domain.ActionAccept, Labels: []domain.FeedbackLabel{domain.LabelAccepted}, Original: &orig,
		Reviewer: "morgan", ReviewerKind: domain.ReviewerHuman, StartedAt: DemoEpoch.Add(40 * time.Minute), DecidedAt: DemoEpoch.Add(41 * time.Minute)}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	b.q.decisions = append(b.q.decisions, d)
	it.Status = domain.ReviewDecided
	b.q.items[it.ID] = it
}

// routine adds items the models agree on: the bulk-review demonstration.
func (b demoBuilder) routine() {
	type row struct {
		product       domain.ProductID
		release, flag string
		before, after string
	}
	for _, r := range []row{
		{"cert-manager", "v1.18.0", "dns01-recursive-nameservers-only", "false", "true"},
		{"cert-manager", "v1.18.0", "acme-http01-solver-nameservers", `""`, `"8.8.8.8:53"`},
		{"cert-manager", "v1.18.0", "max-concurrent-challenges", "60", "30"},
		{"ingress-nginx", "v1.13.0", "enable-ssl-passthrough", "false", "true"},
		{"ingress-nginx", "v1.13.0", "default-ssl-certificate", `""`, `"kube-system/default-tls"`},
		{"external-dns", "v0.19.0", "txt-prefix", `""`, `"edns-"`},
	} {
		up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://github.com/example/"+string(r.product)+"/releases/tag/"+r.release, "## Changes",
			fmt.Sprintf("The default of --%s changed from %s to %s.", r.flag, r.before, r.after), "sha256:"+domain.ShortHash(r.flag), DemoEpoch)
		c := b.candidate(r.product, r.release, "Default of --"+r.flag+" changed", "flag default", up)
		a := domain.SemanticAssertion{
			Subject: &domain.Subject{Family: domain.SubjectCLIFlag, Product: r.product, Name: r.flag},
			Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(r.before), After: str(r.after)},
		}
		p1 := b.proposal(c, domain.TaskSemanticMapping, "anthropic", "claude-opus-5-5", domain.ConfidenceMedium, a, nil, "", "")
		// who answers second: a different family (cross-model consensus), the
		// same family (opus + sonnet) or a second separate call of the same model
		second, provider := "claude-sonnet-5-5", "anthropic"
		switch r.flag {
		case "txt-prefix":
			second = "claude-opus-5-5"
		case "acme-http01-solver-nameservers", "max-concurrent-challenges", "default-ssl-certificate":
			second, provider = "glm-5.3-flash", "zai"
		}
		p2 := b.proposal(c, domain.TaskSemanticMapping, provider, second, domain.ConfidenceMedium, a, nil, "", "")
		v := b.validation(c, "semvalidate.flag@v1", &p1, a, []domain.AspectCheck{
			{Aspect: domain.AspectSubject, Outcome: domain.OutcomeConfirmed, Rule: "cli:flag-listed"},
			{Aspect: domain.AspectChange, Outcome: domain.OutcomeInconclusive, Rule: "cli:default"},
		}, domain.NewEvidence(domain.EvidenceStructured, "helm", "https://example.invalid/chart/"+r.flag+"/values.yaml", "L1", r.flag, "sha256:v"+r.flag, DemoEpoch))
		b.item(c, domain.QuestionSemanticMapping, fmt.Sprintf("Did the default of --%s change from %s to %s?", r.flag, r.before, r.after),
			a, []domain.SemanticProposal{p1, p2}, []domain.ValidationResult{v},
			domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal, Signals: []domain.RoutingSignal{domain.SignalModelsAgree}},
			domain.ReviewPending, nil)
	}
}

// consensusAction is an audit item for a PO-2 fact: two separate calls of
// different models agree on an action-eligible consequence and both requested
// action-required. (The fact itself is minted by routing; every such fact is
// sampled into human review, and this is that review item.)
func (b demoBuilder) consensusAction() {
	up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://github.com/cert-manager/cert-manager/releases/tag/v1.18.0", "## Breaking changes",
		"Certificates whose private key is RSA smaller than 2048 bits are now rejected by the webhook.", "sha256:rsa118", DemoEpoch)
	c := b.candidate("cert-manager", "v1.18.0", "RSA keys below 2048 bits are rejected", "Webhook rejects small RSA keys.", up)
	a := domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "cert-manager", Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.size"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindValidationTightened},
		Applicability: &domain.Applicability{Exposure: certs(
			domain.Condition{Op: domain.OpField, Path: "spec.privateKey.algorithm", State: domain.StateEquals, Values: []string{`"RSA"`}},
			domain.Condition{Op: domain.OpField, Path: "spec.privateKey.size", State: domain.StateEquals, Values: []string{`1024`}})},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceResourceRejected, ExposedClass: domain.ImpactActionRequired,
			Statement: "Applying a Certificate with an RSA key below 2048 bits is rejected by the webhook.", Remediation: "Raise spec.privateKey.size to 2048 or more.", Severity: domain.SeverityHigh},
		Statement: "Certificate.spec.privateKey.size below 2048 (RSA) is rejected",
	}
	p1 := b.proposal(c, domain.TaskFull, "anthropic", "claude-opus-5-5", domain.ConfidenceMedium, a, nil, "", domain.ImpactActionRequired)
	p2 := b.proposal(c, domain.TaskFull, "zai", "glm-5.3-flash", domain.ConfidenceMedium, a, nil, "", domain.ImpactActionRequired)
	b.item(c, domain.QuestionConsequence, "Audit: two separate model calls agree this is ACTION REQUIRED. Is a Certificate with an RSA key below 2048 bits really rejected, and is that action-eligible?",
		a, []domain.SemanticProposal{p1, p2}, nil,
		domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityHigh, Signals: []domain.RoutingSignal{domain.SignalModelsAgree, domain.SignalHighImpact}},
		domain.ReviewPending, nil)
}
