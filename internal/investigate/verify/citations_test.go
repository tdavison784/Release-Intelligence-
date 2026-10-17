package verify

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Synthetic product: "acme-ctl", a Widget CRD whose spec.retention.mode
// default changes Keep → Purge in 2.0.0. Never an evaluation case.

func str(s string) *string { return &s }

func widgetRule() domain.SemanticAssertion {
	unset := domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget",
		Of: []domain.Condition{{Op: domain.OpField, Path: "spec.retention.mode", State: domain.StateUnset}}}
	keep := domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget",
		Of: []domain.Condition{{Op: domain.OpField, Path: "spec.retention.mode", State: domain.StateEquals, Values: []string{`"Keep"`}}}}
	return domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectCRDField, Product: "acme-ctl", Group: "acme.example", Kind: "Widget", Path: "spec.retention.mode"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"Keep"`), After: str(`"Purge"`)},
		Applicability: &domain.Applicability{Exposure: unset, Overlap: &keep},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
	}
}

var bounds = &domain.ProofBounds{From: "1.9.4", To: "2.0.0"}

type mapResolver map[string]string // content digest → body

func (m mapResolver) Resolve(_ context.Context, ev domain.Evidence) (*Document, error) {
	b, ok := m[ev.ContentDigest]
	if !ok {
		return nil, ErrUnresolved
	}
	return &Document{Digest: ev.ContentDigest, Body: []byte(b), Source: "test"}, nil
}

const notes200 = `# Release 2.0
## Breaking
The default value of spec.retention.mode changed from Keep to Purge.
Set retention.mode: Keep explicitly to keep the old behaviour.`

func cite(uri, quote, body string) domain.Evidence {
	return domain.NewEvidence(domain.EvidenceDocument, "art-1", uri, "", quote, domain.Digest([]byte(body)), time.Time{})
}

func TestCitationCheck(t *testing.T) {
	other := "# Release 2.1\nThe default value of spec.retention.mode changed again."
	res := mapResolver{domain.Digest([]byte(notes200)): notes200, domain.Digest([]byte(other)): other}
	ok := cite("https://example.test/acme/blob/main/docs/release-notes-2.0.md", "The default value of spec.retention.mode changed from Keep to Purge.", notes200)
	line := cite("https://example.test/acme/releases/tag/v2.0.0", "Set retention.mode: Keep explicitly", notes200)
	for _, tc := range []struct {
		name   string
		evs    []domain.Evidence
		bounds *domain.ProofBounds
		want   domain.MethodOutcome
		detail string
	}{
		{"in bounds and stating the subject", []domain.Evidence{ok, line}, bounds, domain.MethodPass, "inside (1.9.4, 2.0.0]"},
		{"a document of a later release", []domain.Evidence{ok, cite("https://example.test/acme/release-notes-2.1.md", "changed again", other)}, bounds, domain.MethodFail, "outside the rule's bounds"},
		{"a document of an earlier line", []domain.Evidence{cite("https://example.test/acme/v1.8.0/notes.md", "changed from Keep to Purge", notes200)}, bounds, domain.MethodFail, "1.8.0, outside"},
		{"unresolvable", []domain.Evidence{ok, cite("https://example.test/acme/v2.0.0/x.md", "x", "not cached")}, bounds, domain.MethodFail, "does not resolve"},
		{"passage not in the document", []domain.Evidence{cite("https://example.test/acme/v2.0.0/notes.md", "Purge is never the default", notes200)}, bounds, domain.MethodFail, "does not occur"},
		{"no version named", []domain.Evidence{cite("https://example.test/acme/blob/main/README.md", "changed from Keep to Purge", notes200)}, bounds, domain.MethodFail, "names no version"},
		{"no bounds", []domain.Evidence{ok}, nil, domain.MethodNotRun, "no release bounds"},
		{"inverted bounds", []domain.Evidence{ok}, &domain.ProofBounds{From: "2.0.0", To: "1.0.0"}, domain.MethodNotRun, "not an increasing"},
		{"no citation", nil, bounds, domain.MethodNotRun, "cites no upstream text"},
		{"passages that never name the subject", []domain.Evidence{cite("https://example.test/acme/v2.0.0/notes.md", "# Release 2.0", notes200)}, bounds, domain.MethodInconclusive, "no cited passage names"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := CheckCitations(context.Background(), res, widgetRule(), tc.bounds, tc.evs)
			if m.Outcome != tc.want || !strings.Contains(m.Detail+checksText(m), tc.detail) {
				t.Fatalf("%s: %s (%s)", m.Outcome, m.Detail, checksText(m))
			}
			if m.Level != domain.LevelDeterministic {
				t.Fatalf("level %s", m.Level)
			}
			if (m.Outcome == domain.MethodPass) != (len(m.Evidence) > 0) {
				t.Fatalf("evidence on a %s method: %d", m.Outcome, len(m.Evidence))
			}
		})
	}
}

func checksText(m domain.ProofMethod) string {
	var b strings.Builder
	for _, c := range m.Checks {
		b.WriteString(" | " + c.Detail)
	}
	return b.String()
}

func TestVersionsNamedAndBounds(t *testing.T) {
	got := VersionsNamed("https://x.test/docs/upgrading-1.17-1.18.md", "https://x.test/releases/download/v1.18.0/crds.yaml", "https://x.test/apis/v1/types.go")
	if strings.Join(got, ",") != "1.17,1.18,1.18.0" {
		t.Fatalf("VersionsNamed = %v", got)
	}
	from, to := mustSemver(t, "1.17.2"), mustSemver(t, "1.18.0")
	for tok, want := range map[string]bool{"1.17": true, "1.18": true, "1.18.0": true, "1.17.2": false, "1.17.3": true, "1.19": false, "1.16": false, "1.18.1": false, "1.18.0-alpha.1": true} {
		if inBounds(tok, from, to) != want {
			t.Errorf("inBounds(%s) = %v", tok, !want)
		}
	}
}

func TestHTTPCacheResolver(t *testing.T) {
	dir := t.TempDir()
	put := func(key, url, body string) {
		var gz bytes.Buffer
		zw := gzip.NewWriter(&gz)
		zw.Write([]byte(body))
		zw.Close()
		host := filepath.Join(dir, "http", "raw.example")
		os.MkdirAll(host, 0o755)
		meta, _ := json.Marshal(map[string]any{"url": url, "status": 200, "digest": domain.Digest([]byte(body)), "method": "GET"})
		os.WriteFile(filepath.Join(host, key+".json"), meta, 0o644)
		os.WriteFile(filepath.Join(host, key+".body.gz"), gz.Bytes(), 0o644)
	}
	put("a", "https://raw.githubusercontent.com/acme/docs/main/notes-2.0.md", notes200)
	put("b", "https://raw.example/tampered", "original")
	// tamper: the body no longer matches the recorded digest
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte("changed"))
	zw.Close()
	os.WriteFile(filepath.Join(dir, "http", "raw.example", "b.body.gz"), gz.Bytes(), 0o644)

	r := &HTTPCacheResolver{Dir: dir}
	ctx := context.Background()
	if d, err := r.Resolve(ctx, domain.Evidence{ContentDigest: domain.Digest([]byte(notes200))}); err != nil || string(d.Body) != notes200 {
		t.Fatalf("by digest: %v", err)
	}
	if _, err := r.Resolve(ctx, domain.Evidence{URI: "https://github.com/acme/docs/blob/main/notes-2.0.md"}); err != nil {
		t.Fatalf("blob URL by its raw form: %v", err)
	}
	if _, err := r.Resolve(ctx, domain.Evidence{URI: "https://github.com/acme/docs/blob/main/notes-2.0.md", ContentDigest: "sha256:other"}); err == nil {
		t.Fatal("a cited digest must match exactly; the URL alone must not resolve it")
	}
	if _, err := r.Resolve(ctx, domain.Evidence{ContentDigest: domain.Digest([]byte("original"))}); err == nil {
		t.Fatal("a tampered cache body resolved")
	}
}

func mustSemver(t *testing.T, v string) *semver.Version {
	t.Helper()
	s, err := semver.NewVersion(v)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
