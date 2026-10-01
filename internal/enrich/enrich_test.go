package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// cmEdge is a cert-manager-like edge: the ServiceMonitor port change appears
// as a release note, an upgrade-guide statement and a computed Helm default
// diff; the RotationPolicy default change appears in the release notes and
// the upgrade guide; plus one unrelated change.
func cmEdge() *domain.UpgradeEdge {
	const notesURI = "https://github.com/example/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md"
	const guideURI = "https://github.com/example/website/blob/master/content/docs/releases/upgrading/upgrading-1.17-1.18.md"
	evNote := domain.NewEvidence(domain.EvidenceDocument, "release-notes", notesURI, "L337-L337",
		"- Switched `service/servicemonitor` definitions to use port names instead of numbers. (#7727)", "sha256:n", t0)
	evGuide := domain.NewEvidence(domain.EvidenceDocument, "upgrade-guide", guideURI, "L20-L24",
		"3. The ServiceMonitor now targets the named port `http-metrics`. If you override `prometheus.servicemonitor.targetPort`, use the port name.", "sha256:g", t0)
	evFrom := domain.NewEvidence(domain.EvidenceStructured, "helm-chart", "https://github.com/example/cm/blob/v1.17.0/deploy/charts/cm/values.yaml",
		"deploy/charts/cm/values.yaml", "# +docs:section=Global", "sha256:v17", t0)
	evTo := domain.NewEvidence(domain.EvidenceStructured, "helm-chart", "https://github.com/example/cm/blob/v1.18.0/deploy/charts/cm/values.yaml",
		"deploy/charts/cm/values.yaml", "# +docs:section=Global", "sha256:v18", t0)
	evRotNote := domain.NewEvidence(domain.EvidenceDocument, "release-notes", notesURI, "L338-L338",
		"- The default value of `Certificate.Spec.PrivateKey.RotationPolicy` changed from `Never` to `Always`. (#7723)", "sha256:n", t0)
	evRotGuide := domain.NewEvidence(domain.EvidenceDocument, "upgrade-guide", guideURI, "L8-L10",
		"1. We have changed the default value of `Certificate.Spec.PrivateKey.RotationPolicy` from `Never` to `Always`.", "sha256:g", t0)
	evOther := domain.NewEvidence(domain.EvidenceDocument, "release-notes", notesURI, "L350-L350",
		"- Fix AWS Route53 error detection for not-found errors during deletion of DNS records", "sha256:n", t0)

	decl := func(rule string) domain.Provenance {
		return domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.notes@v1", Rule: rule, Confidence: domain.ConfidenceHigh}
	}
	return &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "cert-manager", Name: "cert-manager"},
		From:          domain.MustVersion("v1.17.0", "1.17.0"),
		To:            domain.MustVersion("v1.18.0", "1.18.0"),
		PathPolicy:    "minor-lineage",
		Path:          []domain.PathStep{{Version: domain.MustVersion("v1.18.0", "1.18.0"), Reason: "minor-release"}},
		Sources: []domain.SourceStatus{
			{SourceID: "release-notes", Kind: "repo-file", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, State: domain.SourceOK},
			{SourceID: "upgrade-guide", Kind: "repo-file", Roles: []domain.SourceRole{domain.RoleUpgradeGuide}, State: domain.SourceOK},
			{SourceID: "helm-chart", Kind: "repo-file", State: domain.SourceOK},
		},
		Changes: []domain.Change{
			{ID: "chg-rot-guide", Category: domain.CategoryMigration, Breaking: true, Release: "1.18.0",
				Title:      "We have changed the default value of `Certificate.Spec.PrivateKey.RotationPolicy` from `Never` to `Always`.",
				Provenance: decl("section:/upgrad/i"), Evidence: []domain.EvidenceID{evRotGuide.ID}},
			{ID: "chg-port-guide", Category: domain.CategoryMigration, ActionRequired: true, Release: "1.18.0",
				Title:      "The ServiceMonitor now targets the named port `http-metrics`. If you override `prometheus.servicemonitor.targetPort`, use the port name.",
				Provenance: decl("section:/upgrad/i"), Evidence: []domain.EvidenceID{evGuide.ID}},
			{ID: "chg-port-diff", Category: domain.CategoryHelmValues,
				Title:      "Default of Helm value `prometheus.servicemonitor.targetPort` changed: 9402 → \"http-metrics\"",
				Detail:     "9402 → \"http-metrics\"",
				Subjects:   []string{"prometheus.servicemonitor.targetPort"},
				Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: "values:default-changed", Confidence: domain.ConfidenceHigh},
				Evidence:   []domain.EvidenceID{evFrom.ID, evTo.ID}},
			{ID: "chg-port-note", Category: domain.CategoryFeature, Release: "1.18.0",
				Title:      "Switched `service/servicemonitor` definitions to use port names instead of numbers",
				Provenance: decl("section:/feature/i"), Evidence: []domain.EvidenceID{evNote.ID}},
			{ID: "chg-rot-note", Category: domain.CategoryFeature, Release: "1.18.0",
				Title:      "The default value of `Certificate.Spec.PrivateKey.RotationPolicy` changed from `Never` to `Always`",
				Provenance: decl("section:/feature/i"), Evidence: []domain.EvidenceID{evRotNote.ID}},
			{ID: "chg-route53", Category: domain.CategoryBugfix, Release: "1.18.0",
				Title:      "Fix AWS Route53 error detection for not-found errors during deletion of DNS records",
				Provenance: decl("section:/bug/i"), Evidence: []domain.EvidenceID{evOther.ID}},
		},
		Evidence:    []domain.Evidence{evRotGuide, evGuide, evFrom, evTo, evNote, evRotNote, evOther},
		GeneratedAt: t0,
	}
}

