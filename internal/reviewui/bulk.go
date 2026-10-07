package reviewui

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Bulk row states.
const (
	bulkOK      = "ok"      // will be submitted
	bulkBlocked = "blocked" // bulk accept refused until the item is opened (high priority / disagreement)
	bulkInvalid = "invalid" // cannot be decided (already decided, validation fails, not found)
)

type bulkRow struct {
	ID    string
	Title string
	State string
	Why   string
	// Priority and Disagrees are shown on the confirmation page.
	Priority  domain.ReviewPriority
	Disagrees bool
	decision  domain.ReviewDecision
}

type bulkView struct {
	Action    domain.DecisionAction
	Reason    string
	Labels    []domain.FeedbackLabel
	Reviewer  string
	Started   string
	Return    string
	Rows      []bulkRow
	OK        int
	Blocked   int
	Invalid   int
	Errors    []string
	Flashes   []flash
	CanSubmit bool
}

var bulkActions = []domain.DecisionAction{domain.ActionAccept, domain.ActionReject, domain.ActionDefer, domain.ActionNeedMoreEvidence}

// disagrees reports whether the item's proposals disagree on any aspect.
func disagrees(rc *knowledge.ReviewContext) bool {
	for _, a := range rc.Agreement {
		if len(a.Groups) >= 2 {
			return true
		}
	}
	return slices.Contains(rc.Item.Routing.Signals, domain.SignalModelsDisagree)
}

// bulkGuard says why a bulk ACCEPT of this item is refused until the item was
// opened in this session ("" = no guard applies).
func bulkGuard(rc *knowledge.ReviewContext) string {
	var why []string
	if rc.Item.Routing.Priority == domain.PriorityHigh {
		why = append(why, "high priority")
	}
	if disagrees(rc) {
		why = append(why, "the models' proposals disagree")
	}
	if len(why) == 0 {
		return ""
	}
	return "bulk accept needs this item opened first: " + strings.Join(why, " and ")
}

