package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

const promptV11 = "investigate-agent/v11"

// v11 (docs/phase6/decision-intel/briefs/upstream.md, ledger D-0009) is v9
// plus upstream evidence: feature upstream with its bounds, request_evidence
// fulfilling upstream-file / upstream-search, one generic rule (fetch the
// upstream behaviour at the target's tag before concluding; cite it or keep it
// unknown) and the item task's line naming what may be read. Everything else
// is v9's; the default stays v9.
func TestPromptV11(t *testing.T) {
	v9, err := LoadPrompt(promptV9)
	must(t, err)
	v11, err := LoadPrompt(promptV11)
	must(t, err)
	want := v9.Features
	want.Upstream = UpstreamBounds{Fetches: 6, Bytes: 384 << 10, SearchFiles: 1500, SearchMatches: 30, SearchContext: 2, SearchStore: 6, HeadLines: 40}
	if v11.Features != want || v11.Features.ScopedVerdict {
		t.Fatalf("features: v9 %+v, v11 %+v", v9.Features, v11.Features)
	}
	if len(v11.Tools) != len(v9.Tools) {
		t.Fatalf("tools: v9 %d, v11 %d", len(v9.Tools), len(v11.Tools))
	}
	for i, x := range v11.Tools {
		y := v9.Tools[i]
		if x.Name != y.Name {
			t.Fatalf("tool %d: v9 %s, v11 %s", i, y.Name, x.Name)
		}
		same := x.Description == y.Description && bytes.Equal(x.InputSchema, y.InputSchema)
		if x.Name != investigate.ToolRequestEvidence && !same {
			t.Errorf("v11 changes tool %s: only request_evidence gains the upstream kinds", x.Name)
		}
		if x.Name == investigate.ToolRequestEvidence {
			for _, s := range []string{`"upstream-file"`, `"upstream-search"`, `"repo"`, `"ref"`, `"path"`, `"dir"`, `"pattern"`, `"glob"`} {
				if !strings.Contains(string(x.InputSchema), s) {
					t.Errorf("v11 request_evidence schema lacks %s", s)
				}
			}
			if strings.Contains(x.Description, "nothing is fetched during this session") {
				t.Error("v11 request_evidence still says nothing is fetched")
			}
		}
	}
	a, _ := promptFS.ReadFile("prompts/" + promptV9 + "/task.md")
	b, _ := promptFS.ReadFile("prompts/" + promptV11 + "/task.md")
	if !bytes.Equal(a, b) {
		t.Error("v11 task.md differs from v9's")
	}
	if !strings.HasPrefix(string(v11.MetaJSON), strings.TrimSuffix(string(v9.MetaJSON), "\n}\n")+",\n  \"upstream\": {") {
		t.Errorf("v11 meta.json is v9's plus upstream only:\n%s", v11.MetaJSON)
	}
	for _, s := range []string{
		"## Upstream behaviour at the target's tag", "does not state that behaviour exactly", "`upstream-search`", "`upstream-file`",
		"implementation", "documentation", "a test whose cases show accepted and rejected inputs", "cite them verbatim",
		`"behaviour at <tag> unavailable:`, "Remembered\n  behaviour is never evidence",
		"Work through these eight questions", "`trace_consumers` with `changeItem`", "## Provisional versus supported", // v9's rules are kept
	} {
		if !strings.Contains(v11.System, s) {
			t.Errorf("v11 system prompt lacks %q", s)
		}
	}
	sections := func(s string) map[string]string {
		out := map[string]string{}
		for _, part := range strings.Split(s, "\n## ")[1:] {
			title, body, _ := strings.Cut(part, "\n")
			out[title] = body
		}
		return out
	}
	s9, s11 := sections(v9.System), sections(v11.System)
	for title, body := range s9 {
		if title == "Verified, inferred, unknown" {
			continue // its last bullet points at the new section
		}
		if s11[title] != body {
			t.Errorf("v11 changes section %q", title)
		}
	}
	if len(s11) != len(s9)+1 || s11["Upstream behaviour at the target's tag"] == "" {
		t.Errorf("v11 sections: %d (v9 %d + the upstream section)", len(s11), len(s9))
	}
	item, _ := promptFS.ReadFile("prompts/" + promptV11 + "/item.md")
	for _, s := range []string{"{{.Upstream}}", "fetch the implementation, documentation or tests at the target's tag"} {
		if !strings.Contains(string(item), s) {
			t.Errorf("v11 item.md lacks %q", s)
		}
	}
	if DefaultPromptVersion != promptV9 {
		t.Fatalf("default %s: v11 waits for the commander's check", DefaultPromptVersion)
	}
	if v9.Digest("t") == v11.Digest("t") {
		t.Error("v9 and v11 share a prompt digest")
	}
	broken := *v11
	broken.Features.Assessment = false
	if err := broken.checkSurface(); err == nil || !strings.Contains(err.Error(), "upstream needs feature assessment") {
		t.Fatalf("upstream without assessment: %v", err)
	}
	broken = *v11
	broken.Features.Upstream.SearchStore = 0
	if err := broken.checkSurface(); err == nil || !strings.Contains(err.Error(), "upstream bounds must all be set") {
		t.Fatalf("incomplete bounds: %v", err)
	}
}

