package eval

// Loading and validating dataset entries (eval/cases/<id>/case.yaml, format
// in eval/FORMAT.md). Expectations are hand-curated data; nothing here ever
// derives them from pipeline output.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Case is one dataset entry.
type Case struct {
	// Dir is the case directory (absolute or as given).
	Dir string `json:"dir" yaml:"-"`
	ID  string `json:"id"`
	// Product is the definition id in products/.
	Product string `json:"product"`
	From    string `json:"from"`
	To      string `json:"to"`
	// ResearchedAt records when the ground truth was written (informational).
	ResearchedAt string `json:"researchedAt,omitempty" yaml:"researchedAt,omitempty"`
	// Sources lists every upstream document consulted while writing the case.
	Sources []string `json:"sources,omitempty"`

	Expected    []Expected    `json:"expected"`
	NotExpected []NotExpected `json:"notExpected,omitempty" yaml:"notExpected,omitempty"`
	Environment *Environment  `json:"environment,omitempty"`
}

// Expected is one item of ground truth: upgrade work a competent operator
// must know about for this transition.
type Expected struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Kind           string `json:"kind"`
	Importance     string `json:"importance"`
	ActionRequired bool   `json:"actionRequired,omitempty" yaml:"actionRequired,omitempty"`
	// Classification (G9) is the class a correct system should output for
	// this item — action-required, review-required, informational (and,
	// where the fixture genuinely cannot decide, unknown). Optional: legacy
	// cases predate it and simply score on presence. See eval/FORMAT.md and
	// docs/ACTION_CLASSIFICATION.md.
	Classification string     `json:"classification,omitempty"`
	Match          []Matcher  `json:"match"`
	Evidence       []Citation `json:"evidence,omitempty"`
	References     []string   `json:"references,omitempty"`
}

// Citation is an authoritative upstream statement backing an expectation.
type Citation struct {
	URL   string `json:"url"`
	Quote string `json:"quote,omitempty"`
}

// NotExpected names output a tool might produce but that is NOT
// upgrade-relevant: matches are false positives. Classification, when set, is
// the class such output should NOT have carried (usually action-required:
// "must NOT false-alarm") — it feeds the confusion matrix and the
// false-action rate.
type NotExpected struct {
	Title string `json:"title"`
	// Classification, when set, is the class such output should NOT have
	// carried (see Expected.Classification).
	Classification string    `json:"classification,omitempty"`
	Match          []Matcher `json:"match"`
}

// Environment describes the fixture in <case>/environment/ (values.yaml,
// manifests/, crds/, images.txt) plus the environment-side expectations.
type Environment struct {
	Description string `json:"description,omitempty"`
	Kubernetes  string `json:"kubernetes,omitempty"`
	// ExpectedImpact links expected items to this environment: does the
	// operator need to act / review / just know, and why.
	ExpectedImpact []ImpactLink `json:"expectedImpact,omitempty" yaml:"expectedImpact,omitempty"`
	// ExpectedFindings are impact findings the join should emit, matched on
	// the finding vocabulary (subject / rule / classification).
	ExpectedFindings []ExpectedFinding `json:"expectedFindings,omitempty" yaml:"expectedFindings,omitempty"`
	// NotExpectedFindings name findings that must NOT appear.
	NotExpectedFindings []NotExpectedFinding `json:"notExpectedFindings,omitempty" yaml:"notExpectedFindings,omitempty"`
}

// ImpactLink ties an expected item to this environment.
type ImpactLink struct {
	// Expected is the id of an Expected item.
	Expected  string `json:"expected"`
	Relevance string `json:"relevance"` // action-required | review | informational | not-affected (the dataset's ground-truth vocabulary)
	Why       string `json:"why,omitempty"`
}

// ExpectedFinding is one expected impact finding: the matcher plus context.
type ExpectedFinding struct {
	ID    string         `json:"id,omitempty"`
	Match FindingMatcher `json:"match"`
	Why   string         `json:"why,omitempty"`
}

// FindingMatcher recognises an impact finding. All set fields must match.
type FindingMatcher struct {
	Subject        string `json:"subject,omitempty"`        // exact subject of one of the finding's matches
	Rule           string `json:"rule,omitempty"`           // exact join rule, e.g. impact:values-pinned
	Classification string `json:"classification,omitempty"` // action-required | review-required | informational (docs/ACTION_CLASSIFICATION.md)
}

// NotExpectedFinding is a finding shape that must not be produced.
type NotExpectedFinding struct {
	Title string `json:"title"`
	// Classification, when set, is the class such a finding should NOT have
	// carried — it labels the confusion matrix's false-alarm cells.
	Classification string         `json:"classification,omitempty"`
	Match          FindingMatcher `json:"match"`
	Why            string         `json:"why,omitempty"`
}

