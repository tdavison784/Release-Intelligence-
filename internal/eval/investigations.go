package eval

// Phase 4 investigation evaluation (docs/phase4/env-impact/DESIGN.md §6,
// eval/investigations/FORMAT.md): the dataset loader, the pre-registered
// gates of eval/investigations/gates.yaml and the input of the scorer.
//
// This file and its siblings (investigations_*.go) are the only pipeline-side
// code that reads expected.yaml. They never feed it back into the pipeline:
// the system under test receives input.yaml and env/ only (InvCase.Request).

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// InvGatesFileName is the frozen gates file of the investigation dataset.
const InvGatesFileName = "gates.yaml"

// Expected verification levels (FORMAT.md).
const (
	VerifyObserved             = "observed"
	VerifyRender               = "render"
	VerifyUpstreamOnly         = "upstream-only"
	VerifyObservedAlternatives = "observed-alternatives"
)

// InvInput is a case's input.yaml (everything the system may read, with env/).
type InvInput struct {
	ID      string `yaml:"id" json:"id"`
	Pair    string `yaml:"pair" json:"pair,omitempty"`
	Product string `yaml:"product" json:"product"`
	From    struct {
		Version      string `yaml:"version" json:"version"`
		ChartVersion string `yaml:"chartVersion" json:"chartVersion,omitempty"`
	} `yaml:"from" json:"from"`
	To struct {
		Version      string `yaml:"version" json:"version"`
		ChartVersion string `yaml:"chartVersion" json:"chartVersion,omitempty"`
	} `yaml:"to" json:"to"`
	Environment struct {
		ID         string `yaml:"id" json:"id"`
		Root       string `yaml:"root" json:"root"`
		Kubernetes string `yaml:"kubernetes" json:"kubernetes,omitempty"`
		Inventory  string `yaml:"inventory" json:"inventory,omitempty"`
	} `yaml:"environment" json:"environment"`
	Scope []struct {
		ID      string `yaml:"id" json:"id"`
		Title   string `yaml:"title" json:"title"`
		Subject string `yaml:"subject" json:"subject,omitempty"`
	} `yaml:"scope" json:"scope"`
}

// InvRef is a component reference of expected.yaml.
type InvRef struct {
	File         string `yaml:"file" json:"file,omitempty"`
	RenderedFrom string `yaml:"renderedFrom" json:"renderedFrom,omitempty"`
	APIVersion   string `yaml:"apiVersion" json:"apiVersion,omitempty"`
	Kind         string `yaml:"kind" json:"kind,omitempty"`
	Namespace    string `yaml:"namespace" json:"namespace,omitempty"`
	Name         string `yaml:"name" json:"name,omitempty"`
}

func (r InvRef) String() string {
	if r.Kind == "" {
		return r.File
	}
	parts := []string{r.Kind}
	if r.Namespace != "" {
		parts = append(parts, r.Namespace)
	}
	return strings.Join(append(parts, r.Name), "/")
}

// InvRequiredEvidence is one requiredEvidence entry.
type InvRequiredEvidence struct {
	File   string `yaml:"file" json:"file"`
	Object string `yaml:"object" json:"object,omitempty"`
	Field  string `yaml:"field" json:"field,omitempty"`
}

// InvComponent is one expected component of an item.
type InvComponent struct {
	Ref              InvRef                `yaml:"ref" json:"ref"`
	Classification   domain.FindingClass   `yaml:"classification" json:"classification"`
	RequiredEvidence []InvRequiredEvidence `yaml:"requiredEvidence" json:"requiredEvidence,omitempty"`
}

// InvMatch is one `match` alternative: the item is selected when any
// alternative matches.
type InvMatch struct {
	Subject string `yaml:"subject" json:"subject,omitempty"`
	Text    string `yaml:"text" json:"text,omitempty"`
}