func TestPromptV11IsGeneric(t *testing.T) {
	assertGenericPrompt(t, promptV11, "nginx", "ledger", "ca.crt", "tls.crt", "tls.key", "trust bundle", "storefront", "catalog",
		"pathType", "ImplementationSpecific", "regex", "CVE", "GHSA")
}

// --- fixtures -------------------------------------------------------------------------

const upstreamRepo = "example/keystore"

// The upstream source of the fixture's product at the target's tag v2.1.0
// (and the current version's tag v2.0.0).
var upstreamSource = map[string]map[string]string{
	"v2.1.0": {
		"internal/validate/name.go":      "package validate\n\nimport \"regexp\"\n\n// validName is the pattern a name must match; it is checked by default.\nvar validName = regexp.MustCompile(`^[a-z0-9-]+$`)\n\nfunc CheckName(s string) bool {\n\treturn validName.MatchString(s)\n}\n",
		"internal/validate/name_test.go": "package validate\n\nfunc TestCheckName(t *testing.T) {\n\tfor in, ok := range map[string]bool{\n\t\t\"billing\":   true,\n\t\t\"Billing_1\": false,\n\t} {\n\t\tif CheckName(in) != ok {\n\t\t\tt.Errorf(\"%s\", in)\n\t\t}\n\t}\n}\n",
		"docs/validation.md":             "# Validation\n\nNames are checked against `^[a-z0-9-]+$` by default.\n",
	},
	"v2.0.0": {
		"internal/validate/name.go": "package validate\n\n// names are not checked before v2.1.0\nfunc CheckName(s string) bool { return true }\n",
	},
}

// fakeUpstream serves upstreamSource from memory and counts fetches.
type fakeUpstream struct {
	src   map[string]map[string]string
	calls int
}

func (f *fakeUpstream) FetchDocument(_ context.Context, loc catalog.Locator) (*sources.Document, error) {
	f.calls++
	c, ok := f.src[loc.Ref][loc.Path]
	if !ok {
		return nil, &fetch.Error{URL: loc.Repository + "@" + loc.Ref + ":" + loc.Path, Status: 404, Err: fetch.ErrNotFound}
	}
	return &sources.Document{Locator: loc, Path: loc.Path, Content: []byte(c), Digest: domain.Digest([]byte(c))}, nil
}