// Matcher recognises a generated Change as covering a ground-truth item.
// A change matches when ALL given fields match; an expectation is met when
// ANY of its matchers matches (eval/FORMAT.md).
type Matcher struct {
	// Text is a regular expression matched against the change title and
	// detail (case-sensitive unless the pattern carries (?i)).
	Text string `json:"text,omitempty" yaml:"text,omitempty"`
	// Subject must be exactly one of the change's subjects (a values key
	// path, CRD name, API id, image reference, ...).
	Subject string `json:"subject,omitempty" yaml:"subject,omitempty"`
	// Category must equal the change's category.
	Category string `json:"category,omitempty" yaml:"category,omitempty"`
	// Release is a regular expression over the release the change is
	// attributed to (empty for endpoint diffs, which match ^$).
	Release string `json:"release,omitempty" yaml:"release,omitempty"`
	// Evidence is a regular expression matched against the URIs of the
	// change's evidence records.
	Evidence string `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	// Breaking / ActionRequired must equal the change's flags.
	Breaking       *bool `json:"breaking,omitempty" yaml:"breaking,omitempty"`
	ActionRequired *bool `json:"actionRequired,omitempty" yaml:"actionRequired,omitempty"`

	textRe    *regexp.Regexp `json:"-" yaml:"-"`
	releaseRe *regexp.Regexp `json:"-" yaml:"-"`
	evRe      *regexp.Regexp `json:"-" yaml:"-"`
}

// prepare defensively compiles any pattern that is not compiled yet. It is
// redundant for loaded cases (compile ran at load time) but keeps matchers of
// in-code cases (tests) correct instead of silently matching everything. A
// pattern that fails to compile at use time makes the matcher never match.
func (m *Matcher) prepare() {
	if m.Text != "" && m.textRe == nil {
		if re, err := regexp.Compile(m.Text); err == nil {
			m.textRe = re
		}
	}
	if m.Release != "" && m.releaseRe == nil {
		if re, err := regexp.Compile(m.Release); err == nil {
			m.releaseRe = re
		}
	}
	if m.Evidence != "" && m.evRe == nil {
		if re, err := regexp.Compile(m.Evidence); err == nil {
			m.evRe = re
		}
	}
}

// compile prepares the regular expressions; called once at load time so that
// matching is pure and malformed patterns fail loudly.
func (m *Matcher) compile() error {
	for name, s := range map[string]string{"text": m.Text, "release": m.Release, "evidence": m.Evidence} {
		if s == "" {
			continue
		}
		re, err := regexp.Compile(s)
		if err != nil {
			return fmt.Errorf("matcher %s %q: %w", name, s, err)
		}
		switch name {
		case "text":
			m.textRe = re
		case "release":
			m.releaseRe = re
		case "evidence":
			m.evRe = re
		}
	}
	return nil
}

// String renders the matcher for audit output ("text:…|subject:…").
func (m *Matcher) String() string {
	var parts []string
	if m.Text != "" {
		parts = append(parts, "text:"+m.Text)
	}
	if m.Subject != "" {
		parts = append(parts, "subject:"+m.Subject)
	}
	if m.Category != "" {
		parts = append(parts, "category:"+m.Category)
	}
	if m.Release != "" {
		parts = append(parts, "release:"+m.Release)
	}
	if m.Evidence != "" {
		parts = append(parts, "evidence:"+m.Evidence)
	}
	if m.Breaking != nil {
		parts = append(parts, fmt.Sprintf("breaking:%v", *m.Breaking))
	}
	if m.ActionRequired != nil {
		parts = append(parts, fmt.Sprintf("actionRequired:%v", *m.ActionRequired))
	}
	return strings.Join(parts, " & ")
}

// LoadCase reads and validates <dir>/case.yaml (and checks the optional
// environment/ directory it declares).
func LoadCase(dir string) (*Case, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "case.yaml"))
	if err != nil {
		return nil, err
	}
	var c Case
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	c.Dir = dir
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	return &c, nil
}

// Validate enforces the format: vocabulary, unique ids, resolvable
// cross-references, compilable matchers and a non-empty must-find list.
func (c *Case) Validate() error {
	var errs []error
	if c.ID == "" {
		errs = append(errs, errors.New("id is empty"))
	}
	if c.Product == "" {
		errs = append(errs, errors.New("product is empty"))
	}
	if c.From == "" || c.To == "" {
		errs = append(errs, errors.New("from/to are required"))
	}
	if len(c.Expected) == 0 {
		errs = append(errs, errors.New("no expected items: a case without ground truth measures nothing"))
	}
	ids := map[string]bool{}
	for i := range c.Expected {
		e := &c.Expected[i]
		if e.ID == "" || e.Title == "" {
			errs = append(errs, fmt.Errorf("expected[%d]: id and title are required", i))
			continue
		}
		if ids[e.ID] {
			errs = append(errs, fmt.Errorf("expected: duplicate id %s", e.ID))
		}
		ids[e.ID] = true
		if !kinds[e.Kind] {
			errs = append(errs, fmt.Errorf("expected %s: unknown kind %q", e.ID, e.Kind))
		}
		if !importances[e.Importance] {
			errs = append(errs, fmt.Errorf("expected %s: unknown importance %q", e.ID, e.Importance))
		}
		if e.Classification != "" && !classes[e.Classification] {
			errs = append(errs, fmt.Errorf("expected %s: unknown classification %q", e.ID, e.Classification))
		}
		if len(e.Match) == 0 {
			errs = append(errs, fmt.Errorf("expected %s: no matchers (an unmatchable expectation can never be found)", e.ID))
		}
		for j := range e.Match {
			if err := e.Match[j].compile(); err != nil {
				errs = append(errs, fmt.Errorf("expected %s matcher[%d]: %w", e.ID, j, err))
			}
			if e.Match[j].String() == "" {
				errs = append(errs, fmt.Errorf("expected %s matcher[%d]: all fields empty", e.ID, j))
			}
		}
		for _, cit := range e.Evidence {
			if cit.URL == "" {
				errs = append(errs, fmt.Errorf("expected %s: citation without url", e.ID))
			}
		}
	}
	for i := range c.NotExpected {
		ne := &c.NotExpected[i]
		if ne.Title == "" {
			errs = append(errs, fmt.Errorf("notExpected[%d]: title is required", i))
		}
		if ne.Classification != "" && !classes[ne.Classification] {
			errs = append(errs, fmt.Errorf("notExpected %q: unknown classification %q", ne.Title, ne.Classification))
		}
		for j := range ne.Match {
			if err := ne.Match[j].compile(); err != nil {
				errs = append(errs, fmt.Errorf("notExpected %q matcher[%d]: %w", ne.Title, j, err))
			}
			if ne.Match[j].String() == "" {
				errs = append(errs, fmt.Errorf("notExpected %q matcher[%d]: all fields empty", ne.Title, j))
			}
		}
	}
	if c.Environment != nil {
		env := c.Environment
		for i, l := range env.ExpectedImpact {
			if !ids[l.Expected] {
				errs = append(errs, fmt.Errorf("environment.expectedImpact[%d]: unknown expected id %q", i, l.Expected))
			}
			if !relevances[l.Relevance] {
				errs = append(errs, fmt.Errorf("environment.expectedImpact[%d]: unknown relevance %q", i, l.Relevance))
			}
		}
		for i, f := range env.ExpectedFindings {
			if f.Match.Subject == "" && f.Match.Rule == "" && f.Match.Classification == "" {
				errs = append(errs, fmt.Errorf("environment.expectedFindings[%d]: match needs subject, rule or classification", i))
			}
		}
		for i, f := range env.NotExpectedFindings {
			if f.Title == "" || (f.Match.Subject == "" && f.Match.Rule == "" && f.Match.Classification == "") {
				errs = append(errs, fmt.Errorf("environment.notExpectedFindings[%d]: title and a matcher are required", i))
			}
		}
		if !c.HasEnvironmentFiles() {
			errs = append(errs, errors.New("environment declared but environment/ holds no input files (values.yaml, manifests/, crds/, images.txt)"))
		}
	}
	return errors.Join(errs...)
}

// EnvironmentDir is the subdirectory of a case that holds environment input
// files (values.yaml, manifests/, crds/, images.txt).
const EnvironmentDir = "environment"

// envFileNames are the optional files/directories of an environment fixture.
var envFileNames = []string{"values.yaml", "manifests", "crds", "images.txt"}

// HasEnvironmentFiles reports whether the case directory actually contains
// environment input files. The environment is only joinable when there is
// something to join against; a case.yaml `environment:` block without files
// is a formatting mistake and fails Validate.
func (c *Case) HasEnvironmentFiles() bool {
	for _, n := range envFileNames {
		if _, err := os.Stat(filepath.Join(c.Dir, EnvironmentDir, n)); err == nil {
			return true
		}
	}
	return false
}

// EnvironmentFiles returns the environment input paths that exist, in a
// stable order (values.yaml, manifests/, crds/, images.txt).
func (c *Case) EnvironmentFiles() []string {
	var out []string
	for _, n := range envFileNames {
		p := filepath.Join(c.Dir, EnvironmentDir, n)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}
