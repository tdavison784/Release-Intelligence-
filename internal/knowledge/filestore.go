package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// EndpointDir is the release directory of records that state no release.
const EndpointDir = "_endpoint"

// DefaultDir is where the committed knowledge lives, relative to the repository root.
const DefaultDir = "knowledge"

// kindDirs maps each record kind to its directory under <product>/<release>/.
var kindDirs = map[domain.RecordKind]string{
	domain.RecordCandidate:  "candidates",
	domain.RecordProposal:   "proposals",
	domain.RecordValidation: "validations",
	domain.RecordReviewItem: "reviews",
	domain.RecordDecision:   "decisions",
	domain.RecordFact:       "facts",
}

// idKinds maps an id prefix to the kind it names.
var idKinds = map[string]domain.RecordKind{
	domain.CandidateIDPrefix:  domain.RecordCandidate,
	domain.ProposalIDPrefix:   domain.RecordProposal,
	domain.ValidationIDPrefix: domain.RecordValidation,
	domain.ReviewItemIDPrefix: domain.RecordReviewItem,
	domain.DecisionIDPrefix:   domain.RecordDecision,
	domain.FactIDPrefix:       domain.RecordFact,
}

// FileStore is the default Store: one JSON file per record under
// <dir>/<product>/<release>/<kind>/<id>.json (DESIGN.md §8). Writes are
// idempotent by content-derived id and atomic (temp file + rename); every
// record is validated before it is written and again when it is read.
//
// A mutex serialises the writers of one process. Two processes writing the
// same record race benignly: content ids make the files identical.
type FileStore struct {
	dir string
	mu  sync.Mutex
}

// NewFileStore returns a Store over the directory dir (created on first write).
func NewFileStore(dir string) Store { return &FileStore{dir: dir} }

var _ Store = (*FileStore)(nil)

func releaseDir(release string) string {
	if release == "" {
		return EndpointDir
	}
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(release)
}

func kindOfID(id string) (domain.RecordKind, bool) {
	for p, k := range idKinds {
		if strings.HasPrefix(id, p) {
			return k, true
		}
	}
	return "", false
}

// path computes where a record lives; candidates and facts by their own
// product and release, the rest by resolving their candidate (the candidate
// is therefore written first).
func (s *FileStore) path(rec domain.KnowledgeRecord) (string, error) {
	var product domain.ProductID
	var release string
	switch rec.Kind {
	case domain.RecordCandidate:
		product, release = rec.Candidate.Product, rec.Candidate.Release
	case domain.RecordFact:
		product, release = rec.Fact.Product, rec.Fact.Release
	default:
		var candidateID string
		switch rec.Kind {
		case domain.RecordProposal:
			candidateID = rec.Proposal.CandidateID
		case domain.RecordValidation:
			candidateID = rec.Validation.CandidateID
		case domain.RecordReviewItem:
			candidateID = rec.ReviewItem.CandidateID
		case domain.RecordDecision:
			item, err := s.getLocked(rec.Decision.ReviewItemID)
			if err != nil {
				return "", fmt.Errorf("%w: decision %s names review item %s", ErrOrphan, rec.Decision.ID, rec.Decision.ReviewItemID)
			}
			candidateID = item.ReviewItem.CandidateID
		}
		c, err := s.getLocked(candidateID)
		if err != nil || c.Candidate == nil {
			return "", fmt.Errorf("%w: %s %s names candidate %s", ErrOrphan, rec.Kind, rec.ID(), candidateID)
		}
		product, release = c.Candidate.Product, c.Candidate.Release
	}
	dir, ok := kindDirs[rec.Kind]
	if !ok {
		return "", fmt.Errorf("knowledge: unknown record kind %q", rec.Kind)
	}
	return filepath.Join(s.dir, safeName(string(product)), releaseDir(release), dir, rec.ID()+".json"), nil
}

func safeName(s string) string {
	return strings.NewReplacer("/", "_", "\\", "_").Replace(s)
}