func (f *fakeUpstream) FetchDirectory(_ context.Context, loc catalog.Locator) ([]sources.Document, error) {
	f.calls++
	var out []sources.Document
	for p, c := range f.src[loc.Ref] {
		ok, _ := path.Match(orStar(loc.Glob), path.Base(p))
		if strings.HasPrefix(p, loc.Path+"/") && ok {
			out = append(out, sources.Document{Locator: loc, Path: p, Content: []byte(c), Digest: domain.Digest([]byte(c))})
		}
	}
	if len(out) == 0 {
		return nil, &fetch.Error{URL: loc.Repository, Err: fetch.ErrNotFound, Detail: "no files under " + loc.Path}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func orStar(g string) string {
	if g == "" {
		return "*"
	}
	return g
}

func (f *fakeUpstream) port() Upstream {
	return Upstream{Files: f, Dirs: f, Product: func(id domain.ProductID) ([]string, func(string) string, bool) {
		return []string{upstreamRepo}, nil, id == "keystore"
	}}
}

func v11Config() investigate.AgentConfig {
	cfg := v9Config()
	cfg.PromptVersion = promptV11
	return cfg
}

// v11Run runs the fixture's item under v11 with the scripted steps.
func v11Run(t *testing.T, f *consumerFixture, up *Upstream, steps ...Step) (*Scripted, *investigate.AgentOutcome) {
	t.Helper()
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: steps}}
	opts := []Option{WithDriver(DriverScripted, d), WithClock(fixedNow)}
	if up != nil {
		opts = append(opts, WithUpstream(*up))
	}
	out, err := New(f.st, opts...).Investigate(context.Background(), f.input(v11Config()))
	must(t, err)
	return d, out
}

func (f *consumerFixture) upstreamStep(call string, args map[string]any) Step {
	a := map[string]any{"changeItem": f.item, "reason": "which names the new validation accepts"}
	for k, v := range args {
		a[k] = v
	}
	return Step{Call: call, Tool: investigate.ToolRequestEvidence, Args: a}
}

func fileStep(f *consumerFixture, p string, extra ...string) Step {
	args := map[string]any{"kind": "upstream-file", "repo": upstreamRepo, "path": p}
	for i := 0; i+1 < len(extra); i += 2 {
		args[extra[i]] = extra[i+1]
	}
	return f.upstreamStep("call-9", args)
}

// resultOf decodes the view of the scripted step at index i.
func resultOf(t *testing.T, d *Scripted, i int) (upstreamView, error) {
	t.Helper()
	var v upstreamView
	r := d.Results[i]
	if r.Err != nil {
		return v, r.Err
	}
	must(t, json.Unmarshal(r.Result, &v))
	return v, nil
}

func artifactOf(out *investigate.AgentOutcome, id string) (domain.ArtifactRef, bool) {
	for _, a := range out.Artifacts {
		if a.ID == id {
			return a, true
		}
	}
	return domain.ArtifactRef{}, false
}

// --- behaviour ------------------------------------------------------------------------

