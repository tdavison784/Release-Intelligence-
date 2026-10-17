// Command kbad builds one PROTOCOL.md §5 S3 negative-control knowledge tree
// from K1 (docs/phase5/learning-loop/PROTOCOL.md). It mutates a COPY of K1
// (made by scripts/phase5/kbad.sh) in place; it never touches knowledge/.
//
// Every mutation that changes what a fact says is minted as a fully
// well-formed record chain (review item, decision, fact whose per-aspect
// verification re-proves from its records), because the dangerous case is a
// fact that LOOKS properly promoted but is wrong or inapplicable; a malformed
// fact would be refused by the loader and prove nothing. The pipeline must
// still not clear anything it should not.
//
//	a proxy     the promoted fact, verified by an AI proxy reviewer only (never human)
//	b refuted   the promotion gate refused: no fact, the review item is still open
//	c release   the fact introduced by a release outside the transition
//	d pin       the pinned value in the applicability is wrong
//	e superseded  the fact is superseded
//	f retracted   the fact is retracted
//	g undecidable the applicability also needs a condition no environment can decide
//	h contradict  a second, active fact for the same subject says the opposite change
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

func main() {
	dir := flag.String("dir", "", "knowledge tree to mutate in place (a copy of K1)")
	factID := flag.String("fact", "", "id of the promoted fact (vf-…)")
	mut := flag.String("mutation", "", "a|b|c|d|e|f|g|h")
	flag.Parse()
	if *dir == "" || *factID == "" || *mut == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*dir, *factID, *mut); err != nil {
		fmt.Fprintln(os.Stderr, "kbad:", err)
		os.Exit(1)
	}
}

type tree struct {
	dir   string
	store knowledge.Store
	snap  *knowledge.Snapshot
}

func run(dir, factID, mut string) error {
	ks, err := app.LoadKnowledge(dir)
	if err != nil {
		return err
	}
	t := &tree{dir: dir, store: knowledge.NewFileStore(dir), snap: ks.Snapshot}
	var f *domain.VerifiedFact
	for i := range t.snap.Facts {
		if t.snap.Facts[i].ID == factID {
			f = &t.snap.Facts[i]
		}
	}
	if f == nil {
		return fmt.Errorf("fact %s is not in %s", factID, dir)
	}
	if f.Status != domain.FactActive {
		return fmt.Errorf("fact %s is %s, not active", factID, f.Status)
	}
	var minted string
	switch mut {
	case "a":
		a := cloneAssertion(f.Assertion)
		minted, err = t.reissue(f, a, f.Release, domain.ReviewerProxy, true)
	case "b":
		err = t.refuse(f)
	case "c":
		rel, e := outsideRelease(f.Release)
		if e != nil {
			return e
		}
		minted, err = t.reissue(f, cloneAssertion(f.Assertion), rel, domain.ReviewerHuman, true)
	case "d":
		a := cloneAssertion(f.Assertion)
		if err = wrongPin(&a); err == nil {
			minted, err = t.reissue(f, a, f.Release, domain.ReviewerHuman, true)
		}
	case "e":
		err = t.setStatus(f, domain.FactSuperseded)
	case "f":
		err = t.setStatus(f, domain.FactRetracted)
	case "g":
		a := cloneAssertion(f.Assertion)
		a.Applicability.Exposure = domain.Condition{Op: domain.OpAll, Of: []domain.Condition{a.Applicability.Exposure, {
			Op: domain.OpUndecidable, Reason: domain.UnknownRuntimeBehaviorGap,
			Needed: "kbad: a runtime property of the environment that no static evidence can establish"}}}
		minted, err = t.reissue(f, a, f.Release, domain.ReviewerHuman, true)
	case "h":
		a := cloneAssertion(f.Assertion)
		if err = contradict(&a); err == nil {
			minted, err = t.reissue(f, a, f.Release, domain.ReviewerHuman, false)
		}
	default:
		return fmt.Errorf("unknown mutation %q", mut)
	}
	if err != nil {
		return err
	}
	return report(dir, factID, minted, mut)
}

func cloneAssertion(a domain.SemanticAssertion) domain.SemanticAssertion {
	var out domain.SemanticAssertion
	raw, _ := jsonMarshal(a)
	_ = jsonUnmarshal(raw, &out)
	return out
}

