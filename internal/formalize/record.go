package formalize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// SchemaVersion is the serialisation version of a formalization record.
const SchemaVersion = "ri.dev/formalization/v1alpha1"

// RecordIDPrefix prefixes formalization record ids.
const RecordIDPrefix = "fmz-"

// Statuses of a formalization.
const (
	StatusComplete = "complete" // an answer passed the floor
	StatusFailed   = "failed"   // no answer passed within MaxAttempts: recorded, never repaired
)

// Record is one formalization of one assessment: its hypotheses, or why
// it has none, or its recorded failure. It is stored beside the run record,
// which it never changes (internal/formalize Store): the run's findings,
// decision and review stay as they were, and its hypotheses reach knowledge
// only through ExportHypotheses, verification and a human decision.
type Record struct {
	SchemaVersion string `json:"schemaVersion"`
	// ID is RecordID(investigation, assessment, input, callId).
	ID            string `json:"id"`
	Investigation string `json:"investigation"`
	Assessment    string `json:"assessment"`
	ChangeItem    string `json:"changeItem"`
	Status        string `json:"status"`
	// Input is the digest of the call's input (Precomputed).
	Input              string              `json:"input"`
	Uncertainties      []Uncertainty       `json:"uncertainties,omitempty"`
	Hypotheses         []domain.Hypothesis `json:"hypotheses,omitempty"`
	NoHypothesisReason string              `json:"noHypothesisReason,omitempty"`
	Failure            *Failure            `json:"failure,omitempty"`
	// RefinementNotes say why a rule hypothesis got no scope-refinement
	// candidate for a consumer role of its subject kind (refine.go;
	// formalize/v2 on). CONTRACT-CHANGE(verify2): additive.
	RefinementNotes []string          `json:"refinementNotes,omitempty"`
	StatementKind   string            `json:"statementKind"` // domain.StatementUnverifiedProse
	Provenance      domain.Provenance `json:"provenance"`    // method ai
}

// Failure records a formalization no answer of which passed the floor.
type Failure struct {
	Attempts   int      `json:"attempts"`
	Violations []string `json:"violations"` // the last answer's
	Answers    []string `json:"answers"`    // digest of every rejected answer
}

// RecordID derives a record id.
func RecordID(investigation, assessment, input, callID string) string {
	return RecordIDPrefix + domain.ShortHash(investigation, assessment, input, callID)
}