// A file at the target's tag becomes an upstream-document run artifact (blob
// URL at the tag, producer evidence-fetch@v1) that a verified statement cites
// verbatim at once; the record validates against the unchanged bundle.
func TestUpstreamFileAtTargetTag(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	manifest := len(f.b.Manifest.Artifacts)
	blob := "https://github.com/example/keystore/blob/v2.1.0/internal/validate/name.go"
	art := domain.ArtifactID(domain.ArtifactUpstreamDocument, "upstream/keystore/v2.1.0/source/example/keystore/internal/validate/name.go",
		domain.Digest([]byte(upstreamSource["v2.1.0"]["internal/validate/name.go"])))
	a := f.assessment()
	a["verified"] = []map[string]any{
		{"statement": "upstream changed the rotation default", "citations": f.upstreamCite()},
		{"statement": "at v2.1.0 a name must match ^[a-z0-9-]+$", "citations": []map[string]any{
			{"artifact": art, "locator": "internal/validate/name.go:6", "quote": "var validName = regexp.MustCompile(`^[a-z0-9-]+$`)"}}},
	}
	d, out := v11Run(t, f, &port, fileStep(f, "internal/validate/name.go"),
		f.upstreamStep("call-10", map[string]any{"kind": "upstream-file", "target": blob}), // the blob URL form: the same file, not fetched again
		f.v9Submit("potentially-affected", a))
	v, err := resultOf(t, d, 0)
	must(t, err)
	if v.Request.Status != domain.RequestFulfilled || v.File == nil || v.File.Artifact != art || v.File.URI != blob || v.File.Lines != 10 ||
		len(v.File.Head) != 10 || v.File.Head[5].Text != "var validName = regexp.MustCompile(`^[a-z0-9-]+$`)" || v.Query.Ref != "v2.1.0" {
		t.Fatalf("file view: %+v %+v", v, v.File)
	}
	if v2, err := resultOf(t, d, 1); err != nil || v2.Request.Status != domain.RequestFulfilled || up.calls != 1 {
		t.Fatalf("blob URL form: %+v %v (fetches %d)", v2, err, up.calls)
	}
	if out.Run.Status != domain.AgentSubmitted {
		t.Fatalf("run %s: %s", out.Run.Status, out.Run.Reason)
	}
	ref, ok := artifactOf(out, art)
	if !ok || ref.Origin != domain.OriginUpstream || ref.Kind != domain.ArtifactUpstreamDocument || ref.URI != blob || ref.Producer != UpstreamProducer {
		t.Fatalf("artifact %+v (found %v)", ref, ok)
	}
	if len(out.EvidenceRequests) != 2 {
		t.Fatalf("requests %+v", out.EvidenceRequests)
	}
	for _, r := range out.EvidenceRequests {
		if r.Kind != domain.RequestUpstreamFile || r.Status != domain.RequestFulfilled || len(r.Fulfilled) != 1 || r.Fulfilled[0] != art ||
			r.Target != blob || r.Upstream == nil || r.Upstream.Repository != upstreamRepo || r.Upstream.Ref != "v2.1.0" || r.ChangeItem != f.item {
			t.Errorf("request %+v %+v", r, r.Upstream)
		}
	}
	if len(out.Assessments) != 1 || len(out.Assessments[0].Verified) != 2 || len(out.Assessments[0].UnavailableEvidence) != 0 {
		t.Fatalf("assessment %+v", out.Assessments)
	}
	cited := false
	for _, e := range out.Evidence {
		if e.SourceID == art {
			cited = e.Kind == domain.EvidenceDocument && e.URI == blob
		}
	}
	if !cited {
		t.Fatal("the verified statement's citation did not become document evidence at the blob URL")
	}
	if len(f.b.Manifest.Artifacts) != manifest {
		t.Fatal("the bundle changed")
	}
	inv := widgetLikeRecord(t, f, out)
	must(t, inv.Validate())
	must(t, inv.ValidateAgainst(f.b))
	inv.EvidenceRequests[0].Fulfilled = []string{"art-000000000000"}
	if err := inv.ValidateAgainst(f.b); err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("a fulfilled request naming no run artifact: %v", err)
	}
}

// A search matches line by line over a directory at the tag, returns the
// matching lines with context, stores each matching file (readable with
// read_artifact), and caps matches and stored files.
func TestUpstreamSearch(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	d, out := v11Run(t, f, &port, f.upstreamStep("call-9", map[string]any{"kind": "upstream-search", "repo": "github.com/example/keystore",
		"dir": "internal/validate", "pattern": `CheckName\(|MustCompile`, "glob": "*.go"}),
		f.v9Submit("potentially-affected", f.assessment()))
	v, err := resultOf(t, d, 0)
	must(t, err)
	s := v.Search
	if v.Request.Status != domain.RequestFulfilled || s == nil || s.FilesListed != 2 || s.FilesScanned != 2 || len(s.Matches) != 4 || s.NotStored != "" {
		t.Fatalf("search view: %+v %+v", v, s)
	}
	m := s.Matches[0]
	if m.Path != "internal/validate/name.go" || m.Line != 6 || m.Artifact == "" || m.URI != "https://github.com/example/keystore/blob/v2.1.0/internal/validate/name.go" ||
		len(m.Context) != 4 || m.Context[0].Line != 4 {
		t.Fatalf("first match %+v", m)
	}
	if !strings.Contains(out.EvidenceRequests[0].Target, "/tree/v2.1.0/internal/validate?pattern=") || len(out.EvidenceRequests[0].Fulfilled) != 2 {
		t.Fatalf("request %+v", out.EvidenceRequests[0])
	}

	// Caps: 4 matching lines in each of 10 files → 30 matches from 8 files,
	// 6 of them stored.
	many := map[string]string{}
	for i := 0; i < 10; i++ {
		many[fmt.Sprintf("pkg/f%d.go", i)] = strings.Repeat("hit\n", 4)
	}
	f2 := newConsumerFixture(t)
	up2 := &fakeUpstream{src: map[string]map[string]string{"v2.1.0": many}}
	port2 := up2.port()
	d2, out2 := v11Run(t, f2, &port2, f2.upstreamStep("call-9", map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "pkg", "pattern": "hit"}),
		f2.v9Submit("potentially-affected", f2.assessment()))
	v2, err := resultOf(t, d2, 0)
	must(t, err)
	s2 := v2.Search
	if len(s2.Matches) != 30 || !s2.MoreMatches || len(out2.EvidenceRequests[0].Fulfilled) != 6 || !strings.Contains(s2.NotStored, "2 matching files") ||
		!strings.Contains(s2.NotStored, "pkg/f6.go, pkg/f7.go") || s2.Matches[29].Artifact != "" || s2.Matches[0].Artifact == "" {
		t.Fatalf("caps: %d matches, more %v, stored %v, not stored %q", len(s2.Matches), s2.MoreMatches, out2.EvidenceRequests[0].Fulfilled, s2.NotStored)
	}
	// A stored match is readable like any artifact.
	f3 := newConsumerFixture(t)
	up3 := &fakeUpstream{src: upstreamSource}
	port3 := up3.port()
	art := m.Artifact
	d3, _ := v11Run(t, f3, &port3, f3.upstreamStep("call-9", map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "internal/validate", "pattern": "MustCompile"}),
		Step{Call: "call-9", Tool: investigate.ToolReadArtifact, Args: map[string]any{"artifact": art, "fromLine": 5, "toLine": 6}},
		f3.v9Submit("potentially-affected", f3.assessment()))
	if r := d3.Results[1]; r.Err != nil || !strings.Contains(string(r.Result), "regexp.MustCompile") {
		t.Fatalf("read_artifact of the fetched file: %s %v", r.Result, r.Err)
	}
}