func uniq(xs []string) []string {
	var out []string
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// planBulk evaluates every selected item (never silently dropping one).
func (s *server) planBulk(r *http.Request, ss *session, ids []string, in decisionInput) []bulkRow {
	var rows []bulkRow
	for _, id := range ids {
		row := bulkRow{ID: id, State: bulkOK}
		rc, err := s.q.Item(r.Context(), id)
		if err != nil {
			row.State, row.Why = bulkInvalid, err.Error()
			rows = append(rows, row)
			continue
		}
		row.Title = rc.Candidate.Title
		row.Priority = rc.Item.Routing.Priority
		row.Disagrees = disagrees(rc)
		switch {
		case !eligible(rc.Item.Status):
			row.State, row.Why = bulkInvalid, fmt.Sprintf("already %s", rc.Item.Status)
		default:
			d, err := buildDecision(rc, in)
			if err != nil {
				row.State, row.Why = bulkInvalid, err.Error()
				break
			}
			row.decision = d
			if g := bulkGuard(rc); in.Action == domain.ActionAccept && g != "" && !s.wasExpanded(ss, id) {
				row.State, row.Why = bulkBlocked, g
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *server) bulk(w http.ResponseWriter, r *http.Request) {
	ss := s.session(w, r)
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusBadRequest, "error.html", errorView{Title: "Bad request", Message: err.Error()})
		return
	}
	back := localPath(r.PostForm.Get("return"), "/")
	bad := func(status int, msg string) {
		s.render(w, status, "error.html", errorView{Title: "Bulk action refused", Message: msg + " Nothing was recorded."})
	}
	if err := refuseProxy(r); err != nil {
		bad(http.StatusBadRequest, err.Error())
		return
	}
	action, err := parseAction(r.PostForm.Get("action"))
	if err != nil {
		bad(http.StatusBadRequest, err.Error())
		return
	}
	if !slices.Contains(bulkActions, action) {
		bad(http.StatusBadRequest, "Corrections are individual-only: a correction needs per-item edits. Open the item to correct it.")
		return
	}
	if r.PostForm.Get("outcome") == string(domain.LabelDuplicate) {
		bad(http.StatusBadRequest, "A duplicate rejection names a fact per item and is individual-only.")
		return
	}
	wrong, err := wrongLabels(r.PostForm["label"])
	if err != nil {
		bad(http.StatusBadRequest, err.Error())
		return
	}
	now := s.opts.Now().UTC()
	started, err := parseStarted(r.PostForm.Get("started"), now)
	if err != nil {
		bad(http.StatusBadRequest, err.Error())
		return
	}
	ids := uniq(r.PostForm["item"])
	if len(ids) == 0 {
		s.addFlash(ss, "warn", "No items were selected.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	reviewer := s.reviewerFrom(r)
	in := decisionInput{Action: action, Reason: r.PostForm.Get("reason"), Reviewer: reviewer, Started: started, Decided: now, Wrong: wrong}

	v := &bulkView{Action: action, Reason: strings.TrimSpace(in.Reason), Labels: wrong, Reviewer: reviewer,
		Started: started.Format(time.RFC3339Nano), Return: back}
	if strings.TrimSpace(reviewer) == "" {
		v.Errors = append(v.Errors, "Reviewer name is required: every decision is attributed.")
	}
	rows := s.planBulk(r, ss, ids, in)
	if r.PostForm.Get("exclude_blocked") == "1" {
		rows = slices.DeleteFunc(rows, func(b bulkRow) bool { return b.State == bulkBlocked })
		v.Errors = nil // recomputed below
		if strings.TrimSpace(reviewer) == "" {
			v.Errors = append(v.Errors, "Reviewer name is required: every decision is attributed.")
		}
	}
	for _, b := range rows {
		switch b.State {
		case bulkOK:
			v.OK++
		case bulkBlocked:
			v.Blocked++
		default:
			v.Invalid++
		}
	}
	v.Rows = rows
	if v.OK < 2 && v.Blocked == 0 {
		v.Errors = append(v.Errors, "A bulk action needs at least two decidable items; use the item's own controls for a single decision.")
	}
	v.CanSubmit = v.Blocked == 0 && v.OK >= 2 && len(v.Errors) == 0

	if r.PostForm.Get("confirm") != "1" || r.PostForm.Get("exclude_blocked") == "1" {
		v.Flashes = s.takeFlashes(ss)
		s.render(w, http.StatusOK, "bulk.html", v)
		return
	}
	if !v.CanSubmit {
		status := http.StatusUnprocessableEntity
		if v.Blocked > 0 {
			status = http.StatusConflict
		}
		s.render(w, status, "bulk.html", v)
		return
	}

	// One decision per item, sharing a batch id: through ONE Decide call.
	var ds []domain.ReviewDecision
	var okIDs []string
	for _, b := range rows {
		if b.State == bulkOK {
			ds = append(ds, b.decision)
			okIDs = append(okIDs, b.ID)
		}
	}
	batch := domain.BatchID(okIDs, reviewer, now)
	for i := range ds {
		ds[i].BatchID, ds[i].BatchSize = batch, len(ds)
		if err := ds[i].Validate(); err != nil {
			v.Errors = append(v.Errors, cleanErr(err).Error())
			v.CanSubmit = false
			s.render(w, http.StatusUnprocessableEntity, "bulk.html", v)
			return
		}
	}
	if _, err := s.q.Decide(r.Context(), ds); err != nil {
		v.Errors = append(v.Errors, "The queue refused the batch; nothing was recorded: "+err.Error())
		v.CanSubmit = false
		s.render(w, http.StatusUnprocessableEntity, "bulk.html", v)
		return
	}
	setReviewerCookie(w, reviewer)
	s.addFlash(ss, "ok", fmt.Sprintf("Recorded %d × %s as batch %s.", len(ds), action, batch))
	for _, b := range rows {
		if b.State == bulkInvalid {
			s.addFlash(ss, "error", fmt.Sprintf("Skipped %s: %s", b.ID, b.Why))
		}
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
