package agent

// The live agent run (lane agent, brief "one live claude -p run"): a real
// bundle from one groundtruth case's input.yaml and env/ (never
// expected.yaml, NOTES.md or observed/), real renders, the real graph and
// traversal, and the claude-cli driver against the logged-in CLI. Opt-in:
//
//	go build -o bin/ri ./cmd/ri
//	RI_AGENT_LIVE=<case id> RI_STATE=<warm .ri> RI_BIN=$PWD/bin/ri RI_AGENT_OUT=<dir> \
//	  [RI_AGENT_MODEL=claude-sonnet-5-5] [RI_AGENT_TOOLCALLS=n RI_AGENT_TURNS=n] \
//	  go test ./internal/investigate/agent -run TestLiveAgentRun -v -timeout 40m
//
// It writes the task, the transcript, the outcome and a bundle summary to
// RI_AGENT_OUT, and checks the run record built from the outcome validates
// against the bundle. It asserts nothing about what the agent concluded.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/bundle"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
	"github.com/tdavison784/release-intelligence/internal/investigate/renderdiff"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/investigate/validate"
)

// liveInput is the part of input.yaml the run needs.
type liveInput struct {
	ID      string `yaml:"id"`
	Product string `yaml:"product"`
	From    struct {
		Version string `yaml:"version"`
	} `yaml:"from"`
	To struct {
		Version string `yaml:"version"`
	} `yaml:"to"`
	Environment struct {
		ID         string `yaml:"id"`
		Root       string `yaml:"root"`
		Kubernetes string `yaml:"kubernetes"`
	} `yaml:"environment"`
}

// liveValidators are lane validate's five validators, as the orchestrator
// wires them: the deterministic pass before the agent and request_validation.
func liveValidators(st investigate.ArtifactStore, rep *domain.ImpactReport, facts []domain.VerifiedFact) []investigate.Validator {
	return []investigate.Validator{validate.JoinVerdicts(rep, validate.WithStore(st)), validate.RenderDiff(validate.WithStore(st)),
		validate.OverrideInspection(validate.WithStore(st), validate.WithFacts(facts...)), validate.SchemaCompat(validate.WithStore(st)),
		validate.StaticReference(validate.WithStore(st))}
}