// Another repository, a mutable ref or a tag outside the upgrade is refused
// without a fetch and recorded unavailable with the reason; the current
// version's tag is allowed for comparison. A refused request whose question a
// later request answered is not a gap; the others are, in unknown.
func TestUpstreamRefusals(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	d, out := v11Run(t, f, &port,
		fileStep(f, "internal/validate/name.go", "repo", "someone/else"),
		fileStep(f, "internal/validate/name.go", "ref", "main"),
		fileStep(f, "internal/validate/name.go", "ref", "v1.0.0"),
		fileStep(f, "internal/validate/name.go", "ref", "0123456789abcdef0123456789abcdef01234567"),
		fileStep(f, "internal/validate/name.go", "ref", "v2.0.0"),
		f.v9Submit("potentially-affected", f.assessment()))
	for i, want := range []string{
		"refused: someone/else is not one of the product's own code repositories (example/keystore)",
		`refused: ref "main" is not an immutable ref`,
		"refused: ref v1.0.0 is neither the target's tag v2.1.0 nor the current version's tag v2.0.0",
		"refused: ref 0123456789abcdef0123456789abcdef01234567 is neither",
	} {
		v, err := resultOf(t, d, i)
		must(t, err)
		if v.Request.Status != domain.RequestUnavailable || !strings.HasPrefix(v.Request.Resolution, want) || !strings.Contains(v.Gap, "unavailable: refused:") {
			t.Errorf("step %d: %+v, want %q", i, v.Request, want)
		}
	}
	if v, err := resultOf(t, d, 4); err != nil || v.Request.Status != domain.RequestFulfilled || v.Query.Ref != "v2.0.0" {
		t.Fatalf("the current version's tag: %+v %v", v, err)
	}
	if up.calls != 1 {
		t.Fatalf("refusals fetched: %d fetches", up.calls)
	}
	// The v2.0.0 read answered the same file of the same repository, so the
	// refused refs are not gaps; the foreign repository's refusal is.
	a := out.Assessments[0]
	if len(a.UnavailableEvidence) != 1 || a.UnavailableEvidence[0] != out.EvidenceRequests[0].ID ||
		!strings.Contains(strings.Join(a.Unknown, "\n"), "behaviour at v2.1.0 unavailable: refused: someone/else is not one of") {
		t.Fatalf("gaps %v: %v", a.UnavailableEvidence, a.Unknown)
	}

	// Unanswered refusals are gaps, with the tag and the reason.
	f2 := newConsumerFixture(t)
	_, out2 := v11Run(t, f2, &port, fileStep(f2, "docs/other.md", "ref", "main"), f2.v9Submit("potentially-affected", f2.assessment()))
	a2 := out2.Assessments[0]
	if len(a2.UnavailableEvidence) != 1 || a2.UnavailableEvidence[0] != out2.EvidenceRequests[0].ID ||
		!strings.Contains(strings.Join(a2.Unknown, "\n"), "investigation gap (recorded by the tool layer): behaviour at main unavailable: refused: ref \"main\"") {
		t.Fatalf("gap: %+v %v", a2.UnavailableEvidence, a2.Unknown)
	}

	// Without a fetch port every upstream request is unavailable, never silent.
	f3 := newConsumerFixture(t)
	d3, out3 := v11Run(t, f3, nil, fileStep(f3, "internal/validate/name.go"), f3.v9Submit("potentially-affected", f3.assessment()))
	if v, _ := resultOf(t, d3, 0); v.Request.Status != domain.RequestUnavailable || !strings.Contains(v.Request.Resolution, "no upstream fetcher") ||
		len(out3.Assessments[0].UnavailableEvidence) != 1 {
		t.Fatalf("no port: %+v %+v", v.Request, out3.Assessments[0])
	}
}

