package app

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// KnowledgeSet is the verified knowledge `ri impact` / `ri eval` evaluate:
// the facts whose verification the records prove, the review contexts of
// those facts (transfer reporting), and every record refused on the way.
type KnowledgeSet struct {
	Dir      string
	Snapshot *knowledge.Snapshot
	Facts    []domain.VerifiedFact
	Contexts map[string][]string
	Warnings []string
}

// LoadKnowledge reads the knowledge records under dir (the committed
// knowledge/ layout of DESIGN.md §8; every *.json below dir is a
// domain.KnowledgeRecord) read-only, validates each record, and keeps the
// facts whose per-aspect verification is proven by the loaded records
// (impact.VerifyFacts). Invalid records are refused with a warning, never
// used.
//
// The knowledge lane's knowledge.Store (NewFileStore) is the write path; this
// reader only needs the same files, so the two stay interchangeable.
func LoadKnowledge(dir string) (*KnowledgeSet, error) {
	ks := &KnowledgeSet{Dir: dir, Snapshot: &knowledge.Snapshot{}}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("knowledge directory %q: not a directory", dir)
	}
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".json") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge directory %q: %w", dir, err)
	}
	sort.Strings(files)
	s := ks.Snapshot
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var r domain.KnowledgeRecord
		if err := json.Unmarshal(b, &r); err != nil {
			ks.Warnings = append(ks.Warnings, fmt.Sprintf("%s: not a knowledge record: %v", p, err))
			continue
		}
		if err := r.Validate(); err != nil {
			ks.Warnings = append(ks.Warnings, fmt.Sprintf("%s: invalid %s record, ignored: %v", p, r.Kind, err))
			continue
		}
		switch r.Kind {
		case domain.RecordCandidate:
			s.Candidates = append(s.Candidates, *r.Candidate)
		case domain.RecordProposal:
			s.Proposals = append(s.Proposals, *r.Proposal)
		case domain.RecordValidation:
			s.Validations = append(s.Validations, *r.Validation)
		case domain.RecordReviewItem:
			s.ReviewItems = append(s.ReviewItems, *r.ReviewItem)
		case domain.RecordDecision:
			s.Decisions = append(s.Decisions, *r.Decision)
		case domain.RecordFact:
			s.Facts = append(s.Facts, *r.Fact)
		}
	}
	facts, rejected := impact.VerifyFacts(s)
	ks.Facts = facts
	for _, r := range rejected {
		ks.Warnings = append(ks.Warnings, "fact refused: "+r)
	}
	ks.Contexts = impact.ReviewContexts(s)
	return ks, nil
}

// ParseVerificationLevel reads a -min-verification value ("" = human).
func ParseVerificationLevel(s string) (domain.VerificationLevel, error) {
	if s == "" {
		return domain.VerifiedHuman, nil
	}
	l := domain.VerificationLevel(s)
	if !l.Valid() {
		return "", fmt.Errorf("%w: -min-verification must be one of %v, got %q", ErrUsage, domain.VerificationLevels, s)
	}
	return l, nil
}