// outsideRelease is a release the fact cannot belong to: two minors on.
func outsideRelease(rel string) (string, error) {
	parts := strings.Split(rel, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("release %q is not major.minor[.patch]", rel)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("release %q: %w", rel, err)
	}
	return fmt.Sprintf("%s.%d.0", parts[0], minor+2), nil
}

// wrongPin changes the first pinned value in the applicability; with none,
// it inverts the first set/unset state.
func wrongPin(a *domain.SemanticAssertion) error {
	if a.Applicability == nil {
		return fmt.Errorf("the fact has no applicability to mutate")
	}
	var walk func(c *domain.Condition) bool
	walk = func(c *domain.Condition) bool {
		if len(c.Values) > 0 {
			c.Values = []string{strconv.Quote("kbad-wrong-value")}
			return true
		}
		for i := range c.Of {
			if walk(&c.Of[i]) {
				return true
			}
		}
		return false
	}
	conds := []*domain.Condition{&a.Applicability.Exposure}
	if a.Applicability.Overlap != nil {
		conds = append(conds, a.Applicability.Overlap)
	}
	for _, c := range conds {
		if walk(c) {
			return nil
		}
	}
	var flip func(c *domain.Condition) bool
	flip = func(c *domain.Condition) bool {
		switch c.State {
		case domain.StateSet:
			c.State = domain.StateUnset
			return true
		case domain.StateUnset:
			c.State = domain.StateSet
			return true
		}
		for i := range c.Of {
			if flip(&c.Of[i]) {
				return true
			}
		}
		return false
	}
	for _, c := range conds {
		if flip(c) {
			return nil
		}
	}
	return fmt.Errorf("the applicability has no pinned value or set/unset state to corrupt")
}

// contradict makes the change say the opposite (before and after swapped).
func contradict(a *domain.SemanticAssertion) error {
	if a.Change == nil {
		return fmt.Errorf("the fact has no change to contradict")
	}
	if a.Change.Before != nil && a.Change.After != nil {
		a.Change.Before, a.Change.After = a.Change.After, a.Change.Before
		return nil
	}
	v := "kbad-contradiction"
	a.Change.After = &v
	return nil
}

// decisionFor finds the human decision a fact's verification rests on, with
// its review item.
func (t *tree) decisionFor(f *domain.VerifiedFact) (domain.ReviewDecision, domain.ReviewItem, error) {
	ds := map[string]domain.ReviewDecision{}
	for _, d := range t.snap.Decisions {
		ds[d.ID] = d
	}
	items := map[string]domain.ReviewItem{}
	for _, it := range t.snap.ReviewItems {
		items[it.ID] = it
	}
	ids := []string{f.AuditedBy}
	for _, v := range f.Verification {
		ids = append(ids, v.Basis...)
	}
	for _, id := range ids {
		if d, ok := ds[id]; ok && d.ReviewerKind == domain.ReviewerHuman {
			if it, ok := items[d.ReviewItemID]; ok {
				return d, it, nil
			}
		}
	}
	return domain.ReviewDecision{}, domain.ReviewItem{}, fmt.Errorf("fact %s rests on no human decision with a review item", f.ID)
}