// Put validates and writes one record (see Store.Put).
func (s *FileStore) Put(ctx context.Context, rec domain.KnowledgeRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := rec.Validate(); err != nil {
		return fmt.Errorf("knowledge: refusing invalid record: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.putLocked(rec)
}

func (s *FileStore) putLocked(rec domain.KnowledgeRecord) error {
	path, err := s.path(rec)
	if err != nil {
		return err
	}
	// kind-specific reference and basis checks
	switch rec.Kind {
	case domain.RecordFact:
		if err := s.checkFact(*rec.Fact); err != nil {
			return err
		}
	}
	existing, err := readRecord(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// new record
	case err != nil:
		return err
	default:
		same, err := sameRecord(existing, rec)
		if err != nil {
			return err
		}
		if same {
			return nil
		}
		if err := mutableChange(existing, rec); err != nil {
			return err
		}
	}
	return writeRecord(path, rec)
}

// checkFact verifies the references of a fact (its candidates exist) and
// re-proves its per-aspect verification from the stored records.
func (s *FileStore) checkFact(f domain.VerifiedFact) error {
	for _, id := range f.Candidates {
		r, err := s.getLocked(id)
		if err != nil || r.Candidate == nil {
			return fmt.Errorf("%w: fact %s names candidate %s", ErrOrphan, f.ID, id)
		}
	}
	snap, err := s.loadLocked(Query{Product: f.Product, Kinds: []domain.RecordKind{
		domain.RecordValidation, domain.RecordDecision, domain.RecordReviewItem, domain.RecordProposal}})
	if err != nil {
		return err
	}
	vs := map[string]domain.ValidationResult{}
	for _, v := range snap.Validations {
		vs[v.ID] = v
	}
	ds := map[string]domain.ReviewDecision{}
	for _, d := range snap.Decisions {
		ds[d.ID] = d
	}
	is := map[string]domain.ReviewItem{}
	for _, i := range snap.ReviewItems {
		is[i.ID] = i
	}
	ps := map[string]domain.SemanticProposal{}
	for _, p := range snap.Proposals {
		ps[p.ID] = p
	}
	// ValidateFactRecords (not ValidateFactBasis): consensus facts rest on proposals
	if err := domain.ValidateFactRecords(f, domain.FactRecords{Validations: vs, Decisions: ds, Items: is, Proposals: ps}); err != nil {
		return fmt.Errorf("knowledge: fact %s fails basis verification: %w", f.ID, err)
	}
	return nil
}

func sameRecord(a, b domain.KnowledgeRecord) (bool, error) {
	ab, err := json.Marshal(a)
	if err != nil {
		return false, err
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(ab, bb), nil
}

// mutableChange allows only the documented mutations of existing records:
// review item status; fact status, anchors, candidates, and verification
// upgrades (CONTRACT-CHANGE(knowledge): a fact verified by a proxy is later
// human-reviewed under the same id, so each aspect's level may be replaced by
// an equal or more trusted one; ValidateFactBasis re-proves it on write).
func mutableChange(old, next domain.KnowledgeRecord) error {
	switch old.Kind {
	case domain.RecordReviewItem:
		a, b := *old.ReviewItem, *next.ReviewItem
		a.Status, b.Status = "", ""
		if reflect.DeepEqual(a, b) {
			return nil
		}
	case domain.RecordFact:
		a, b := *old.Fact, *next.Fact
		for _, x := range domain.Aspects {
			if !levelNotWeaker(b.AspectLevel(x), a.AspectLevel(x)) {
				return fmt.Errorf("%w: fact %s: %s verification may not be weakened (%s → %s)", ErrConflict, a.ID, x, a.AspectLevel(x), b.AspectLevel(x))
			}
		}
		a.Status, b.Status = "", ""
		a.Anchors, b.Anchors = nil, nil
		a.Candidates, b.Candidates = nil, nil
		a.Verification, b.Verification = nil, nil
		a.AutoApproved, b.AutoApproved = false, false
		if reflect.DeepEqual(a, b) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s %s already exists with different content (only %s may change)", ErrConflict, old.Kind, old.ID(), mutableFields(old.Kind))
}

func mutableFields(k domain.RecordKind) string {
	switch k {
	case domain.RecordReviewItem:
		return "status"
	case domain.RecordFact:
		return "status, anchors, candidates and verification upgrades"
	}
	return "nothing: the kind is immutable"
}

// levelNotWeaker reports whether n is at least as trusted as o.
func levelNotWeaker(n, o domain.VerificationLevel) bool {
	// deterministic and human are equally trusted (domain.AtLeast); consensus
	// and proxy are weaker, in that order.
	rank := map[domain.VerificationLevel]int{domain.VerifiedDeterministic: 0, domain.VerifiedHuman: 0, domain.VerifiedConsensus: 1, domain.VerifiedProxy: 2}
	return rank[n] <= rank[o]
}

// Get returns one record by id.
func (s *FileStore) Get(ctx context.Context, id string) (domain.KnowledgeRecord, error) {
	if err := ctx.Err(); err != nil {
		return domain.KnowledgeRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(id)
}

func (s *FileStore) getLocked(id string) (domain.KnowledgeRecord, error) {
	kind, ok := kindOfID(id)
	if !ok || strings.ContainsAny(id, "/\\") {
		return domain.KnowledgeRecord{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	var found string
	_ = filepath.WalkDir(s.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && d.Name() == id+".json" && filepath.Base(filepath.Dir(p)) == kindDirs[kind] {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	if found == "" {
		return domain.KnowledgeRecord{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return readRecord(found)
}

// Load returns the records matching q.
func (s *FileStore) Load(ctx context.Context, q Query) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(q)
}

func (s *FileStore) loadLocked(q Query) (*Snapshot, error) {
	snap := &Snapshot{}
	wantKind := map[domain.RecordKind]bool{}
	for _, k := range q.Kinds {
		wantKind[k] = true
	}
	wantRelease := map[string]bool{}
	for _, r := range q.Releases {
		wantRelease[releaseDir(r)] = true
	}
	wantStatus := map[domain.FactStatus]bool{}
	for _, st := range q.FactStatus {
		wantStatus[st] = true
	}
	products, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return snap, nil
	}
	if err != nil {
		return nil, err
	}
	for _, p := range products {
		if !p.IsDir() || (q.Product != "" && p.Name() != safeName(string(q.Product))) {
			continue
		}
		releases, err := os.ReadDir(filepath.Join(s.dir, p.Name()))
		if err != nil {
			return nil, err
		}
		for _, r := range releases {
			if !r.IsDir() || (len(wantRelease) > 0 && !wantRelease[r.Name()]) {
				continue
			}
			for kind, dirName := range kindDirs {
				if len(wantKind) > 0 && !wantKind[kind] {
					continue
				}
				dir := filepath.Join(s.dir, p.Name(), r.Name(), dirName)
				files, err := os.ReadDir(dir)
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return nil, err
				}
				for _, f := range files {
					if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
						continue
					}
					rec, err := readRecord(filepath.Join(dir, f.Name()))
					if err != nil {
						return nil, err
					}
					if rec.Kind != kind || rec.ID()+".json" != f.Name() {
						return nil, fmt.Errorf("knowledge: %s: file does not match its record (%s %s)", filepath.Join(dir, f.Name()), rec.Kind, rec.ID())
					}
					switch kind {
					case domain.RecordCandidate:
						snap.Candidates = append(snap.Candidates, *rec.Candidate)
					case domain.RecordProposal:
						snap.Proposals = append(snap.Proposals, *rec.Proposal)
					case domain.RecordValidation:
						snap.Validations = append(snap.Validations, *rec.Validation)
					case domain.RecordReviewItem:
						snap.ReviewItems = append(snap.ReviewItems, *rec.ReviewItem)
					case domain.RecordDecision:
						snap.Decisions = append(snap.Decisions, *rec.Decision)
					case domain.RecordFact:
						f := *rec.Fact
						if len(wantStatus) > 0 && !wantStatus[f.Status] {
							continue
						}
						if q.MinVerification != "" && !f.Level().AtLeast(q.MinVerification) {
							continue
						}
						snap.Facts = append(snap.Facts, f)
					}
				}
			}
		}
	}
	snap.sort()
	return snap, nil
}

// sort orders every list deterministically (creation time, then id), so
// loads, exports and metrics do not depend on directory order.
func (s *Snapshot) sort() {
	sort.Slice(s.Candidates, func(i, j int) bool {
		return lessTimeID(s.Candidates[i].CreatedAt.UnixNano(), s.Candidates[i].ID, s.Candidates[j].CreatedAt.UnixNano(), s.Candidates[j].ID)
	})
	sort.Slice(s.Proposals, func(i, j int) bool {
		a, b := s.Proposals[i], s.Proposals[j]
		return lessTimeID(generatedAt(a), a.ID, generatedAt(b), b.ID)
	})
	sort.Slice(s.Validations, func(i, j int) bool {
		return lessTimeID(s.Validations[i].CheckedAt.UnixNano(), s.Validations[i].ID, s.Validations[j].CheckedAt.UnixNano(), s.Validations[j].ID)
	})
	sort.Slice(s.ReviewItems, func(i, j int) bool {
		return lessTimeID(s.ReviewItems[i].CreatedAt.UnixNano(), s.ReviewItems[i].ID, s.ReviewItems[j].CreatedAt.UnixNano(), s.ReviewItems[j].ID)
	})
	sort.Slice(s.Decisions, func(i, j int) bool {
		return lessTimeID(s.Decisions[i].DecidedAt.UnixNano(), s.Decisions[i].ID, s.Decisions[j].DecidedAt.UnixNano(), s.Decisions[j].ID)
	})
	sort.Slice(s.Facts, func(i, j int) bool {
		return lessTimeID(s.Facts[i].CreatedAt.UnixNano(), s.Facts[i].ID, s.Facts[j].CreatedAt.UnixNano(), s.Facts[j].ID)
	})
}

func generatedAt(p domain.SemanticProposal) int64 {
	if p.Provenance.GeneratedAt == nil {
		return 0
	}
	return p.Provenance.GeneratedAt.UnixNano()
}

func lessTimeID(ta int64, ia string, tb int64, ib string) bool {
	if ta != tb {
		return ta < tb
	}
	return ia < ib
}

func readRecord(path string) (domain.KnowledgeRecord, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return domain.KnowledgeRecord{}, err
	}
	var rec domain.KnowledgeRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return domain.KnowledgeRecord{}, fmt.Errorf("knowledge: %s: %w", path, err)
	}
	if err := rec.Validate(); err != nil {
		return domain.KnowledgeRecord{}, fmt.Errorf("knowledge: %s: %w", path, err)
	}
	return rec, nil
}

// writeRecord writes the record as indented JSON with a trailing newline,
// atomically, so a reader never sees a partial file.
func writeRecord(path string, rec domain.KnowledgeRecord) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
