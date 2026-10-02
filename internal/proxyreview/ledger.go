package proxyreview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Ledger outcomes and stages.
const (
	OutcomeDecided = "decided" // a decision was recorded
	OutcomeFailed  = "failed"  // no decision: the call, the verdict or the record was refused
	OutcomeSkipped = "skipped" // not called: the item was no longer pending (script)

	StageCall    = "call"    // the model call failed or returned no structured output (script)
	StageVerdict = "verdict" // the verdict was refused (schema, correction, stale item)
	StageRecord  = "record"  // the queue refused the decision
)

// LedgerEntry is one line of the run ledger (JSONL): one per item attempted,
// decided or failed. Failures are recorded, never dropped, never retried
// blindly. It also records, per decision, who authored the proposals under
// review, so self-family review can be split out (brief step 3).
type LedgerEntry struct {
	ItemID       string                `json:"itemId"`
	CandidateID  string                `json:"candidateId,omitempty"`
	Product      domain.ProductID      `json:"product,omitempty"`
	Release      string                `json:"release,omitempty"`
	QuestionType domain.QuestionType   `json:"questionType,omitempty"`
	Priority     domain.ReviewPriority `json:"priority,omitempty"`
	Outcome      string                `json:"outcome"`
	Stage        string                `json:"stage,omitempty"`
	Error        string                `json:"error,omitempty"`
	// Verdict is the model's action as answered (e.g. evidence-sufficient);
	// Action the recorded decision action.
	Verdict       string                 `json:"verdict,omitempty"`
	DecisionID    string                 `json:"decisionId,omitempty"`
	Action        domain.DecisionAction  `json:"action,omitempty"`
	Labels        []domain.FeedbackLabel `json:"labels,omitempty"`
	Confidence    string                 `json:"confidence,omitempty"`
	ResultingFact string                 `json:"resultingFact,omitempty"`
	FactLevel     string                 `json:"factLevel,omitempty"`
	FollowUps     []string               `json:"followUps,omitempty"`
	ProxyModel    string                 `json:"proxyModel,omitempty"`
	ModelVersion  string                 `json:"modelVersion,omitempty"`
	CallID        string                 `json:"callId,omitempty"`
	PromptDigest  string                 `json:"promptDigest,omitempty"`
	StartedAt     time.Time              `json:"startedAt,omitzero"`
	DecidedAt     time.Time              `json:"decidedAt,omitzero"`
	DurationMs    int64                  `json:"durationMs,omitempty"`
	CostUSD       float64                `json:"costUSD,omitempty"`
	// Authors of the proposals under review (anonymised in the prompt).
	ProposalModels   []string `json:"proposalModels,omitempty"`
	ProposalFamilies []string `json:"proposalFamilies,omitempty"`
	// SelfModel: a proposal under review was authored by the proxy's own
	// model; SelfFamily: by its model family (domain.ModelFamily).
	SelfModel  bool `json:"selfModel"`
	SelfFamily bool `json:"selfFamily"`
	// HumanDecisionsShown counts earlier human decisions in the prompt.
	HumanDecisionsShown int `json:"humanDecisionsShown"`
}

// Authors fills the authorship fields from the request.
func (e *LedgerEntry) Authors(req *Request, proxyModel string) {
	models, fams := map[string]bool{}, map[string]bool{}
	for _, p := range req.Proposals {
		models[p.Model] = true
		fams[p.Family] = true
	}
	e.ProposalModels, e.ProposalFamilies = keys(models), keys(fams)
	e.SelfModel = models[proxyModel]
	e.SelfFamily = fams[domain.ModelFamily(proxyModel)]
	e.HumanDecisionsShown = req.HumanDecisionsShown
	e.CandidateID, e.Product, e.Release = req.CandidateID, req.Product, req.Release
	e.QuestionType, e.Priority, e.PromptDigest = req.QuestionType, req.Priority, req.PromptDigest
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// AppendLedger appends one entry to a JSONL ledger.
func AppendLedger(path string, e LedgerEntry) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ReadLedger reads a JSONL ledger.
func ReadLedger(path string) ([]LedgerEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []LedgerEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for n := 1; sc.Scan(); n++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e LedgerEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}
