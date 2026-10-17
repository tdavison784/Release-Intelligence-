package main

// `ri eval -investigations`: the Phase 4 investigation evaluation
// (docs/phase4/env-impact/DESIGN.md §6). Every case's input.yaml + env/ goes
// through the real in-process investigation pipeline (the same wiring as
// `ri investigate`); the stored run records are scored against expected.yaml
// by internal/eval and judged by the frozen eval/investigations/gates.yaml.
// The pipeline never sees expected.yaml.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/eval"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
	runpkg "github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
)

// invResultsDir is where scored runs are kept (never overwritten).
const invResultsDir = "results"

// InvRunMeta is results/<run-id>/run.json: what was run, by which binary, on
// which inputs.
type InvRunMeta struct {
	RunID        string             `json:"runId"`
	Arm          string             `json:"arm"`
	ArmDetail    string             `json:"armDetail"`
	Commit       string             `json:"commit"`
	Dirty        bool               `json:"dirty"`
	GoVersion    string             `json:"goVersion"`
	Args         []string           `json:"args"`
	StartedAt    time.Time          `json:"startedAt"`
	FinishedAt   time.Time          `json:"finishedAt"`
	Config       investigate.Config `json:"config"`
	ConfigDigest string             `json:"configDigest"`
	GatesDigest  string             `json:"gatesDigest"`
	Cases        []InvRunMetaCase   `json:"cases"`
	// KnowledgeDigest is the content digest of the knowledge tree the run
	// used (PROTOCOL.md §3, §8); "none" when knowledge was switched off.
	// KnowledgeLive marks a run that used the live tree on request.
	KnowledgeDigest string `json:"knowledgeDigest,omitempty"`
	KnowledgeLive   bool   `json:"knowledgeLive,omitempty"`
	// Appends records every later invocation that added cases to the run
	// (-append): the run is built case by case, never rewritten.
	Appends []InvRunAppend `json:"appends,omitempty"`
}

