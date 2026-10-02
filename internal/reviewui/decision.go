package reviewui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// decisionInput is everything a decision needs besides the item.
type decisionInput struct {
	Action      domain.DecisionAction
	Reason      string
	Reviewer    string
	Started     time.Time
	Decided     time.Time
	Wrong       []domain.FeedbackLabel // wrong-* labels the reviewer ticked
	Outcome     string                 // reject only: "rejected" (default) or "duplicate"
	DuplicateOf string
	Corrected   *domain.SemanticAssertion
}

// refuseProxy rejects any attempt to record a non-human decision through the
// UI: proxy decisions come only from the CLI, with provenance (DESIGN.md §9).
func refuseProxy(r *http.Request) error {
	for _, k := range []string{"reviewer_kind", "reviewerKind", "reviewerkind"} {
		if v := strings.TrimSpace(r.PostForm.Get(k)); v != "" && v != string(domain.ReviewerHuman) {
			return fmt.Errorf("the review UI records human decisions only (reviewer kind %q refused); proxy decisions come from the CLI with provenance", v)
		}
	}
	for k := range r.PostForm {
		if strings.HasPrefix(strings.ToLower(k), "proxy") {
			return errors.New("the review UI records human decisions only; proxy provenance is not accepted here")
		}
	}
	return nil
}

func parseAction(s string) (domain.DecisionAction, error) {
	a := domain.DecisionAction(strings.TrimSpace(s))
	switch a {
	case domain.ActionAccept, domain.ActionReject, domain.ActionCorrect, domain.ActionNeedMoreEvidence, domain.ActionDefer:
		return a, nil
	}
	return "", fmt.Errorf("unknown action %q (want accept, reject, correct, need-more-evidence or defer)", s)
}

// wrongLabels reads the ticked wrong-* labels; any other label is refused
// (the outcome label follows from the action).
func wrongLabels(vals []string) ([]domain.FeedbackLabel, error) {
	var out []domain.FeedbackLabel
	for _, v := range vals {
		l := domain.FeedbackLabel(strings.TrimSpace(v))
		if l == "" {
			continue
		}
		if !strings.HasPrefix(string(l), "wrong-") || !slices.Contains(domain.FeedbackLabels, l) {
			return nil, fmt.Errorf("label %q cannot be set by hand (only wrong-* labels; the outcome label follows from the action)", v)
		}
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out, nil
}

// aspectLabel maps an aspect to the wrong-* label that names its error.
var aspectLabel = map[domain.Aspect]domain.FeedbackLabel{
	domain.AspectSubject:       domain.LabelWrongSubject,
	domain.AspectChange:        domain.LabelWrongChangeType,
	domain.AspectApplicability: domain.LabelWrongApplicability,
	domain.AspectConsequence:   domain.LabelWrongConsequence,
}

// buildDecision turns a reviewer's input into a decision on rc's item. The
// original is always the item's own proposed assertion (never client data),
// and the kind is always human. The decision is validated before it is
// returned.
func buildDecision(rc *knowledge.ReviewContext, in decisionInput) (domain.ReviewDecision, error) {
	orig := rc.Item.Proposed
	d := domain.ReviewDecision{
		ReviewItemID: rc.Item.ID, Action: in.Action, Original: &orig, Reviewer: strings.TrimSpace(in.Reviewer),
		ReviewerKind: domain.ReviewerHuman, Reason: strings.TrimSpace(in.Reason), StartedAt: in.Started, DecidedAt: in.Decided,
	}
	switch in.Action {
	case domain.ActionAccept:
		d.Labels = []domain.FeedbackLabel{domain.LabelAccepted}
	case domain.ActionReject:
		switch in.Outcome {
		case "", string(domain.LabelRejected):
			d.Labels = append([]domain.FeedbackLabel{domain.LabelRejected}, in.Wrong...)
		case string(domain.LabelDuplicate):
			if strings.TrimSpace(in.DuplicateOf) == "" {
				return d, errors.New("a duplicate rejection names the fact it duplicates (vf-…)")
			}
			d.Labels = append([]domain.FeedbackLabel{domain.LabelDuplicate}, in.Wrong...)
			d.DuplicateOf = strings.TrimSpace(in.DuplicateOf)
		default:
			return d, fmt.Errorf("unknown reject outcome %q", in.Outcome)
		}
	case domain.ActionCorrect:
		if in.Corrected == nil {
			return d, errors.New("a correction needs the corrected assertion")
		}
		d.Corrected = in.Corrected
		wrong := in.Wrong
		proseChanged := consequenceProse(&orig) != consequenceProse(in.Corrected)
		if in.Corrected.Digest() == orig.Digest() {
			// prose-only correction (contract-5): only the consequence
			// statement/remediation changed. The typed assertion was right, so
			// it is labelled exactly [corrected, improved-statement], never wrong-*.
			if len(wrong) > 0 {
				return d, errors.New("a prose-only correction (only the consequence statement/remediation changed) carries no wrong-* labels: the typed assertion was right")
			}
			d.Labels = []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelImprovedStatement}
			break
		}
		if len(wrong) == 0 { // name the aspects that actually changed
			for _, a := range domain.Aspects {
				if orig.Has(a) && orig.AspectDigest(a) != in.Corrected.AspectDigest(a) {
					wrong = append(wrong, aspectLabel[a])
				}
			}
		}
		d.Labels = append([]domain.FeedbackLabel{domain.LabelCorrected}, wrong...)
		if proseChanged { // the prose changed as well as a typed aspect
			d.Labels = append(d.Labels, domain.LabelImprovedStatement)
		}
	case domain.ActionNeedMoreEvidence:
		d.Labels = []domain.FeedbackLabel{domain.LabelInsufficientEvidence}
	case domain.ActionDefer:
	default:
		return d, fmt.Errorf("unknown action %q", in.Action)
	}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	if err := d.Validate(); err != nil {
		return d, cleanErr(err)
	}
	return d, nil
}