// Malformed requests reject the call (nothing is recorded); the model fixes
// them.
func TestUpstreamMalformedRequestsAreRejected(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	cases := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"kind": "upstream-file", "repo": upstreamRepo}, "upstream-file needs repo and path"},
		{map[string]any{"kind": "upstream-file", "repo": upstreamRepo, "path": "a/../b"}, "without . or .. segments"},
		{map[string]any{"kind": "upstream-file", "repo": upstreamRepo, "path": "x", "dir": "y"}, "dir, pattern and glob are for upstream-search"},
		{map[string]any{"kind": "upstream-file", "target": "https://github.com/example/keystore/tree/v2.1.0/x"}, "want a blob URL"},
		{map[string]any{"kind": "upstream-file", "target": "https://github.com/example/keystore/blob/v2.1.0/x", "path": "y"}, "names another file"},
		{map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "/", "pattern": "x"}, "the repository root is not searched"},
		{map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "internal", "pattern": "("}, "pattern:"},
		{map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "internal"}, "needs repo, dir and pattern"},
		{map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "internal", "pattern": "x", "target": "t"}, "leave target and path empty"},
		{map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "internal", "pattern": "x", "glob": "a/b"}, "pattern on base names"},
		{map[string]any{"kind": "upstream-file", "repo": "-x", "path": "y"}, "repo:"},
		{map[string]any{"kind": "artifact", "target": "https://example.dev/x", "repo": upstreamRepo}, "are for kinds upstream-file and upstream-search"},
		{map[string]any{"kind": "upstream-file", "repo": upstreamRepo, "path": "x", "reason": " "}, "reason is required"},
	}
	var steps []Step
	for _, c := range cases {
		steps = append(steps, f.upstreamStep("call-9", c.args))
	}
	steps = append(steps, f.v9Submit("potentially-affected", f.assessment()))
	d, out := v11Run(t, f, &port, steps...)
	for i, c := range cases {
		if err := d.Results[i].Err; err == nil || !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("case %d %v: %v, want %q", i, c.args, err, c.want)
		}
	}
	if len(out.EvidenceRequests) != 0 || up.calls != 0 {
		t.Fatalf("rejected calls recorded %d requests, %d fetches", len(out.EvidenceRequests), up.calls)
	}
}