// InvExpectedItem is one expected item decision.
type InvExpectedItem struct {
	Item           string                `yaml:"item" json:"item"`
	Classification domain.FindingClass   `yaml:"classification" json:"classification"`
	Acceptable     []domain.FindingClass `yaml:"acceptable" json:"acceptable,omitempty"`
	Verification   string                `yaml:"verification" json:"verification"`
	Components     []InvComponent        `yaml:"components" json:"components,omitempty"`
	Match          []InvMatch            `yaml:"match" json:"match"`
}

// InvExpected is a case's expected.yaml (ground truth).
type InvExpected struct {
	Case  string            `yaml:"case" json:"case"`
	Items []InvExpectedItem `yaml:"items" json:"items"`
}

// InvCase is one loaded case.
type InvCase struct {
	ID       string
	Dir      string // absolute case directory
	Input    InvInput
	Expected InvExpected
	// Digests of what the case consists of (reproducibility of a stored run).
	InputDigest, EnvDigest, ExpectedDigest string
}

// Request is the investigation request for the case: input.yaml + env/ only
// (FORMAT.md "How the harness runs a case"). The config is the run's.
func (c *InvCase) Request(cfg investigate.Config) investigate.Request {
	r := investigate.Request{Product: c.Input.Product, From: c.Input.From.Version, To: c.Input.To.Version, Config: cfg}
	root := c.Input.Environment.Root
	if root == "" {
		root = "env"
	}
	r.Environment = investigate.EnvironmentInputs{Name: c.Input.Environment.ID, Repo: filepath.Join(c.Dir, root),
		KubernetesVersion: c.Input.Environment.Kubernetes}
	if inv := c.Input.Environment.Inventory; inv != "" {
		r.Environment.Inventory = filepath.Join(c.Dir, inv)
	}
	return r
}