// Validate checks the record's shape.
func (r Record) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("formalization %s: %s", r.ID, fmt.Sprintf(format, args...)))
	}
	if r.SchemaVersion != SchemaVersion {
		bad("schemaVersion %q", r.SchemaVersion)
	}
	if r.ID != RecordID(r.Investigation, r.Assessment, r.Input, r.Provenance.CallID) {
		bad("id is not derived")
	}
	if !strings.HasPrefix(r.Investigation, domain.InvestigationIDPrefix) || !strings.HasPrefix(r.Assessment, domain.AgentAssessmentIDPrefix) ||
		!strings.HasPrefix(r.ChangeItem, domain.ChangeItemIDPrefix) {
		bad("investigation, assessment and changeItem are required")
	}
	if r.StatementKind != domain.StatementUnverifiedProse {
		bad("statementKind must be %s", domain.StatementUnverifiedProse)
	}
	producer := ProducerFor(r.Provenance.PromptVersion)
	if r.Provenance.Method != domain.MethodAI || r.Provenance.Producer != producer {
		bad("provenance is ai, producer %s", producer)
	}
	if err := r.Provenance.Validate(); err != nil {
		bad("provenance: %v", err)
	}
	if r.Provenance.Model == "" || r.Provenance.ModelVersion == "" || r.Provenance.PromptVersion == "" || r.Provenance.PromptDigest == "" ||
		r.Provenance.GeneratedAt == nil || r.Provenance.CallID == "" || len(r.Provenance.InputEvidence) == 0 {
		bad("provenance is incomplete (model, modelVersion, promptVersion, promptDigest, generatedAt, callId, inputEvidence)")
	}
	for i, u := range r.Uncertainties {
		if u.ID != UncertaintyIDPrefix+domain.ShortHash(u.Kind, u.Text) {
			bad("uncertainties[%d]: id is not derived", i)
		}
	}
	switch r.Status {
	case StatusComplete:
		if r.Failure != nil {
			bad("a complete formalization has no failure")
		}
		if len(r.Hypotheses) == 0 && strings.TrimSpace(r.NoHypothesisReason) == "" {
			bad("a complete formalization has hypotheses or says why it has none")
		}
	case StatusFailed:
		if r.Failure == nil || r.Failure.Attempts < 1 || len(r.Failure.Answers) != r.Failure.Attempts || len(r.Failure.Violations) == 0 {
			bad("a failed formalization records its attempts, rejected answers and violations")
		}
		if len(r.Hypotheses) != 0 || r.NoHypothesisReason != "" {
			bad("a failed formalization has no hypotheses")
		}
	default:
		bad("unknown status %q", r.Status)
	}
	seen := map[string]bool{}
	for _, h := range r.Hypotheses {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
		if seen[h.ID] {
			bad("duplicate hypothesis %s", h.ID)
		}
		seen[h.ID] = true
		if h.ChangeItem != r.ChangeItem {
			bad("hypothesis %s is about %s", h.ID, h.ChangeItem)
		}
		if h.ProposedVerification == nil || h.ProposedVerification.Assessment != r.Assessment {
			bad("hypothesis %s does not name its assessment", h.ID)
		}
		if h.Mitigation == nil && h.MitigationRejected == "" {
			bad("hypothesis %s carries neither a mitigation nor the limitation that kept it out of the grammar", h.ID)
		}
		if h.Provenance.Producer != producer || h.Provenance.CallID != r.Provenance.CallID {
			bad("hypothesis %s is not of this formalization's call", h.ID)
		}
	}
	// a refinement refines a rule hypothesis of the same record
	for _, h := range r.Hypotheses {
		if v := h.ProposedVerification; v != nil && v.Refinement != nil {
			ok := false
			for _, b := range r.Hypotheses {
				ok = ok || (b.ID == v.Refinement.Refines && b.Mitigation != nil && b.Mitigation.ProposedRule.Assertion.Digest() == v.Refinement.BaseRule &&
					(b.ProposedVerification == nil || b.ProposedVerification.Refinement == nil))
			}
			if !ok {
				bad("refinement %s does not refine a rule hypothesis of this record (with its rule digest)", h.ID)
			}
		}
	}
	return errors.Join(errs...)
}

// Store keeps formalization records as <dir>/<investigation>/<id>.json,
// beside (never inside) the run records.
type Store struct{ Dir string }

// StateDir is the store directory of an RI state directory.
func StateDir(state string) string { return filepath.Join(state, "investigations", "formalizations") }

var storeID = regexp.MustCompile(`^[a-z]+-[0-9a-f]{6,64}$`)

// Put writes a record (validated). Records are content-addressed: an
// existing file with the same id is left as it is.
func (s Store) Put(r *Record) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if !storeID.MatchString(r.Investigation) || !storeID.MatchString(r.ID) {
		return fmt.Errorf("formalize: bad ids %q / %q", r.Investigation, r.ID)
	}
	dir := filepath.Join(s.Dir, r.Investigation)
	path := filepath.Join(dir, r.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// List reads every record of an investigation, sorted by id (none: empty).
func (s Store) List(ctx context.Context, investigation string) ([]Record, error) {
	if !storeID.MatchString(investigation) {
		return nil, fmt.Errorf("formalize: %q is not an investigation id", investigation)
	}
	paths, err := filepath.Glob(filepath.Join(s.Dir, investigation, RecordIDPrefix+"*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []Record
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var r Record
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("formalize: %s: %w", p, err)
		}
		if r.Investigation != investigation {
			return nil, fmt.Errorf("formalize: %s holds a record of %s", p, r.Investigation)
		}
		out = append(out, r)
	}
	return out, nil
}

// Hypotheses are the hypotheses of the complete records, sorted by id.
func Hypotheses(rs []Record) []domain.Hypothesis {
	var out []domain.Hypothesis
	for _, r := range rs {
		if r.Status == StatusComplete {
			out = append(out, r.Hypotheses...)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