// The item's fetch count and stored bytes are bounded; a spent budget, a file
// not found and a search without a match are unavailable and become gaps.
func TestUpstreamBudgetAndUnavailableGaps(t *testing.T) {
	f := newConsumerFixture(t)
	src := map[string]map[string]string{"v2.1.0": {"big.txt": strings.Repeat("x\n", 200<<10)}}
	for i := 0; i < 6; i++ {
		src["v2.1.0"][fmt.Sprintf("f%d.go", i)] = "package f\n"
	}
	up := &fakeUpstream{src: src}
	port := up.port()
	steps := []Step{fileStep(f, "big.txt"), fileStep(f, "missing.go"),
		f.upstreamStep("call-9", map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "nowhere", "pattern": "x"})}
	for i := 0; i < 3; i++ {
		steps = append(steps, fileStep(f, fmt.Sprintf("f%d.go", i)))
	}
	steps = append(steps, fileStep(f, "f0.go"), fileStep(f, "f5.go"), f.v9Submit("potentially-affected", f.assessment()))
	d, out := v11Run(t, f, &port, steps...)
	reason := func(i int) string {
		v, err := resultOf(t, d, i)
		must(t, err)
		return string(v.Request.Status) + " " + v.Request.Resolution
	}
	for i, want := range map[int]string{
		0: "unavailable budget: the file is 409600 bytes and 393216 of the item's upstream bytes remain",
		1: "unavailable not found at v2.1.0",
		2: "unavailable not found at v2.1.0",
		3: "fulfilled", 5: "fulfilled",
		6: "fulfilled", // f0.go again: served from the item's earlier read, not counted
		7: "unavailable budget: the item's 6 upstream fetches are spent",
	} {
		if got := reason(i); !strings.HasPrefix(got, want) {
			t.Errorf("step %d: %q, want %q", i, got, want)
		}
	}
	if up.calls != 6 {
		t.Errorf("fetches %d, want 6 (the re-read and the over-budget request fetch nothing)", up.calls)
	}
	if v, _ := resultOf(t, d, 6); v.Budget != "6 of 6 upstream fetches and 30 of 393216 stored bytes used for this item" {
		t.Errorf("budget line %q", v.Budget)
	}
	a := out.Assessments[0]
	unknown := strings.Join(a.Unknown, "\n")
	if len(a.UnavailableEvidence) != 4 || !strings.Contains(unknown, "behaviour at v2.1.0 unavailable: budget: the file is 409600 bytes") ||
		!strings.Contains(unknown, "behaviour at v2.1.0 unavailable: not found at v2.1.0") || !strings.Contains(unknown, "upstream fetches are spent") ||
		!strings.HasPrefix(a.Unknown[0], "whether the application reloads the key") {
		t.Fatalf("gaps %v\n%s", a.UnavailableEvidence, unknown)
	}
	// A search without a match is a gap too.
	f2 := newConsumerFixture(t)
	up2 := &fakeUpstream{src: upstreamSource}
	port2 := up2.port()
	_, out2 := v11Run(t, f2, &port2, f2.upstreamStep("call-9", map[string]any{"kind": "upstream-search", "repo": upstreamRepo, "dir": "internal", "pattern": "nothing-matches-this"}),
		f2.v9Submit("potentially-affected", f2.assessment()))
	if r := out2.EvidenceRequests[0]; r.Status != domain.RequestUnavailable || !strings.HasPrefix(r.Resolution, `no match: pattern "nothing-matches-this" matched no line in the 2 files scanned`) ||
		len(out2.Assessments[0].UnavailableEvidence) != 1 {
		t.Fatalf("no match: %+v", r)
	}
}

// Prompts without feature upstream decode request_evidence exactly as
// before: an argument they never took, and an upstream kind, are refused
// with the messages their transcripts recorded.
func TestEarlierPromptsDecodeRequestEvidenceAsBefore(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	d, out := v9Run(t, f,
		Step{Call: "call-9", Tool: investigate.ToolRequestEvidence, Args: map[string]any{"changeItem": f.item, "kind": "artifact", "target": "https://example.dev/x", "reason": "r", "repo": upstreamRepo}},
		Step{Call: "call-9", Tool: investigate.ToolRequestEvidence, Args: map[string]any{"changeItem": f.item, "kind": "upstream-file", "target": "https://github.com/example/keystore/blob/v2.1.0/x", "reason": "r"}},
		Step{Call: "call-9", Tool: investigate.ToolRequestEvidence, Args: map[string]any{"changeItem": f.item, "kind": "artifact", "target": "https://example.dev/x", "reason": "r"}},
		f.v9Submit("potentially-affected", f.assessment()))
	if err := d.Results[0].Err; err == nil || err.Error() != `investigate: rejected: invalid arguments for request_evidence: json: unknown field "repo"` {
		t.Errorf("v9 with repo: %v", err)
	}
	if err := d.Results[1].Err; err == nil || err.Error() != `investigate: rejected: kind "upstream-file": want artifact, source, environment-input or live-observation` {
		t.Errorf("v9 with an upstream kind: %v", err)
	}
	if r := d.Results[2]; r.Err != nil || !strings.Contains(string(r.Result), "nothing is fetched during this session") {
		t.Errorf("v9 artifact request: %s %v", r.Result, r.Err)
	}
	if len(out.EvidenceRequests) != 1 || out.EvidenceRequests[0].Upstream != nil || len(out.Assessments[0].UnavailableEvidence) != 0 || up.calls != 0 {
		t.Fatalf("v9 outcome %+v", out.EvidenceRequests)
	}
}