var (
	promptChangeRe   = regexp.MustCompile(`(?m)^\[(chg-[^\]]+)\]`)
	promptEvidenceRe = regexp.MustCompile(`(?m)^\[(ev-[^\]]+)\]`)
)

func promptIDs(req llm.Request) (changes, evidence []string) {
	u := req.Messages[0].Content
	for _, m := range promptChangeRe.FindAllStringSubmatch(u, -1) {
		changes = append(changes, m[1])
	}
	for _, m := range promptEvidenceRe.FindAllStringSubmatch(u, -1) {
		evidence = append(evidence, m[1])
	}
	return changes, evidence
}

func evidenceOf(e *domain.UpgradeEdge, changeID string) []string {
	for _, c := range e.Changes {
		if c.ID == changeID {
			var out []string
			for _, x := range c.Evidence {
				out = append(out, string(x))
			}
			return out
		}
	}
	return nil
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// clusterFake answers every group whose prompt shows the given changes with
// one cluster citing the given evidence, and nothing otherwise.
func clusterFake(e *domain.UpgradeEdge) *llm.Fake {
	return &llm.Fake{Model: "fake-model", ModelVersion: "fake-model-v1", Respond: func(req llm.Request) (string, error) {
		changes, _ := promptIDs(req)
		set := toSet(changes...)
		var out []proposal
		if set["chg-port-diff"] {
			out = append(out, proposal{Kind: "cluster", Title: "ServiceMonitor targets the named metrics port",
				Content:   "The release notes, the upgrade guide and the chart default all state that the ServiceMonitor now targets the named port http-metrics instead of 9402.",
				Changes:   []string{"chg-port-note", "chg-port-guide", "chg-port-diff"},
				Citations: []string{evidenceOf(e, "chg-port-note")[0], evidenceOf(e, "chg-port-guide")[0], evidenceOf(e, "chg-port-diff")[1]}, Confidence: "high"})
		}
		if set["chg-rot-guide"] {
			out = append(out, proposal{Kind: "cluster", Title: "Private key rotation defaults to Always",
				Content:   "Certificate.Spec.PrivateKey.RotationPolicy now defaults to Always instead of Never.",
				Changes:   []string{"chg-rot-note", "chg-rot-guide"},
				Citations: []string{evidenceOf(e, "chg-rot-guide")[0], evidenceOf(e, "chg-rot-note")[0]}, Confidence: "medium"},
				proposal{Kind: "migration-summary", Title: "Set rotationPolicy explicitly to keep the old behaviour",
					Content:   "Certificates that rely on the old default now rotate their private key on renewal.",
					Changes:   []string{"chg-rot-guide"},
					Citations: []string{evidenceOf(e, "chg-rot-guide")[0]}, Confidence: "medium"})
		}
		b, _ := json.Marshal(answer{Enrichments: out})
		return string(b), nil
	}}
}

func TestClusterOfReleaseNoteUpgradeGuideAndHelmDiff(t *testing.T) {
	e := cmEdge()
	before := mustJSON(t, struct {
		C []domain.Change
		F []domain.Fact
		E []domain.Evidence
	}{e.Changes, e.Facts, e.Evidence})
	fake := clusterFake(e)
	res, err := Run(context.Background(), e, Options{Client: fake, Clock: func() time.Time { return t0 }})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(e, res); err != nil {
		t.Fatal(err)
	}
	if after := mustJSON(t, struct {
		C []domain.Change
		F []domain.Fact
		E []domain.Evidence
	}{e.Changes, e.Facts, e.Evidence}); after != before {
		t.Fatal("enrichment must not modify changes, facts or evidence")
	}
	if len(res.Run.Rejected) != 0 {
		t.Fatalf("unexpected rejections: %+v", res.Run.Rejected)
	}

	var port *domain.Enrichment
	for i := range e.Enrichments {
		if e.Enrichments[i].Kind == domain.EnrichmentCluster && strings.Contains(e.Enrichments[i].Title, "metrics port") {
			port = &e.Enrichments[i]
		}
	}
	if port == nil {
		t.Fatalf("no ServiceMonitor cluster: %+v", e.Enrichments)
	}
	if want := []string{"chg-port-guide", "chg-port-diff", "chg-port-note"}; !reflect.DeepEqual(port.RelatesTo, want) {
		t.Errorf("relatesTo %v, want %v (edge order)", port.RelatesTo, want)
	}
	if len(port.Citations) != 3 {
		t.Fatalf("the cluster must cite three evidence records: %v", port.Citations)
	}
	p := port.Provenance
	if p.Method != domain.MethodAI || p.Model != "fake-model" || p.ModelVersion != "fake-model-v1" || p.PromptVersion != PromptVersion ||
		p.Producer != Producer || p.GeneratedAt == nil || !p.GeneratedAt.Equal(t0) || p.Confidence != domain.ConfidenceMedium {
		t.Errorf("provenance: %+v", p)
	}
	// The digest is the digest of the exact request the model was given, and
	// the input evidence is what that request showed.
	var asked *llm.Request
	for _, r := range fake.Requests() {
		if llm.PromptDigest(r) == p.PromptDigest {
			r := r
			asked = &r
		}
	}
	if asked == nil {
		t.Fatal("promptDigest matches no request")
	}
	_, shown := promptIDs(*asked)
	var input []string
	for _, id := range p.InputEvidence {
		input = append(input, string(id))
	}
	if !reflect.DeepEqual(input, shown) || len(input) != 4 {
		t.Errorf("input evidence %v, shown %v", input, shown)
	}
	if len(asked.JSONSchema) == 0 || len(asked.Messages[0].Content) > maxPromptLength {
		t.Error("prompts are bounded and ask for structured output")
	}

	// Duplicate metric: 5 changes in 2 clusters → 3 duplicates consolidated.
	r := e.EnrichmentRun
	if r == nil || r.Clusters != 2 || r.ClusteredChanges != 5 || r.DuplicatesConsolidated != 3 || r.Accepted != 3 || r.Requests != res.Run.CandidateGroups {
		t.Errorf("run metadata: %+v", r)
	}
	for _, en := range e.Enrichments {
		for _, c := range e.Changes {
			if c.ID == en.ID {
				t.Fatal("an enrichment appears as a change")
			}
		}
	}
	validateSchema(t, e)
}

func validateSchema(t *testing.T, e *domain.UpgradeEdge) {
	t.Helper()
	c := jsonschema.NewCompiler()
	sch, err := c.Compile(filepath.Join("..", "..", "schemas", "upgrade-edge.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(mustJSON(t, e)))
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Fatalf("the enriched edge does not match the published schema: %v", err)
	}
}

func TestValidatorRejectsUngroundedProposals(t *testing.T) {
	e := cmEdge()
	guideEv := evidenceOf(e, "chg-port-guide")[0]
	noteEv := evidenceOf(e, "chg-port-note")[0]
	diffEv := evidenceOf(e, "chg-port-diff")[1]
	rotEv := evidenceOf(e, "chg-rot-guide")[0]
	fake := &llm.Fake{Model: "fake-model", Respond: func(req llm.Request) (string, error) {
		changes, _ := promptIDs(req)
		if !toSet(changes...)["chg-port-diff"] {
			return `{"enrichments":[]}`, nil
		}
		out := []proposal{
			{Kind: "cluster", Title: "x", Content: "Same change.", Changes: []string{"chg-port-guide", "chg-hallucinated"}, Citations: []string{guideEv}, Confidence: "high"},
			{Kind: "cluster", Title: "x", Content: "Same change.", Changes: []string{"chg-port-guide", "chg-route53"}, Citations: []string{guideEv}, Confidence: "high"},
			{Kind: "cluster", Title: "x", Content: "Same change.", Changes: []string{"chg-port-guide", "chg-port-note"}, Citations: []string{guideEv, rotEv}, Confidence: "high"},
			{Kind: "cluster", Title: "x", Content: "Same change.", Changes: []string{"chg-port-guide", "chg-port-note"}, Citations: []string{guideEv, "ev-made-up"}, Confidence: "high"},
			{Kind: "migration-summary", Title: "x", Content: "  ", Changes: []string{"chg-port-guide"}, Citations: []string{guideEv}, Confidence: "high"},
			{Kind: "cluster", Title: "x", Content: "Same change.", Changes: []string{"chg-port-guide"}, Citations: []string{guideEv}, Confidence: "high"},
			{Kind: "cluster", Title: "x", Content: "Same change.", Changes: []string{"chg-port-guide", "chg-port-note"}, Citations: []string{guideEv}, Confidence: "high"},
			{Kind: "diff-explanation", Title: "x", Content: "The diff matters.", Changes: []string{"chg-port-diff"}, Citations: []string{diffEv}, Confidence: "high"},
			{Kind: "diff-explanation", Title: "x", Content: "The diff matters.", Changes: []string{"chg-port-note"}, Citations: []string{noteEv}, Confidence: "high"},
			{Kind: "migration-summary", Title: "x", Content: "Nothing cited.", Changes: []string{"chg-port-guide"}, Citations: []string{}, Confidence: "high"},
			// accepted:
			{Kind: "diff-explanation", Title: "Why the targetPort default changed", Content: "The chart now targets the named port, as the release note says.",
				Changes: []string{"chg-port-diff", "chg-port-note"}, Citations: []string{diffEv, noteEv}, Confidence: "high"},
			{Kind: "related", Title: "Possibly related", Content: "These may concern the same ServiceMonitor change.",
				Changes: []string{"chg-port-note", "chg-port-guide"}, Citations: []string{noteEv}, Confidence: "high"},
			{Kind: "related", Title: "Possibly related", Content: "Again.",
				Changes: []string{"chg-port-guide", "chg-port-note"}, Citations: []string{noteEv}, Confidence: "low"},
		}
		b, _ := json.Marshal(answer{Enrichments: out})
		return string(b), nil
	}}
	res, err := Run(context.Background(), e, Options{Client: fake, Clock: func() time.Time { return t0 }})
	if err != nil {
		t.Fatal(err)
	}
	wantReasons := []string{
		"unknown change id chg-hallucinated",
		"change chg-route53 was not part of this group's input",
		"citation " + rotEv + " was not part of the input evidence",
		"citation ev-made-up was not part of the input evidence",
		"empty content",
		"kind cluster needs at least 2 changes",
		"cluster member chg-port-note has none of its evidence cited",
		"must cite the release-note or upgrade-guide statement",
		"must relate to a computed change",
		"cites no evidence",
		"duplicate of an accepted enrichment",
	}
	if len(res.Run.Rejected) != len(wantReasons) {
		t.Fatalf("rejections: %+v", res.Run.Rejected)
	}
	for i, want := range wantReasons {
		if got := res.Run.Rejected[i].Reason; !strings.Contains(got, want) {
			t.Errorf("rejection %d: %q, want %q", i, got, want)
		}
		if res.Run.Rejected[i].PromptDigest == "" || res.Run.Rejected[i].Group == "" {
			t.Errorf("rejection %d lacks its group/prompt: %+v", i, res.Run.Rejected[i])
		}
	}
	if len(res.Enrichments) != 2 {
		t.Fatalf("accepted: %+v", res.Enrichments)
	}
	rel := res.Enrichments[1]
	if rel.Kind != domain.EnrichmentRelated || !rel.Unverified || rel.Provenance.Confidence != domain.ConfidenceLow {
		t.Errorf("related enrichments are unverified and low confidence: %+v", rel)
	}
	if res.Enrichments[0].Provenance.Confidence != domain.ConfidenceMedium {
		t.Error("AI output is never high confidence")
	}
	if err := Apply(e, res); err != nil {
		t.Fatal(err)
	}
	validateSchema(t, e)
}

func TestWholeAnswersRejected(t *testing.T) {
	for name, text := range map[string]string{
		"not json":      "Sure! Here are the clusters.",
		"wrong schema":  `{"enrichments":[{"kind":"cluster","content":"x"}]}`,
		"unknown kind":  `{"enrichments":[{"kind":"summary","title":"","content":"x","changes":[],"citations":[],"confidence":"low"}]}`,
		"extra members": `{"enrichments":[],"note":"x"}`,
		"unbounded content": `{"enrichments":[{"kind":"related","title":"t","content":"` + strings.Repeat("x", 2001) +
			`","changes":["chg-port-note","chg-port-guide"],"citations":[],"confidence":"low"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := cmEdge()
			res, err := Run(context.Background(), e, Options{Client: &llm.Fake{Respond: func(llm.Request) (string, error) { return text, nil }}})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Enrichments) != 0 || len(res.Run.Rejected) != res.Run.Requests || res.Run.Requests == 0 {
				t.Fatalf("every answer must be rejected: %+v", res.Run)
			}
			for _, r := range res.Requests {
				if r.Status != StatusRejected {
					t.Fatalf("request status %+v", r)
				}
			}
		})
	}
}

func TestWithoutModelCandidatesAreNotConclusions(t *testing.T) {
	e := cmEdge()
	res, err := Run(context.Background(), e, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) == 0 {
		t.Fatal("expected candidate groups")
	}
	if len(res.Enrichments) != 0 || res.Run.Requests != 0 {
		t.Fatalf("no model, no enrichment: %+v", res.Run)
	}
	if err := Apply(e, res); err != nil {
		t.Fatal(err)
	}
	if len(e.Enrichments) != 0 || e.EnrichmentRun.CandidateGroups != len(res.Candidates) {
		t.Fatalf("edge: %+v %+v", e.Enrichments, e.EnrichmentRun)
	}
}

func TestCacheReplayIsByteForByte(t *testing.T) {
	dir := t.TempDir()
	e := cmEdge()
	fake := clusterFake(e)
	clock := func() time.Time { return t0.Add(90 * time.Minute) }
	online, err := Run(context.Background(), e, Options{Client: &llm.Cache{Dir: dir, Inner: fake, Clock: clock}, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	asked := len(fake.Requests())
	// Offline: no inner client and a different clock; the answers, models and
	// generation times all come from the cache.
	offline, err := Run(context.Background(), cmEdge(), Options{Client: &llm.Cache{Dir: dir}, Clock: func() time.Time { return t0.AddDate(1, 0, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Requests()) != asked {
		t.Fatal("the offline run must not ask the model")
	}
	a, b := mustJSON(t, online.Enrichments), mustJSON(t, offline.Enrichments)
	if a != b || len(online.Enrichments) == 0 {
		t.Fatalf("offline replay differs:\n%s\n%s", a, b)
	}
	for _, r := range offline.Requests {
		if r.Origin != llm.OriginCache {
			t.Fatalf("offline answer from %q", r.Origin)
		}
	}
	// An empty cache offline leaves every request pending, without failing.
	empty, err := Run(context.Background(), cmEdge(), Options{Client: &llm.Cache{Dir: t.TempDir()}})
	if err != nil || empty.Run.Pending != empty.Run.Requests || len(empty.Enrichments) != 0 {
		t.Fatalf("empty cache: %+v %v", empty.Run, err)
	}
}

func TestExchangeRoundTrip(t *testing.T) {
	exDir, cacheDir := t.TempDir(), t.TempDir()
	e := cmEdge()
	client := &llm.Cache{Dir: cacheDir, Inner: &llm.Exchange{Dir: exDir}}

	first, err := Run(context.Background(), e, Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if first.Run.Pending == 0 || first.Run.Pending != first.Run.Requests || len(first.Enrichments) != 0 {
		t.Fatalf("first run must leave every request pending: %+v", first.Run)
	}
	reqs, _ := filepath.Glob(filepath.Join(exDir, "*.request.json"))
	if len(reqs) != first.Run.Requests {
		t.Fatalf("%d request files for %d requests", len(reqs), first.Run.Requests)
	}

	// Answer the requests "with another model" through the files; one answer
	// carries the digest of a different request and must be rejected.
	answerer := clusterFake(e)
	var mismatched string
	for i, path := range reqs {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var xr llm.ExchangeRequest
		if err := json.Unmarshal(b, &xr); err != nil {
			t.Fatal(err)
		}
		text, _ := answerer.Respond(xr.Request)
		resp := llm.ExchangeResponse{Format: llm.ExchangeResponseFormat, PromptDigest: xr.PromptDigest, Model: "batch-model",
			ModelVersion: "batch-model-2026-09", GeneratedAt: t0.Add(time.Hour), Output: json.RawMessage(text)}
		if i == 0 {
			resp.PromptDigest = "sha256:" + strings.Repeat("0", 64)
			mismatched = xr.PromptDigest
		}
		out, _ := json.Marshal(resp)
		if err := os.WriteFile(strings.TrimSuffix(path, ".request.json")+".response.json", out, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	second, err := Run(context.Background(), cmEdge(), Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	rejected := 0
	for _, r := range second.Requests {
		if r.PromptDigest == mismatched {
			if r.Status != StatusRejected || !strings.Contains(r.Detail, "does not match its request") {
				t.Fatalf("mismatched response must be rejected: %+v", r)
			}
			rejected++
		} else if r.Status != StatusAnswered || r.Origin != llm.OriginExchange {
			t.Fatalf("request %+v", r)
		}
	}
	if rejected != 1 || len(second.Run.Rejected) != 1 || second.Run.Rejected[0].PromptDigest != mismatched ||
		!strings.Contains(second.Run.Rejected[0].Reason, "does not match its request") {
		t.Fatalf("the mismatch must be recorded as a rejection: %+v", second.Run)
	}
	for _, en := range second.Enrichments {
		if en.Provenance.Model != "batch-model" || en.Provenance.ModelVersion != "batch-model-2026-09" || !en.Provenance.GeneratedAt.Equal(t0.Add(time.Hour)) {
			t.Fatalf("the recorded model is the one named in the response file: %+v", en.Provenance)
		}
	}

	// Accepted answers were cached: they replay offline without the exchange.
	if err := os.RemoveAll(exDir); err != nil {
		t.Fatal(err)
	}
	third, err := Run(context.Background(), cmEdge(), Options{Client: &llm.Cache{Dir: cacheDir}})
	if err != nil {
		t.Fatal(err)
	}
	if mustJSON(t, third.Enrichments) != mustJSON(t, second.Enrichments) || third.Run.Pending != 1 {
		t.Fatalf("offline replay of exchanged answers: %+v", third.Run)
	}
}

func TestRepeatedFailuresStopAsking(t *testing.T) {
	e := cmEdge()
	boom := errors.New("401 invalid key")
	fake := &llm.Fake{Respond: func(llm.Request) (string, error) { return "", boom }}
	res, err := Run(context.Background(), e, Options{Client: fake, MaxConsecutiveFailures: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Requests()) != 1 || res.Run.Failed != 1 {
		t.Fatalf("asked %d times: %+v", len(fake.Requests()), res.Run)
	}
	skipped := 0
	for _, r := range res.Requests {
		if r.Status == StatusSkipped {
			skipped++
		}
	}
	if skipped != len(res.Candidates)-1 {
		t.Fatalf("skipped %d of %d", skipped, len(res.Candidates))
	}
}

func TestPromptIsDeterministic(t *testing.T) {
	e := cmEdge()
	var a, b bytes.Buffer
	for _, buf := range []*bytes.Buffer{&a, &b} {
		fake := &llm.Fake{Respond: func(llm.Request) (string, error) { return `{"enrichments":[]}`, nil }}
		if _, err := Run(context.Background(), cmEdge(), Options{Client: fake}); err != nil {
			t.Fatal(err)
		}
		for _, r := range fake.Requests() {
			buf.WriteString(llm.PromptDigest(r))
		}
	}
	if a.String() != b.String() || a.Len() == 0 {
		t.Fatal("the same edge must yield the same prompts (and digests)")
	}
	_ = e
}