// cleanErr flattens a joined validation error to readable lines.
func cleanErr(err error) error {
	var parts []string
	for _, l := range strings.Split(err.Error(), "\n") {
		l = strings.TrimSpace(l)
		if i := strings.Index(l, ": "); strings.HasPrefix(l, "decision ") && i > 0 {
			l = l[i+2:]
		}
		if l != "" {
			parts = append(parts, l)
		}
	}
	return errors.New(strings.Join(parts, "; "))
}

// --- POST /items/{id}/decision ---------------------------------------------------------

func (s *server) decide(w http.ResponseWriter, r *http.Request) {
	ss := s.session(w, r)
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusBadRequest, "error.html", errorView{Title: "Bad request", Message: err.Error()})
		return
	}
	refuse := func(status int, errs ...string) {
		v, err := s.itemView(r, id, r.PostForm, errs)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v.Flashes = s.takeFlashes(ss)
		s.render(w, status, "item.html", v)
	}
	if err := refuseProxy(r); err != nil {
		refuse(http.StatusBadRequest, err.Error())
		return
	}
	action, err := parseAction(r.PostForm.Get("action"))
	if err != nil {
		refuse(http.StatusBadRequest, err.Error())
		return
	}
	rc, err := s.q.Item(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := s.opts.Now().UTC()
	in := decisionInput{Action: action, Reason: r.PostForm.Get("reason"), Reviewer: s.reviewerFrom(r), Decided: now,
		Outcome: r.PostForm.Get("outcome"), DuplicateOf: r.PostForm.Get("duplicate_of")}
	if strings.TrimSpace(in.Reviewer) == "" {
		refuse(http.StatusUnprocessableEntity, "Reviewer name is required: every decision is attributed.")
		return
	}
	if !eligible(rc.Item.Status) {
		refuse(http.StatusConflict, fmt.Sprintf("This item is already %s; a decision cannot be added.", rc.Item.Status))
		return
	}
	if in.Started, err = parseStarted(r.PostForm.Get("started"), now); err != nil {
		refuse(http.StatusBadRequest, err.Error())
		return
	}
	if in.Wrong, err = wrongLabels(r.PostForm["label"]); err != nil {
		refuse(http.StatusBadRequest, err.Error())
		return
	}
	if action == domain.ActionCorrect {
		if rc.Item.Proposed.Empty() {
			refuse(http.StatusUnprocessableEntity, "This question has nothing to correct; use accept, reject, need more evidence or defer.")
			return
		}
		if in.Corrected, err = parseCorrection(r.PostForm, rc.Item.Proposed); err != nil {
			refuse(http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	d, err := buildDecision(rc, in)
	if err != nil {
		refuse(http.StatusUnprocessableEntity, err.Error())
		return
	}
	if _, err := s.q.Decide(r.Context(), []domain.ReviewDecision{d}); err != nil {
		refuse(http.StatusUnprocessableEntity, err.Error())
		return
	}
	setReviewerCookie(w, in.Reviewer)
	s.addFlash(ss, "ok", fmt.Sprintf("Recorded %s on %s.", action, id))
	http.Redirect(w, r, localPath(r.PostForm.Get("return"), "/"), http.StatusSeeOther)
}

// --- correction parsing -------------------------------------------------------------------

func field(f url.Values, k string) string { return strings.TrimSpace(f.Get(k)) }

func optString(f url.Values, k string) *string {
	if v := field(f, k); v != "" {
		return &v
	}
	return nil
}

// parseCondition decodes a condition from JSON, refusing unknown fields.
func parseCondition(raw string) (*domain.Condition, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	var c domain.Condition
	if err := dec.Decode(&c); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing data after the condition")
	}
	return &c, nil
}

// parseCorrection builds the corrected assertion from the edit form. Every
// aspect the original states is re-read from its form fields (the form is
// pre-filled with the proposed values, so untouched fields keep them); an
// aspect without form fields keeps the original. Nothing is dropped.
func parseCorrection(f url.Values, orig domain.SemanticAssertion) (*domain.SemanticAssertion, error) {
	out := orig
	var errs []string
	has := func(prefix string) bool {
		for k := range f {
			if strings.HasPrefix(k, prefix) {
				return true
			}
		}
		return false
	}
	if _, ok := f["c_statement"]; ok {
		out.Statement = field(f, "c_statement")
	}
	if orig.Subject != nil && has("c_subject_") {
		out.Subject = &domain.Subject{
			Family:  domain.SubjectFamily(field(f, "c_subject_family")),
			Product: domain.ProductID(field(f, "c_subject_product")),
			Group:   field(f, "c_subject_group"), Version: field(f, "c_subject_version"), Kind: field(f, "c_subject_kind"),
			Path: field(f, "c_subject_path"), Name: field(f, "c_subject_name"), Component: field(f, "c_subject_component"),
		}
	}
	if orig.Change != nil && has("c_change_") {
		ch := &domain.ChangeSpec{Type: domain.ChangeKind(field(f, "c_change_type")), Before: optString(f, "c_change_before"), After: optString(f, "c_change_after")}
		if raw := field(f, "c_change_replacedby"); raw != "" {
			var sub domain.Subject
			dec := json.NewDecoder(strings.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&sub); err != nil {
				errs = append(errs, "replaced-by is not a valid subject JSON: "+err.Error())
			} else {
				ch.ReplacedBy = &sub
			}
		}
		out.Change = ch
	}
	if orig.Applicability != nil && has("c_app_") {
		app := &domain.Applicability{}
		if exp, err := parseCondition(field(f, "c_app_exposure")); err != nil {
			errs = append(errs, "exposure condition: "+err.Error())
		} else {
			app.Exposure = *exp
		}
		if raw := field(f, "c_app_overlap"); raw != "" {
			if ov, err := parseCondition(raw); err != nil {
				errs = append(errs, "overlap condition: "+err.Error())
			} else {
				app.Overlap = ov
			}
		}
		out.Applicability = app
	}
	if orig.Consequence != nil && has("c_cons_") {
		kind := domain.ConsequenceKind(field(f, "c_cons_kind"))
		out.Consequence = &domain.Consequence{
			Kind: kind, ExposedClass: kind.ExposedClass(), // the class follows the kind
			Statement: field(f, "c_cons_statement"), Remediation: field(f, "c_cons_remediation"),
			Severity: domain.ImpactSeverity(field(f, "c_cons_severity")),
		}
	}
	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	if err := out.Validate(false); err != nil {
		return nil, cleanErr(err)
	}
	if out.Digest() == orig.Digest() && consequenceProse(&orig) == consequenceProse(&out) {
		return nil, errors.New("a correction must change at least one aspect, or the consequence statement/remediation; nothing differs from the proposal (use accept instead)")
	}
	return &out, nil
}

// consequenceProse is the consequence's free text (statement + remediation),
// which aspect digests ignore (mirrors the domain's prose-only rule).
func consequenceProse(a *domain.SemanticAssertion) string {
	if a == nil || a.Consequence == nil {
		return ""
	}
	return a.Consequence.Statement + "\x00" + a.Consequence.Remediation
}
