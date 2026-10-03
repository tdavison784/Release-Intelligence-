package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Regression from the real records (proxy-shadow REPORT.md finding 3): in the
// proxy-2 shadow pass the queue refused two proxy decisions because the
// composition of the candidate's aspects was invalid — migration-required
// (a change only the migration family may carry) on a gvk subject:
//
//	ri-936e84e2656e  external-secrets  consequence   verdict correct
//	ri-a52785036b5e  kyverno           applicability verdict accept
//
// The store the pass ran against is committed under proxy-shadow/knowledge; the
// test replays each verdict against a copy of that product's records.

const shadowStore = "../../docs/phase3/learning-loop/proxy-shadow"

func copyProduct(t *testing.T, product string) Store {
	t.Helper()
	src := filepath.Join(shadowStore, "knowledge", product)
	if _, err := os.Stat(src); err != nil {
		t.Skipf("shadow store not present: %v", err)
	}
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, product, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewFileStore(dst)
}

func proxyDecision(t *testing.T, it domain.ReviewItem, action domain.DecisionAction) domain.ReviewDecision {
	t.Helper()
	at := time.Date(2026, 10, 3, 2, 30, 0, 0, time.UTC)
	orig := it.Proposed
	prov := aiProv("claude-opus-5-5")
	prov.Provider = "anthropic"
	d := domain.ReviewDecision{ReviewItemID: it.ID, Action: action, Original: &orig, Reviewer: "claude-opus-5-5",
		ReviewerKind: domain.ReviewerProxy, ProxyProvenance: &prov, StartedAt: at.Add(-10 * time.Second), DecidedAt: at}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	return d
}

func getItem(t *testing.T, s Store, id string) domain.ReviewItem {
	t.Helper()
	rec, err := s.Get(context.Background(), id)
	if err != nil || rec.ReviewItem == nil {
		t.Fatalf("item %s: %v", id, err)
	}
	return *rec.ReviewItem
}

// assertNoInvalidFact checks that every fact in the store validates (the queue
// never mints one the domain refuses) and that the decision was recorded.
func assertRecorded(t *testing.T, s Store, d domain.ReviewDecision, o DecisionOutcome) {
	t.Helper()
	ctx := context.Background()
	rec, err := s.Get(ctx, d.ID)
	if err != nil || rec.Decision == nil {
		t.Fatalf("the verdict was lost: decision %s not stored (%v)", d.ID, err)
	}
	if o.Fact != nil {
		if err := o.Fact.Validate(); err != nil {
			t.Fatalf("invalid fact minted: %v", err)
		}
	}
	snap, _ := s.Load(ctx, Query{})
	for _, f := range snap.Facts {
		if err := f.Assertion.Validate(true); err != nil {
			t.Fatalf("stored fact %s is invalid: %v", f.ID, err)
		}
	}
	for _, it := range o.FollowUps {
		if err := it.Proposed.Validate(false); err != nil {
			t.Fatalf("follow-up %s proposes an invalid assertion: %v", it.ID, err)
		}
	}
}

func TestRealRecordKyvernoAcceptIsRecordedNotRefused(t *testing.T) {
	s := copyProduct(t, "kyverno")
	it := getItem(t, s, "ri-a52785036b5e")
	d := proxyDecision(t, it, domain.ActionAccept)
	d.Labels = []domain.FeedbackLabel{domain.LabelAccepted}
	out, err := NewQueue(s, nil).Decide(context.Background(), []domain.ReviewDecision{d})
	if err != nil {
		t.Fatalf("the queue refused the real verdict: %v", err)
	}
	assertRecorded(t, s, d, out[0])
}

func TestRealRecordExternalSecretsCorrectionIsRecordedNotRefused(t *testing.T) {
	s := copyProduct(t, "external-secrets")
	it := getItem(t, s, "ri-936e84e2656e")
	var resp struct {
		Output struct {
			Reason     string   `json:"reason"`
			Labels     []string `json:"labels"`
			Correction struct {
				Statement   string `json:"statement"`
				Consequence struct {
					Kind        string `json:"kind"`
					Severity    string `json:"severity"`
					Statement   string `json:"statement"`
					Remediation string `json:"remediation"`
				} `json:"consequence"`
			} `json:"correction"`
		} `json:"output"`
	}
	b, err := os.ReadFile(filepath.Join(shadowStore, "responses", "ri-936e84e2656e.response.json"))
	if err != nil {
		t.Skip(err)
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	d := proxyDecision(t, it, domain.ActionCorrect)
	c := *d.Original
	kind := domain.ConsequenceKind(resp.Output.Correction.Consequence.Kind)
	c.Consequence = &domain.Consequence{Kind: kind, ExposedClass: kind.ExposedClass(), Severity: domain.ImpactSeverity(resp.Output.Correction.Consequence.Severity),
		Statement: resp.Output.Correction.Consequence.Statement, Remediation: resp.Output.Correction.Consequence.Remediation}
	d.Corrected = &c
	for _, l := range resp.Output.Labels {
		d.Labels = append(d.Labels, domain.FeedbackLabel(l))
	}
	d.Labels = append([]domain.FeedbackLabel{domain.LabelCorrected}, d.Labels...)
	d.Reason = resp.Output.Reason
	out, err := NewQueue(s, nil).Decide(context.Background(), []domain.ReviewDecision{d})
	if err != nil {
		t.Fatalf("the queue refused the real verdict: %v", err)
	}
	assertRecorded(t, s, d, out[0])
}
