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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	// The deterministic pass before the agent (DESIGN.md §5.1): every
	// validator over every in-scope item.
	validators := liveValidators(st, rep, facts)
	var pre []domain.ValidationRecord
	seen := map[string]bool{}
	for _, it := range b.Change.Items {
		if !it.InScope {
			continue
		}
		for _, v := range validators {
			if !v.Applies(it.Family) {
				continue
			}
			vo, err := v.Validate(ctx, investigate.ValidationInput{Bundle: b, Graph: g, Traversal: trav, Report: rep, ChangeItem: it.ID})
			must(t, err)
			for _, r := range vo.Records {
				if !seen[r.ID] {
					seen[r.ID] = true
					pre = append(pre, r)
				}
			}
		}
	}

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
	cfg := investigate.AgentConfig{Enabled: true, Driver: DriverClaudeCLI, Model: model, PromptVersion: DefaultPromptVersion, Budget: budget}
	input := investigate.InvestigateInput{Bundle: b, Graph: g, Traversal: trav, Report: rep, Validations: pre, Config: cfg}
	_, as, err := Prepare(st, input, nil, nil)
	must(t, err)
	must(t, os.MkdirAll(outDir, 0o755))
	write := func(name string, b []byte) { must(t, os.WriteFile(filepath.Join(outDir, name), b, 0o644)) }
	write("task.md", []byte(as.Task))

	inv := New(st, WithDriver(DriverClaudeCLI, &ClaudeCLI{RIBinary: riBin, Stderr: os.Stderr}), WithValidators(validators...))
	out, err := inv.Investigate(ctx, input)
	must(t, err)

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
		"traversal": map[string]any{"roots": len(trav.Roots), "paths": len(trav.Paths), "unresolved": len(trav.Unresolved), "complete": trav.Complete},
		"impactReport": rep != nil, "validators": len(validators), "deterministicPass": len(pre)}
	var items []item
	for _, it := range b.Change.Items {
		if it.InScope {
			items = append(items, item{it.ID, it.Title, string(it.Family), string(it.Manifestation), it.ManifestationRule, it.ScopeRule})
		}
	}
	sum["inScope"] = items
	sb, _ := json.MarshalIndent(sum, "", "  ")
	write("bundle-summary.json", append(sb, '\n'))

	run := runRecord(t, &fixture{st: st, b: b, g: g, trav: trav}, out, pre...)
	if err := run.ValidateAgainst(b); err != nil {
		t.Errorf("run record: %v", err)
	}
	t.Logf("agent run: status %s (%s), model %s, %d turns, %d tool calls, %d tokens; %d hypotheses, %d assertions, %d evidence requests, %d validation requests",
		out.Run.Status, out.Run.Reason, out.Run.Model, out.Run.Turns, out.Run.ToolCalls, out.Run.Tokens, len(out.Hypotheses), len(out.Assertions),
		len(out.EvidenceRequests), len(out.ValidationRequests))
}