func TestLiveAgentRun(t *testing.T) {
	caseID, state, riBin, outDir := os.Getenv("RI_AGENT_LIVE"), os.Getenv("RI_STATE"), os.Getenv("RI_BIN"), os.Getenv("RI_AGENT_OUT")
	if caseID == "" {
		t.Skip("set RI_AGENT_LIVE=<groundtruth case id> (and RI_STATE, RI_BIN, RI_AGENT_OUT) for the live claude -p run")
	}
	if state == "" || riBin == "" || outDir == "" {
		t.Fatal("RI_STATE, RI_BIN and RI_AGENT_OUT are required")
	}
	for _, bin := range []string{"claude", "helm", "kubectl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not on PATH", bin)
		}
	}
	root := filepath.Join("..", "..", "..")
	caseDir := filepath.Join(root, "eval", "investigations", caseID)
	raw, err := os.ReadFile(filepath.Join(caseDir, "input.yaml")) // inputs only (FLEET.md "Evaluation integrity")
	must(t, err)
	var in liveInput
	must(t, yaml.Unmarshal(raw, &in))
	envDir, err := filepath.Abs(filepath.Join(caseDir, in.Environment.Root))
	must(t, err)

	ctx := context.Background()
	a, err := app.New(app.Config{ProductsDir: filepath.Join(root, "products"), StateDir: state, Offline: true})
	must(t, err)
	st, err := store.New(t.TempDir())
	must(t, err)
	var facts []domain.VerifiedFact
	if ks, err := app.LoadKnowledge(filepath.Join(root, "knowledge")); err == nil {
		facts = ks.Facts
	}
	change, err := bundle.NewChangeCollector(a, st, bundle.WithKnowledge(facts...)).CollectChange(ctx, investigate.ChangeRequest{Product: in.Product, From: in.From.Version, To: in.To.Version, Render: true})
	must(t, err)
	snap, err := bundle.NewSnapshotBuilderWithHints(st, env.HintsFromCatalog(a.Catalog)).BuildSnapshot(ctx,
		investigate.EnvironmentInputs{Name: in.Environment.ID, Repo: envDir, KubernetesVersion: in.Environment.Kubernetes})
	must(t, err)
	renders, err := renderdiff.New(a, st).RenderEnvironment(ctx, change, snap, investigate.Config{Render: true, KubeVersion: in.Environment.Kubernetes})
	must(t, err)
	b, err := bundle.NewAssembler(st).Assemble(ctx, change, snap, renders)
	must(t, err)
	rep, err := a.Impact(ctx, in.Product, in.From.Version, in.To.Version, app.ImpactOptions{Environment: env.Inputs{Repo: envDir, KubernetesVersion: in.Environment.Kubernetes},
		Facts: facts, MinVerification: domain.VerifiedHuman, Render: &app.RenderOptions{Repo: envDir, KubeVersion: in.Environment.Kubernetes}})
	if err != nil {
		t.Logf("no impact report: %v", err)
		rep = nil
	}
	g, err := relations.NewBuilder(st, relations.WithCatalog(a.Catalog)).BuildGraph(ctx, b)
	must(t, err)
	roots, err := relations.NewExposure().Exposure(ctx, b, g, rep)
	must(t, err)
	trav, err := relations.NewTraverser().Traverse(ctx, g, roots, domain.DefaultTraversalBound)
	must(t, err)

	// The deterministic pass before the agent (DESIGN.md §5.1).
	validators := liveValidators(st, rep, facts)
	pass, err := DeterministicPass(ctx, validators, investigate.ValidationInput{Bundle: b, Graph: g, Traversal: trav, Report: rep})
	must(t, err)
	pre, preArts, preEv := pass.Records, pass.Artifacts, pass.Evidence

	model := os.Getenv("RI_AGENT_MODEL")
	if model == "" {
		model = "claude-sonnet-5-5"
	}
	budget := investigate.DefaultAgentBudget
	if n, err := strconv.Atoi(os.Getenv("RI_AGENT_TOOLCALLS")); err == nil {
		budget.MaxToolCalls = n
	}
	if n, err := strconv.Atoi(os.Getenv("RI_AGENT_TURNS")); err == nil {
		budget.MaxTurns = n
	}
	prompt := os.Getenv("RI_AGENT_PROMPT") // e.g. investigate-agent/v1 to replay run 1
	if prompt == "" {
		prompt = DefaultPromptVersion
	}
	cfg := investigate.AgentConfig{Enabled: true, Driver: DriverClaudeCLI, Model: model, PromptVersion: prompt, Budget: budget}
	input := investigate.InvestigateInput{Bundle: b, Graph: g, Traversal: trav, Report: rep, Validations: pre,
		ValidationArtifacts: preArts, ValidationEvidence: preEv, Config: cfg}
	_, as, err := Prepare(st, input, nil, nil)
	must(t, err)
	must(t, os.MkdirAll(outDir, 0o755))
	write := func(name string, b []byte) { must(t, os.WriteFile(filepath.Join(outDir, name), b, 0o644)) }
	write("task.md", []byte(as.Task))

	var driver investigate.AgentDriver = &ClaudeCLI{RIBinary: riBin, Stderr: os.Stderr}
	var replay *Replay
	if p := os.Getenv("RI_AGENT_REPLAY"); p != "" {
		// Re-issue a recorded live run against the rebuilt bundle: no model call.
		rec, err := os.ReadFile(p)
		must(t, err)
		replay = &Replay{Transcript: rec}
		driver = replay
	}
	inv := New(st, WithDriver(DriverClaudeCLI, driver), WithValidators(validators...))
	out, err := inv.Investigate(ctx, input)
	must(t, err)
	if replay != nil {
		if len(replay.Mismatches) > 0 {
			t.Fatalf("replay of %s: %d results differ: %+v", os.Getenv("RI_AGENT_REPLAY"), len(replay.Mismatches), replay.Mismatches)
		}
		t.Logf("replay: every one of the recorded results reproduced (status %s)", out.Run.Status)
		outDir = filepath.Join(outDir, "replay")
		must(t, os.MkdirAll(outDir, 0o755))
	}

	rc, err := st.Open(ctx, out.Artifacts[0].Digest)
	must(t, err)
	transcript, _ := io.ReadAll(rc)
	rc.Close()
	write("transcript.jsonl", transcript)
	ob, _ := json.MarshalIndent(out, "", "  ")
	write("outcome.json", append(ob, '\n'))
	type item struct {
		ID, Title, Family, Manifestation, ManifestationRule, ScopeRule string
	}
	sum := map[string]any{"case": in.ID, "bundle": b.ID, "promptVersion": as.PromptVersion, "promptDigest": as.PromptDigest, "model": model,
		"budget": budget, "renders": renderdiff.Summaries(b), "uncorrelated": len(b.Uncorrelated()), "graph": map[string]int{"nodes": len(g.Nodes), "edges": len(g.Edges)},
		"traversal":    map[string]any{"roots": len(trav.Roots), "paths": len(trav.Paths), "unresolved": len(trav.Unresolved), "complete": trav.Complete},
		"impactReport": rep != nil, "validators": len(validators), "deterministicPass": len(pre), "agenda": len(out.Run.Coverage)}
	var items []item
	for _, it := range b.Items() {
		if it.InScope {
			items = append(items, item{it.ID, it.Title, string(it.Family), string(it.Manifestation), it.ManifestationRule, it.ScopeRule})
		}
	}
	sum["inScope"] = items
	sb, _ := json.MarshalIndent(sum, "", "  ")
	write("bundle-summary.json", append(sb, '\n'))
	if len(out.Run.Coverage) > 0 {
		var md strings.Builder
		counts := map[domain.CoverageDisposition]int{}
		fmt.Fprintf(&md, "| # | item | agenda rule | disposition | examined | inspections | validation requests | records | session | tool calls | turns | tokens | summary |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
		for i, c := range out.Run.Coverage {
			counts[c.Disposition]++
			session := "—"
			if c.SessionStatus != "" {
				session = string(c.SessionStatus)
			}
			fmt.Fprintf(&md, "| %d | %s | %s | %s | %v | %d | %d | %d | %s | %d | %d | %d | %s |\n", i+1, c.ChangeItem, c.PriorityRule, c.Disposition, c.Examined, c.Inspections,
				c.ValidationRequests, len(c.Records), session, c.ToolCalls, c.Turns, c.Tokens, strings.ReplaceAll(clip(c.Summary, 220), "|", "\\|"))
		}
		fmt.Fprintf(&md, "\nTotals:")
		for _, d := range domain.CoverageDispositions {
			fmt.Fprintf(&md, " %s %d;", d, counts[d])
		}
		fmt.Fprintf(&md, " budget rule: %s\n", out.Run.BudgetRule)
		tr, err := ParseTranscript(transcript)
		must(t, err)
		byTool := map[string]int{}
		for _, c := range tr.Calls {
			if c.OK {
				byTool[c.Tool]++
			} else {
				byTool[c.Tool+" (rejected)"]++
			}
		}
		fmt.Fprintf(&md, "\nTool calls: %d (%v)\n", len(tr.Calls), byTool)
		fmt.Fprintf(&md, "\n## Validation requests (%d)\n\n", len(out.ValidationRequests))
		for _, r := range out.ValidationRequests {
			fmt.Fprintf(&md, "- %s %s %s subjects=%v answers=%v → %s %s %s\n", r.ID, r.ChangeItem, r.Kind, r.Subjects, r.Answers, r.Status, r.Result, clip(r.Resolution, 200))
		}
		for _, v := range out.Validations {
			fmt.Fprintf(&md, "  - record %s %s %s answers=%v: %s\n", v.ID, v.Validator, v.Outcome, v.Answers, clip(v.Detail, 200))
		}
		fmt.Fprintf(&md, "\n## Answer links (%d)\n\n", len(out.AnswerLinks))
		for _, l := range out.AnswerLinks {
			fmt.Fprintf(&md, "- %s ← %v (request %s)\n", l.Validation, l.Assertions, l.Request)
		}
		fmt.Fprintf(&md, "\n## Investigation-level notes (%d, AI; never item concerns)\n\n", len(out.Notes))
		for _, n := range out.Notes {
			fmt.Fprintf(&md, "- %s about %s (%s): %s\n", n.ID, n.About, n.Reason, clip(n.Statement, 240))
		}
		write("coverage.md", []byte(md.String()))
		t.Logf("coverage: %v; tool calls %v; validation requests %d; answer links %d", counts, byTool, len(out.ValidationRequests), len(out.AnswerLinks))
	}

	run := runRecord(t, &fixture{st: st, b: b, g: g, trav: trav}, out, ApplyAnswerLinks(pre, out.AnswerLinks)...)
	have := map[domain.EvidenceID]bool{}
	for _, e := range run.Evidence {
		have[e.ID] = true
	}
	for _, e := range preEv {
		if !have[e.ID] {
			have[e.ID] = true
			run.Evidence = append(run.Evidence, e)
		}
	}
	run.Artifacts = append(run.Artifacts, preArts...)
	if err := run.ValidateAgainst(b); err != nil {
		t.Errorf("run record: %v", err)
	}
	t.Logf("agent run: status %s (%s), model %s, %d turns, %d tool calls, %d tokens; %d hypotheses, %d assertions, %d evidence requests, %d validation requests",
		out.Run.Status, out.Run.Reason, out.Run.Model, out.Run.Turns, out.Run.ToolCalls, out.Run.Tokens, len(out.Hypotheses), len(out.Assertions),
		len(out.EvidenceRequests), len(out.ValidationRequests))
}