// InvRunAppend is one -append invocation.
type InvRunAppend struct {
	Args       []string  `json:"args"`
	Cases      []string  `json:"cases"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
}

// InvRunMetaCase pins one case's inputs and outputs.
type InvRunMetaCase struct {
	ID             string `json:"id"`
	InputDigest    string `json:"inputDigest"`
	EnvDigest      string `json:"envDigest"`
	ExpectedDigest string `json:"expectedDigest"`
	Investigation  string `json:"investigation,omitempty"`
	Bundle         string `json:"bundle,omitempty"`
	RecordDigest   string `json:"recordDigest,omitempty"`
	AgentPrompt    string `json:"agentPromptDigest,omitempty"`
	AgentModel     string `json:"agentModel,omitempty"`
	Error          string `json:"error,omitempty"`
	Seconds        int    `json:"seconds"`
}

// invStored is everything a case's run leaves under cases/<id>/: enough to
// re-classify and re-score offline.
type invStored struct {
	Request    investigate.Request
	Inv        *domain.Investigation
	Bundle     *domain.EvidenceBundle
	Graph      *domain.RelationshipGraph
	Report     *domain.ImpactReport
	ChangeText map[string]string
	Transcript []byte
}

func hasFlag(args []string, name string) ([]string, bool) {
	var out []string
	found := false
	for _, a := range args {
		if a == "-"+name || a == "--"+name {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

func (c *cli) evalInvestigations(args []string) error {
	fs := c.flags("eval -investigations", "")
	dir := fs.String("dir", "eval/investigations", "investigation dataset (cases, gates.yaml, results/)")
	only := fs.String("case", "", "comma-separated case ids (default: every case)")
	output := fs.String("o", "text", "output format: text|json")
	save := fs.Bool("save", false, "store the scored run under <dir>/results/<run-id>/ (never overwritten)")
	appendTo := fs.String("append", "", "add the selected cases to this stored run (same arm, configuration and binary commit; a case already in the run is refused) and re-score the run over all its cases")
	arm := fs.String("arm", "", "arm label for the stored run (default: A deterministic, B +agent, C +runtime)")
	runID := fs.String("run-id", "", "run id (default <arm>-<UTC time>-<commit>)")
	work := fs.String("work", "", "investigation store directory (default: a temporary directory, removed afterwards)")
	rescore := fs.String("rescore", "", "re-classify and re-score a stored run directory offline (no pipeline run)")
	pack := fs.String("pack", "", "convert a stored run directory from the legacy cases/ layout to cases.tar.gz (contents byte-identical) and exit")
	knowledge := fs.String("knowledge", "knowledge", "directory of verified knowledge (as for ri investigate)")
	agentOn := fs.Bool("agent", false, "run the investigation agent (arm B)")
	driver := fs.String("agent-driver", "claude-cli", "agent driver: claude-cli|exchange|replay|scripted")
	model := fs.String("agent-model", "", "model for the agent driver")
	replayDir := fs.String("agent-replay-dir", "", "replay driver: a stored run directory whose per-case transcript.jsonl (in cases.tar.gz) is replayed")
	claudeBin := fs.String("claude-bin", "", "claude CLI binary (claude-cli driver)")
	allowLive := fs.Bool("allow-live-knowledge", false, "permit the live knowledge/ tree (default: -knowledge must name a snapshot; see scripts/phase5/snapshot.sh)")
	runtimeOn := fs.Bool("runtime", false, "add runtime (observed) validation in throwaway kind clusters (arm C)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if *pack != "" {
		return packInvRun(*pack)
	}
	gates, err := eval.LoadInvGates(filepath.Join(*dir, eval.InvGatesFileName))
	if err != nil {
		return err
	}
	cases, err := eval.LoadInvCases(*dir, splitList(*only)...)
	if err != nil {
		return err
	}
	if *rescore != "" {
		return c.invRescore(*rescore, cases, gates, *output)
	}
	if *runtimeOn {
		// One throwaway kind cluster per case: fail before creating anything
		// when the tools are missing (a missing tool is a usage error, never
		// a silent no-op that would mislabel the arm).
		for _, bin := range []string{"helm", "kubectl", "kind", "docker"} {
			if _, err := exec.LookPath(bin); err != nil {
				return fmt.Errorf("%w: -runtime needs %s on PATH", app.ErrUsage, bin)
			}
		}
	}
	knowledgeSet := false
	fs.Visit(func(f *flag.Flag) { knowledgeSet = knowledgeSet || f.Name == "knowledge" })
	if err := checkEvalKnowledge(knowledgeSet, *knowledge, *allowLive, liveKnowledgeRoots()); err != nil {
		return err
	}
	kdigest, err := knowledgeTreeDigest(*knowledge)
	if err != nil {
		return err
	}
	if *agentOn && *driver == "replay" && *replayDir == "" {
		return fmt.Errorf("%w: -agent-driver replay needs -agent-replay-dir", app.ErrUsage)
	}
	label, detail := "A", "deterministic only (no agent, no runtime)"
	switch {
	case *agentOn && *runtimeOn:
		label, detail = "B+C", fmt.Sprintf("+agent (driver %s, model %s) +runtime (observed validation, throwaway kind clusters)", *driver, orDefault(*model, "default"))
	case *agentOn:
		label, detail = "B", fmt.Sprintf("+agent (driver %s, model %s)", *driver, orDefault(*model, "default"))
	case *runtimeOn:
		label, detail = "C", "+runtime: observed validation in throwaway kind clusters (no agent)"
	}
	if *arm != "" {
		label = *arm
	}
	workDir := *work
	if workDir == "" {
		if workDir, err = os.MkdirTemp("", "ri-eval-inv-*"); err != nil {
			return err
		}
		defer os.RemoveAll(workDir)
	}
	commit, dirty := buildCommit()
	meta := InvRunMeta{Arm: label, ArmDetail: detail, Commit: commit, Dirty: dirty, GoVersion: runtime.Version(),
		Args: append([]string{"eval", "-investigations"}, args...), StartedAt: time.Now().UTC(), GatesDigest: gates.Digest,
		KnowledgeDigest: kdigest, KnowledgeLive: *allowLive && knowledgeSet && samePathAny(*knowledge, liveKnowledgeRoots())}
	short := commit
	if len(short) > 10 {
		short = short[:10]
	}
	meta.RunID = *runID
	if meta.RunID == "" {
		meta.RunID = fmt.Sprintf("%s-%s-%s", label, meta.StartedAt.Format("20060102T150405Z"), short)
	}
	outDir := filepath.Join(*dir, invResultsDir, meta.RunID)
	if *save && *appendTo != "" {
		return fmt.Errorf("%w: -save starts a run, -append extends one; not both", app.ErrUsage)
	}
	if *save {
		if _, err := os.Stat(outDir); err == nil {
			return fmt.Errorf("%s exists: scored runs are never overwritten", outDir)
		}
	}
	var prior *InvRunMeta
	if *appendTo != "" {
		if prior, err = readInvRunMeta(*appendTo); err != nil {
			return err
		}
		switch {
		case prior.Arm != label:
			return fmt.Errorf("-append: run %s is arm %s, this invocation is arm %s", prior.RunID, prior.Arm, label)
		case prior.KnowledgeDigest != kdigest:
			return fmt.Errorf("-append: run %s used knowledge %q, this invocation %q — one arm, one knowledge snapshot", prior.RunID, prior.KnowledgeDigest, kdigest)
		case prior.Commit != commit || prior.Dirty != dirty:
			return fmt.Errorf("-append: run %s was made by binary %s (dirty=%v), this is %s (dirty=%v): use the same binary", prior.RunID, prior.Commit, prior.Dirty, commit, dirty)
		}
		for _, m := range prior.Cases {
			for _, cs := range cases {
				if cs.ID == m.ID {
					return fmt.Errorf("-append: case %s is already in run %s (scored runs are never overwritten)", cs.ID, prior.RunID)
				}
			}
		}
		outDir, meta.RunID, short = *appendTo, prior.RunID, shortCommit(prior.Commit)
	}

	runs := map[string]*eval.InvRun{}
	stored := map[string]*invStored{}
	for _, cs := range cases {
		t0 := time.Now()
		fmt.Fprintf(c.err, "· %s …\n", cs.ID)
		s, run, err := c.invRunCase(cs, workDir, invCaseFlags{knowledge: *knowledge, agent: *agentOn, driver: *driver, model: *model,
			replayDir: *replayDir, claudeBin: *claudeBin, runtime: *runtimeOn})
		m := InvRunMetaCase{ID: cs.ID, InputDigest: cs.InputDigest, EnvDigest: cs.EnvDigest, ExpectedDigest: cs.ExpectedDigest,
			Seconds: int(time.Since(t0).Seconds())}
		if err != nil {
			run = &eval.InvRun{Case: cs.ID, Error: err.Error()}
			m.Error = err.Error()
			fmt.Fprintf(c.err, "  ✗ pipeline failure: %v\n", err)
		} else {
			stored[cs.ID] = s
			m.Investigation, m.Bundle = s.Inv.ID, s.Bundle.ID
			m.RecordDigest = recordDigest(s.Inv)
			if s.Inv.Agent != nil {
				m.AgentPrompt, m.AgentModel = s.Inv.Agent.PromptDigest, s.Inv.Agent.Model
			}
			if meta.ConfigDigest == "" {
				meta.Config, meta.ConfigDigest = s.Request.Config, s.Request.Config.Digest()
			}
			if prior != nil && s.Request.Config.Digest() != prior.ConfigDigest {
				return fmt.Errorf("-append: %s ran with configuration %s, run %s has %s — not the same arm", cs.ID, s.Request.Config.Digest(), prior.RunID, prior.ConfigDigest)
			}
		}
		runs[cs.ID] = run
		meta.Cases = append(meta.Cases, m)
	}
	meta.FinishedAt = time.Now().UTC()
	var priorFiles map[string][]byte
	scored := cases
	if prior != nil {
		merged, files, all, err := c.invMergeAppend(*dir, *appendTo, prior, meta, runs, args)
		if err != nil {
			return err
		}
		meta, priorFiles, scored = merged, files, all
	}
	score := eval.ScoreInvestigations(scored, runs, gates)
	title := fmt.Sprintf("investigation eval — arm %s: %s — run %s (commit %s%s)", label, detail, meta.RunID, short, map[bool]string{true: ", DIRTY tree"}[dirty])

	var text bytes.Buffer
	eval.WriteInvText(&text, title, score)
	if *save || prior != nil {
		if err := saveInvRun(outDir, meta, score, stored, text.Bytes(), priorFiles); err != nil {
			return err
		}
		fmt.Fprintf(c.err, "stored %s\n", outDir)
	}
	if *output == "json" {
		if err := c.writeJSON(score); err != nil {
			return err
		}
	} else if _, err := c.out.Write(text.Bytes()); err != nil {
		return err
	}
	return invGateError(score)
}

func invGateError(score *eval.InvScore) error {
	fail, insufficient := score.GateFailures()
	if fail+insufficient > 0 {
		return fmt.Errorf("%d gate(s) FAIL, %d INSUFFICIENT-DATA (eval/investigations/gates.yaml; never a pass) — see the gate panel", fail, insufficient)
	}
	return nil
}

type invCaseFlags struct {
	knowledge                           string
	agent                               bool
	runtime                             bool
	driver, model, replayDir, claudeBin string
}

// invRunCase runs one case through the real pipeline and loads what it stored.
func (c *cli) invRunCase(cs *eval.InvCase, workDir string, cf invCaseFlags) (*invStored, *eval.InvRun, error) {
	req0 := cs.Request(investigate.Config{})
	str := func(s string) *string { return &s }
	b := func(v bool) *bool { return &v }
	replay := ""
	if cf.agent && cf.driver == "replay" {
		rf, err := openInvRunFiles(cf.replayDir)
		if err != nil {
			return nil, nil, err
		}
		raw, err := rf.read(cs.ID, "transcript.jsonl")
		if err != nil {
			return nil, nil, fmt.Errorf("replay transcript of %s: %w", cs.ID, err)
		}
		replay = filepath.Join(workDir, "replay", cs.ID+".jsonl")
		if err := os.MkdirAll(filepath.Dir(replay), 0o755); err != nil {
			return nil, nil, err
		}
		if err := os.WriteFile(replay, raw, 0o644); err != nil {
			return nil, nil, err
		}
	}
	f := invFlags{
		repo: str(req0.Environment.Repo), values: str(""), manifests: str(""), crds: str(""), images: str(""),
		inventory: str(req0.Environment.Inventory), kubernetes: str(req0.Environment.KubernetesVersion), envName: str(req0.Environment.Name),
		knowledge: str(cf.knowledge), minVerification: str(""), render: b(true), agent: b(cf.agent), runtime: b(cf.runtime),
		driver: str(cf.driver), model: str(cf.model), exchange: str(""), replay: str(replay), script: str(""), claudeBin: str(cf.claudeBin),
	}
	req, err := f.request()
	if err != nil {
		return nil, nil, err
	}
	req.Product, req.From, req.To = req0.Product, req0.From, req0.To
	p, st, err := c.pipelineAt(f, workDir)
	if err != nil {
		return nil, nil, err
	}
	r := &runpkg.Runner{P: p, Definition: c.definitionDigest}
	if c.g.verbose {
		r.Logf = func(f string, a ...any) { fmt.Fprintf(c.err, "  · "+f+"\n", a...) }
	}
	res, err := r.Run(c.ctx, req)
	if err != nil {
		return nil, nil, err
	}
	s, err := loadInvFromStore(c.ctx, st, req, res.Investigation)
	if err != nil {
		return nil, nil, err
	}
	run := s.evalRun(cs.ID)
	return s, run, nil
}

// loadInvFromStore reads the run record, its bundle and artifacts back.
func loadInvFromStore(ctx context.Context, st *store.Store, req investigate.Request, id string) (*invStored, error) {
	inv, err := st.GetInvestigation(ctx, id)
	if err != nil {
		return nil, err
	}
	b, err := st.GetBundle(ctx, inv.Bundle)
	if err != nil {
		return nil, err
	}
	s := &invStored{Request: req, Inv: inv, Bundle: b, ChangeText: map[string]string{}}
	byID := map[string]domain.ArtifactRef{}
	for _, a := range append(append([]domain.ArtifactRef{}, b.Manifest.Artifacts...), inv.Artifacts...) {
		byID[a.ID] = a
	}
	read := func(id string, v any) error {
		a, ok := byID[id]
		if !ok {
			return fmt.Errorf("artifact %s not in the run", id)
		}
		rc, err := st.Open(ctx, a.Digest)
		if err != nil {
			return err
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		if bb, ok := v.(*[]byte); ok {
			*bb = raw
			return nil
		}
		return json.Unmarshal(raw, v)
	}
	if inv.Graph != "" {
		s.Graph = &domain.RelationshipGraph{}
		if err := read(inv.Graph, s.Graph); err != nil {
			return nil, fmt.Errorf("graph: %w", err)
		}
	}
	if inv.ImpactReport != "" {
		s.Report = &domain.ImpactReport{}
		if err := read(inv.ImpactReport, s.Report); err != nil {
			return nil, fmt.Errorf("impact report: %w", err)
		}
	}
	if b.Change.Edge != "" {
		var edge domain.UpgradeEdge
		if err := read(b.Change.Edge, &edge); err != nil {
			return nil, fmt.Errorf("upgrade edge: %w", err)
		}
		for _, ch := range edge.Changes {
			s.ChangeText[ch.ID] = ch.Detail
		}
	}
	if inv.Agent != nil && inv.Agent.Transcript != "" {
		if err := read(inv.Agent.Transcript, &s.Transcript); err != nil {
			return nil, fmt.Errorf("agent transcript: %w", err)
		}
	}
	return s, nil
}

// evalRun turns the stored records into the scorer's input, re-classifying
// them (the reproducibility metric).
func (s *invStored) evalRun(caseID string) *eval.InvRun {
	run := &eval.InvRun{Case: caseID, Investigation: s.Inv, Bundle: s.Bundle, Graph: s.Graph, ChangeText: s.ChangeText,
		Verdicts: agentVerdictInputs(s)}
	ok, detail := reclassify(s)
	run.Reproduced, run.ReproduceDetail = &ok, detail
	return run
}

// reclassify re-runs the deterministic classifier on the stored records and
// compares its findings and decision with the stored ones byte for byte
// (DESIGN.md §4.5, §6.2 reproducibility).
func reclassify(s *invStored) (bool, string) {
	var ag *investigate.AgentOutcome
	if s.Inv.Agent != nil {
		ag = &investigate.AgentOutcome{Run: *s.Inv.Agent, Hypotheses: s.Inv.Hypotheses, Assertions: s.Inv.Assertions,
			EvidenceRequests: s.Inv.EvidenceRequests, ValidationRequests: s.Inv.ValidationRequests}
	}
	cls, err := classify.New().Classify(context.Background(), investigate.ClassifyInput{Bundle: s.Bundle, Traversal: s.Inv.Traversal,
		Report: s.Report, Agent: ag, Validations: s.Inv.Validations, Config: s.Request.Config})
	if err != nil {
		return false, "re-classification failed: " + err.Error()
	}
	got, _ := json.Marshal(cls.Findings)
	want, _ := json.Marshal(s.Inv.Findings)
	if !bytes.Equal(got, want) {
		return false, fmt.Sprintf("findings differ (%d re-classified vs %d stored)", len(cls.Findings), len(s.Inv.Findings))
	}
	if s.Inv.Decision != nil {
		gd, _ := json.Marshal(cls.Decision)
		wd, _ := json.Marshal(s.Inv.Decision)
		if !bytes.Equal(gd, wd) {
			return false, "decision differs"
		}
	}
	return true, ""
}

// recordDigest is the run package's content digest of the record (no audit,
// no wall-clock stamps), so two runs of the same inputs compare equal.
func recordDigest(inv *domain.Investigation) string { return runpkg.ContentDigest(inv) }

func saveInvRun(dir string, meta InvRunMeta, score *eval.InvScore, stored map[string]*invStored, text []byte, prior map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	small := map[string]any{"run.json": meta, "gates.json": invGatesSummary(meta, score), "report.txt": text}
	for name, v := range small {
		raw, err := invFileBytes(v)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			return err
		}
	}
	raw, err := invFileBytes(score)
	if err != nil {
		return err
	}
	if err := writeInvGzip(filepath.Join(dir, invScoreFile), raw); err != nil {
		return err
	}
	files := map[string][]byte{}
	for n, b := range prior {
		files[n] = b
	}
	for id, s := range stored {
		parts := map[string]any{"request.json": s.Request, "investigation.json": s.Inv, "bundle.json": s.Bundle, "changes.json": s.ChangeText}
		if s.Graph != nil {
			parts["graph.json"] = s.Graph
		}
		if s.Report != nil {
			parts["impact-report.json"] = s.Report
		}
		if len(s.Transcript) > 0 {
			parts["transcript.jsonl"] = s.Transcript
		}
		for name, v := range parts {
			raw, err := invFileBytes(v)
			if err != nil {
				return err
			}
			files[invCasePath(id, name)] = raw
		}
	}
	return writeInvArchive(filepath.Join(dir, invCasesArchive), files)
}

// invRescore re-classifies and re-scores a stored run offline.
func (c *cli) invRescore(dir string, cases []*eval.InvCase, gates *eval.InvGates, output string) error {
	var meta InvRunMeta
	raw, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return err
	}
	pinned := map[string]InvRunMetaCase{}
	for _, m := range meta.Cases {
		pinned[m.ID] = m
	}
	rf, err := openInvRunFiles(dir)
	if err != nil {
		return err
	}
	runs := map[string]*eval.InvRun{}
	for _, cs := range cases {
		m, ok := pinned[cs.ID]
		switch {
		case !ok:
			continue
		case m.ExpectedDigest != cs.ExpectedDigest || m.InputDigest != cs.InputDigest || m.EnvDigest != cs.EnvDigest:
			fmt.Fprintf(c.err, "warning: %s changed since run %s (input/env/expected digest differs)\n", cs.ID, meta.RunID)
		}
		if m.Error != "" {
			runs[cs.ID] = &eval.InvRun{Case: cs.ID, Error: m.Error}
			continue
		}
		s, err := loadInvStored(rf, cs.ID)
		if err != nil {
			return fmt.Errorf("%s: %w", cs.ID, err)
		}
		runs[cs.ID] = s.evalRun(cs.ID)
	}
	var sel []*eval.InvCase
	for _, cs := range cases {
		if _, ok := runs[cs.ID]; ok {
			sel = append(sel, cs)
		}
	}
	score := eval.ScoreInvestigations(sel, runs, gates)
	if output == "json" {
		if err := c.writeJSON(score); err != nil {
			return err
		}
	} else {
		eval.WriteInvText(c.out, fmt.Sprintf("investigation eval — RESCORE of stored run %s (arm %s: %s)", meta.RunID, meta.Arm, meta.ArmDetail), score)
	}
	return invGateError(score)
}

// loadInvStored reads one case's stored records from a run (archive or
// legacy directory layout).
func loadInvStored(rf *invRunFiles, caseID string) (*invStored, error) {
	s := &invStored{Inv: &domain.Investigation{}, Bundle: &domain.EvidenceBundle{}, ChangeText: map[string]string{}}
	get := func(name string, v any, required bool) (bool, error) {
		raw, err := rf.read(caseID, name)
		if errors.Is(err, os.ErrNotExist) && !required {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, json.Unmarshal(raw, v)
	}
	for _, f := range []struct {
		name     string
		v        any
		required bool
	}{{"request.json", &s.Request, true}, {"investigation.json", s.Inv, true}, {"bundle.json", s.Bundle, true}, {"changes.json", &s.ChangeText, false}} {
		if _, err := get(f.name, f.v, f.required); err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
	}
	g := &domain.RelationshipGraph{}
	if ok, err := get("graph.json", g, false); err != nil {
		return nil, err
	} else if ok {
		s.Graph = g
	}
	rep := &domain.ImpactReport{}
	if ok, err := get("impact-report.json", rep, false); err != nil {
		return nil, err
	} else if ok {
		s.Report = rep
	}
	if raw, err := rf.read(caseID, "transcript.jsonl"); err == nil {
		s.Transcript = raw
	}
	return s, nil
}

// buildCommit is the binary's VCS revision (go build stamps it), falling back
// to the working tree's HEAD.
func buildCommit() (string, bool) {
	if bi, ok := debug.ReadBuildInfo(); ok {
		rev, dirty := "", false
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if rev != "" {
			return rev, dirty
		}
	}
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown", true
	}
	st, _ := exec.Command("git", "status", "--porcelain").Output()
	return strings.TrimSpace(string(out)), len(bytes.TrimSpace(st)) > 0
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