// reissue mints fact f with assertion a / release rel as a well-formed
// record chain at the given reviewer level. dropOriginal removes f (the
// mutant replaces it); otherwise both stay active.
func (t *tree) reissue(f *domain.VerifiedFact, a domain.SemanticAssertion, rel string, kind domain.ReviewerKind, dropOriginal bool) (string, error) {
	oldDec, oldItem, err := t.decisionFor(f)
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	nf := *f
	nf.Release, nf.Assertion = rel, a
	nf.ID = domain.VerifiedFactID(nf.Product, nf.Release, nf.Assertion)
	nf.Anchors = nil
	for _, an := range f.Anchors {
		an.Release = rel
		nf.Anchors = append(nf.Anchors, an)
	}
	item := oldItem
	item.Proposed, item.Status = a, domain.ReviewDecided
	item.ID = domain.ReviewItemID(item.CandidateID, item.QuestionType, a)
	dec := oldDec
	dec.ReviewItemID, dec.Action, dec.Corrected = item.ID, domain.ActionAccept, nil
	dec.Original, dec.ResultingFact, dec.ReviewerKind = &a, nf.ID, kind
	if kind == domain.ReviewerProxy {
		gen := oldDec.DecidedAt
		dec.Reviewer = "proxy:kbad-negative-control"
		dec.ProxyProvenance = &domain.Provenance{Method: domain.MethodAI, Producer: "knowledge.proxy@v1", Confidence: "medium",
			Model: "kbad-synthetic", ModelVersion: "kbad-synthetic", PromptVersion: "kbad/v1", PromptDigest: "sha256:" + strings.Repeat("0", 64),
			InputEvidence: []domain.EvidenceID{"ev-kbad"}, GeneratedAt: &gen, CallID: "kbad-negative-control", Provider: "kbad"}
		dec.Reason = "kbad negative control: a proxy reviewer, never human"
	}
	// a later instant keeps the id distinct from the decision the original fact rests on
	dec.DecidedAt = dec.DecidedAt.Add(time.Second)
	dec.ID = domain.DecisionID(item.ID, dec.Reviewer, dec.DecidedAt)
	nf.Verification = nil
	for _, v := range f.Verification {
		if v.Level == domain.VerifiedHuman {
			v.Level = domain.VerificationLevel(kind)
			v.Basis = []string{dec.ID}
		}
		nf.Verification = append(nf.Verification, v)
	}
	nf.AuditedBy = ""
	if kind == domain.ReviewerHuman {
		nf.AuditedBy = dec.ID
	}
	if dropOriginal {
		if err := removeFact(t.dir, f.ID); err != nil {
			return "", err
		}
	}
	for _, e := range []any{item, dec, nf} {
		rec, err := domain.NewRecord(e)
		if err != nil {
			return "", err
		}
		if err := t.store.Put(ctx, rec); err != nil {
			return "", fmt.Errorf("minting %T: %w", e, err)
		}
	}
	return nf.ID, nil
}

// refuse models a refused promotion: the fact and the decisions that
// produced it are gone and its review item is pending again.
func (t *tree) refuse(f *domain.VerifiedFact) error {
	if err := removeFact(t.dir, f.ID); err != nil {
		return err
	}
	for _, d := range t.snap.Decisions {
		if d.ResultingFact != f.ID {
			continue
		}
		if err := removeByID(t.dir, "decisions", d.ID); err != nil {
			return err
		}
		for _, it := range t.snap.ReviewItems {
			if it.ID == d.ReviewItemID {
				it.Status = domain.ReviewPending
				rec, err := domain.NewRecord(it)
				if err != nil {
					return err
				}
				if err := writeOver(t.dir, "reviews", it.ID, rec); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (t *tree) setStatus(f *domain.VerifiedFact, s domain.FactStatus) error {
	nf := *f
	nf.Status = s
	rec, err := domain.NewRecord(nf)
	if err != nil {
		return err
	}
	return t.store.Put(context.Background(), rec)
}

func removeFact(dir, id string) error { return removeByID(dir, "facts", id) }

func removeByID(dir, kindDir, id string) error {
	n := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == id+".json" && filepath.Base(filepath.Dir(p)) == kindDir {
			n++
			return os.Remove(p)
		}
		return err
	})
	if err == nil && n == 0 {
		err = fmt.Errorf("%s/%s.json not found under %s", kindDir, id, dir)
	}
	return err
}

// writeOver rewrites an existing record file (a review item's status is the
// one mutable field of that kind; the store handles it too, but a pending
// item replacing a decided one is a status change only).
func writeOver(dir, kindDir, id string, rec domain.KnowledgeRecord) error {
	return knowledge.NewFileStore(dir).Put(context.Background(), rec)
}

// report reloads the mutated tree the way the pipeline does and says what
// the mutant looks like to it.
func report(dir, factID, minted, mut string) error {
	ks, err := app.LoadKnowledge(dir)
	if err != nil {
		return err
	}
	facts, rejected := impact.VerifyFacts(ks.Snapshot)
	fmt.Printf("mutation %s: original fact %s", mut, factID)
	if minted != "" {
		fmt.Printf(", minted %s", minted)
	}
	fmt.Println()
	for _, f := range facts {
		if f.ID == factID || f.ID == minted {
			fmt.Printf("  loads: %s status=%s level=%s release=%s\n", f.ID, f.Status, f.Level(), f.Release)
		}
	}
	for _, r := range rejected {
		fmt.Printf("  REJECTED by the loader: %s\n", r)
	}
	if len(ks.Warnings) > 0 {
		fmt.Printf("  %d loader warning(s)\n", len(ks.Warnings))
	}
	return nil
}

func jsonMarshal(v any) ([]byte, error)   { return json.Marshal(v) }
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