// The item task names what upstream requests may read; earlier versions'
// tasks are unchanged.
func TestUpstreamLineInItemTask(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	s, _, err := prepare(f.st, f.input(v11Config()), nil, fixedNow, nil, 0, &port)
	must(t, err)
	if got := s.upstreamLine(); got != "repositories example/keystore; target's tag v2.1.0 (the default ref), current version's tag v2.0.0 (for comparison)" {
		t.Fatalf("upstream line %q", got)
	}
	p, err := LoadPrompt(promptV11)
	must(t, err)
	agenda := ItemAgenda(f.b, nil)
	task, err := p.renderItemTaskWith(f.input(v11Config()), agenda[0], 1, 1, 10, ItemDefaultBudget, []byte("{}"), "art-x", s.upstreamLine())
	must(t, err)
	if !strings.Contains(task, "- upstream source code readable with request_evidence (upstream-file, upstream-search): repositories example/keystore;") {
		t.Fatalf("v11 item task:\n%s", task)
	}
	p9, err := LoadPrompt(promptV9)
	must(t, err)
	a, err := p9.renderItemTask(f.input(v9Config()), agenda[0], 1, 1, 10, ItemDefaultBudget, []byte("{}"), "art-x")
	must(t, err)
	b, err := p9.renderItemTaskWith(f.input(v9Config()), agenda[0], 1, 1, 10, ItemDefaultBudget, []byte("{}"), "art-x", "ignored")
	must(t, err)
	if a != b || strings.Contains(a, "upstream source code") {
		t.Fatal("the v9 item task changed")
	}
}

// Upstream requests, fetched artifacts and the gaps they leave never change
// the classification.
func TestUpstreamNeverReachesThePolicy(t *testing.T) {
	f := newConsumerFixture(t)
	up := &fakeUpstream{src: upstreamSource}
	port := up.port()
	confident := f.assessment()
	confident["likelyImpact"] = "none: the upstream validation accepts every name this environment uses"
	_, out := v11Run(t, f, &port, Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		fileStep(f, "internal/validate/name.go"), fileStep(f, "missing.go"), f.v9Submit("not-affected", confident))
	if len(out.EvidenceRequests) != 2 || len(out.Assessments[0].UnavailableEvidence) != 1 {
		t.Fatalf("requests %+v", out.EvidenceRequests)
	}
	cfg := investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: v11Config()}
	run := func(agent *investigate.AgentOutcome) []byte {
		cls, err := classify.New().Classify(context.Background(), investigate.ClassifyInput{Bundle: f.b, Traversal: f.trav, Agent: agent, Validations: out.Validations, Config: cfg})
		must(t, err)
		b, err := json.Marshal(cls)
		must(t, err)
		return b
	}
	without := *out
	without.EvidenceRequests, without.Assessments, without.Verdicts = nil, nil, nil
	without.Artifacts, without.Evidence = nil, nil
	if a, b := run(&without), run(out); !bytes.Equal(a, b) {
		t.Fatalf("upstream evidence changed the classification:\nwithout: %s\nwith:    %s", a, b)
	}
}

// transcriptOf reads the outcome's transcript.
func transcriptOf(t *testing.T, f *consumerFixture, out *investigate.AgentOutcome) []byte {
	t.Helper()
	ref, ok := artifactOf(out, out.Run.Transcript)
	if !ok {
		t.Fatal("no transcript")
	}
	rc, err := f.st.Open(context.Background(), ref.Digest)
	must(t, err)
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	return b
}