// LoadInvCases loads every case under dir (one subdirectory with input.yaml
// and expected.yaml each), sorted by id; only selects them when only is
// non-empty. A malformed case is an error: a typo must fail loudly, never
// silently score nothing.
func LoadInvCases(dir string, only ...string) ([]*InvCase, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	want, found := map[string]bool{}, map[string]bool{}
	for _, o := range only {
		want[o] = true
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	var out []*InvCase
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cdir := filepath.Join(abs, e.Name())
		if _, err := os.Stat(filepath.Join(cdir, "input.yaml")); err != nil {
			continue // results/, other non-case directories
		}
		if len(want) > 0 && !want[e.Name()] {
			continue
		}
		c, err := loadInvCase(cdir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		found[e.Name()] = true
		out = append(out, c)
	}
	if len(want) > len(found) {
		var missing []string
		for k := range want {
			if !found[k] {
				missing = append(missing, k)
			}
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("unknown investigation case(s): %s", strings.Join(missing, ", "))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func loadInvCase(dir string) (*InvCase, error) {
	c := &InvCase{ID: filepath.Base(dir), Dir: dir}
	in, err := os.ReadFile(filepath.Join(dir, "input.yaml"))
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(in, &c.Input); err != nil {
		return nil, fmt.Errorf("input.yaml: %w", err)
	}
	ex, err := os.ReadFile(filepath.Join(dir, "expected.yaml"))
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(ex, &c.Expected); err != nil {
		return nil, fmt.Errorf("expected.yaml: %w", err)
	}
	c.InputDigest, c.ExpectedDigest = digestBytes(in), digestBytes(ex)
	root := c.Input.Environment.Root
	if root == "" {
		root = "env"
	}
	if c.EnvDigest, err = treeDigest(filepath.Join(dir, root)); err != nil {
		return nil, fmt.Errorf("env: %w", err)
	}
	if c.Input.ID != c.ID || c.Expected.Case != c.ID {
		return nil, fmt.Errorf("case id mismatch: dir %q, input %q, expected %q", c.ID, c.Input.ID, c.Expected.Case)
	}
	if c.Input.Product == "" || c.Input.From.Version == "" || c.Input.To.Version == "" || c.Input.Environment.ID == "" {
		return nil, fmt.Errorf("input.yaml: product, from.version, to.version and environment.id are required")
	}
	seen := map[string]bool{}
	for _, it := range c.Expected.Items {
		if seen[it.Item] {
			return nil, fmt.Errorf("expected.yaml: item %s twice", it.Item)
		}
		seen[it.Item] = true
		if !validClass(it.Classification) {
			return nil, fmt.Errorf("expected.yaml: item %s: unknown classification %q", it.Item, it.Classification)
		}
		for _, a := range it.Acceptable {
			if !validClass(a) {
				return nil, fmt.Errorf("expected.yaml: item %s: unknown acceptable class %q", it.Item, a)
			}
		}
		switch it.Verification {
		case VerifyObserved, VerifyRender, VerifyUpstreamOnly, VerifyObservedAlternatives:
		default:
			return nil, fmt.Errorf("expected.yaml: item %s: unknown verification %q", it.Item, it.Verification)
		}
		if len(it.Match) == 0 {
			return nil, fmt.Errorf("expected.yaml: item %s has no match", it.Item)
		}
		for _, m := range it.Match {
			if (m.Subject == "") == (m.Text == "") {
				return nil, fmt.Errorf("expected.yaml: item %s: a match alternative needs exactly one of subject/text", it.Item)
			}
			if m.Text != "" {
				if _, err := regexp.Compile(m.Text); err != nil {
					return nil, fmt.Errorf("expected.yaml: item %s: match text: %w", it.Item, err)
				}
			}
		}
		for _, comp := range it.Components {
			if !validClass(comp.Classification) {
				return nil, fmt.Errorf("expected.yaml: item %s: component %s: unknown classification %q", it.Item, comp.Ref, comp.Classification)
			}
		}
	}
	return c, nil
}

func validClass(c domain.FindingClass) bool {
	for _, k := range domain.FindingClasses {
		if c == k {
			return true
		}
	}
	return false
}

// DigestBytes is the "sha256:<hex>" digest of b.
func DigestBytes(b []byte) string { return digestBytes(b) }

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// treeDigest digests a directory's files (relative path + content), sorted.
func treeDigest(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%s\x00", filepath.ToSlash(rel), digestBytes(b))
		return nil
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// InvGate is one pre-registered gate.
type InvGate struct {
	Min       *float64 `yaml:"min" json:"min,omitempty"`
	Max       *float64 `yaml:"max" json:"max,omitempty"`
	MinSample int      `yaml:"minSample" json:"minSample"`
}

// InvGates is eval/investigations/gates.yaml.
type InvGates struct {
	Version  int                `yaml:"version" json:"version"`
	Gates    map[string]InvGate `yaml:"gates" json:"gates"`
	Autonomy struct {
		Requires               []string `yaml:"requires" json:"requires"`
		MinObservedNotAffected int      `yaml:"minObservedNotAffected" json:"minObservedNotAffected"`
		MinObservedAffected    int      `yaml:"minObservedAffected" json:"minObservedAffected"`
	} `yaml:"autonomy" json:"autonomy"`
	Digest string `yaml:"-" json:"digest"`
}

// LoadInvGates reads the frozen gates file. Every gate must name exactly one
// of min/max and a metric this harness computes: a gate that silently never
// evaluates would be a silent pass.
func LoadInvGates(path string) (*InvGates, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g InvGates
	if err := yaml.Unmarshal(b, &g); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	g.Digest = digestBytes(b)
	if len(g.Gates) == 0 {
		return nil, fmt.Errorf("%s: no gates", path)
	}
	for name, gate := range g.Gates {
		if (gate.Min == nil) == (gate.Max == nil) {
			return nil, fmt.Errorf("%s: gate %s needs exactly one of min/max", path, name)
		}
		if !knownInvMetric(name) {
			return nil, fmt.Errorf("%s: gate %s names a metric the harness does not compute", path, name)
		}
	}
	for _, r := range g.Autonomy.Requires {
		if _, ok := g.Gates[r]; !ok {
			return nil, fmt.Errorf("%s: autonomy requires unknown gate %s", path, r)
		}
	}
	return &g, nil
}
