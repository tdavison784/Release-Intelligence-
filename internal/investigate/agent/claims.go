package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Structured claims (prompt feature claims; commander decision 2026-10-16).
// A fact about the environment — "setting S is absent / present / has value V
// in input X" — is stated with record_claim and verified deterministically
// before it is recorded; a failed verification rejects the claim with the
// contradicting citation. Free text stays, labelled unverified AI prose.

// ClaimVerifier is the deterministic claim check.
const ClaimVerifier = "investigate.claim-check@v1"

// ClaimInputEnvironment names every environment file of the bundle as the input.
const ClaimInputEnvironment = "environment-files"

const (
	maxClaimSetting   = 300
	maxClaimEvidence  = 5
	maxContradictions = 3
)

var (
	identRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// claimMatch is one occurrence of the setting in an input.
type claimMatch struct {
	art   domain.ArtifactRef
	line  int
	text  string
	value any  // the YAML value at a key match (nil for a token match)
	isKey bool // matched as a YAML key (value comparable)
}

// claimInputs resolves the claim's input to the artifacts to check.
func (s *Session) claimInputs(input string) (string, []domain.ArtifactRef, error) {
	if input == ClaimInputEnvironment {
		var out []domain.ArtifactRef
		for _, a := range s.cfg.Bundle.Manifest.Artifacts {
			if a.Origin == domain.OriginEnvironment {
				out = append(out, a)
			}
		}
		if len(out) == 0 {
			return "", nil, rejectf("the bundle has no environment files")
		}
		return ClaimInputEnvironment, out, nil
	}
	envRender := map[string]bool{}
	for _, p := range s.cfg.Bundle.Renders {
		envRender[p.From], envRender[p.To] = true, true
	}
	for _, a := range s.allArtifacts() {
		if a.ID != input && a.Path != input && a.URI != input && a.Path != "env/"+strings.TrimPrefix(input, "./") {
			continue
		}
		if a.Origin != domain.OriginEnvironment && !envRender[a.ID] {
			return "", nil, rejectf("input %s (%s) is not an environment file or an environment render: claims are about the environment", input, a.Path)
		}
		return a.ID, []domain.ArtifactRef{a}, nil
	}
	return "", nil, rejectf("unknown input %q: name an environment file or environment render (artifact id or path), or %q", input, ClaimInputEnvironment)
}

// claimMatches finds every occurrence of the setting in the artifacts: a
// dotted key path from a document root; the key's last segment (or a bare
// identifier) as a key anywhere; and the setting (and that segment) as a
// whole token in the text — so an absence is verified only when the
// setting occurs nowhere in any form.
func (s *Session) claimMatches(ctx context.Context, setting string, arts []domain.ArtifactRef) ([]claimMatch, []string, error) {
	var out []claimMatch
	var checked []string
	path := []string(nil)
	leaf := ""
	switch {
	case valuesKeyRe.MatchString(setting):
		path = strings.Split(setting, ".")
		leaf = path[len(path)-1]
	case identRe.MatchString(setting):
		leaf = setting
	}
	tokens := []string{setting}
	if leaf != "" && leaf != setting {
		tokens = append(tokens, leaf)
	}
	var tokRes []*regexp.Regexp
	for _, t := range tokens {
		tokRes = append(tokRes, regexp.MustCompile(`(^|[^A-Za-z0-9_.-])`+regexp.QuoteMeta(t)+`($|[^A-Za-z0-9_-])`))
	}
	for _, a := range arts {
		b, err := s.bytesOf(ctx, a)
		if err != nil {
			return nil, nil, err
		}
		if isBinary(b) {
			continue
		}
		checked = append(checked, a.ID)
		lines := strings.Split(string(b), "\n")
		seen := map[int]bool{}
		add := func(m claimMatch) {
			if m.line < 1 || m.line > len(lines) || seen[m.line] {
				return
			}
			seen[m.line] = true
			m.art, m.text = a, strings.TrimSpace(lines[m.line-1])
			out = append(out, m)
		}
		if leaf != "" {
			dec := yaml.NewDecoder(strings.NewReader(string(b)))
			for {
				var doc yaml.Node
				if err := dec.Decode(&doc); err != nil {
					break
				}
				root := &doc
				if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
					root = root.Content[0]
				}
				if path != nil {
					if k, v := yamlAt(root, path); k != nil {
						add(claimMatch{line: k.Line, value: decodeNode(v), isKey: true})
					}
				}
				walkKeys(root, func(k, v *yaml.Node) {
					if k.Value == leaf {
						add(claimMatch{line: k.Line, value: decodeNode(v), isKey: path == nil})
					}
				})
			}
		}
		for i, l := range lines {
			for _, re := range tokRes {
				if re.MatchString(l) {
					add(claimMatch{line: i + 1})
					break
				}
			}
		}
	}
	sort.Strings(checked)
	return out, checked, nil
}

func yamlAt(n *yaml.Node, path []string) (*yaml.Node, *yaml.Node) {
	var key *yaml.Node
	for _, seg := range path {
		if n == nil || n.Kind != yaml.MappingNode {
			return nil, nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == seg {
				key, next = n.Content[i], n.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil, nil
		}
		n = next
	}
	return key, n
}

func walkKeys(n *yaml.Node, f func(k, v *yaml.Node)) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			f(n.Content[i], n.Content[i+1])
		}
	}
	for _, c := range n.Content {
		walkKeys(c, f)
	}
}

func decodeNode(n *yaml.Node) any {
	var v any
	if n == nil || n.Decode(&v) != nil {
		return nil
	}
	return v
}

// canonical renders a decoded YAML/JSON value for comparison.
func canonical(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func (s *Session) recordClaim(ctx context.Context, in investigate.ClaimInput) (*outcome, error) {
	if err := s.writable(); err != nil {
		return nil, err
	}
	var item domain.ChangeItem
	if in.ChangeItem != "" {
		it, err := s.item(in.ChangeItem)
		if err != nil {
			return nil, err
		}
		item = it
	}
	setting := strings.TrimSpace(in.Setting)
	switch {
	case setting == "" || len(setting) > maxClaimSetting || strings.ContainsAny(setting, "\n\t"):
		return nil, rejectf("setting must be a key path or a token (1..%d bytes, one line)", maxClaimSetting)
	case in.Kind != domain.ClaimAbsent && in.Kind != domain.ClaimPresent && in.Kind != domain.ClaimValue:
		return nil, rejectf("kind %q: want absent, present or value", in.Kind)
	case (in.Kind == domain.ClaimValue) != (strings.TrimSpace(in.Value) != ""):
		return nil, rejectf("a value is given exactly for kind value")
	}
	inputID, arts, err := s.claimInputs(strings.TrimSpace(in.Input))
	if err != nil {
		return nil, err
	}
	matches, checked, err := s.claimMatches(ctx, setting, arts)
	if err != nil {
		return nil, err
	}
	if len(checked) == 0 {
		return nil, rejectf("input %s has no text artifact to check", in.Input)
	}
	cite := func(ms []claimMatch) string {
		var parts []string
		for i, m := range ms {
			if i == maxContradictions {
				parts = append(parts, fmt.Sprintf("… %d more", len(ms)-i))
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%s) L%d: %q", m.art.ID, m.art.Path, m.line, clip(m.text, 160)))
		}
		return strings.Join(parts, "; ")
	}
	var value string
	var support []claimMatch
	var detail string
	switch in.Kind {
	case domain.ClaimAbsent:
		if len(matches) > 0 {
			return nil, rejectf("claim not verified: %s occurs in the input — %s", setting, cite(matches))
		}
		detail = fmt.Sprintf("absent: %s occurs in none of %d artifact(s) — not as a key path, not as its last key anywhere, not as a token", setting, len(checked))
	case domain.ClaimPresent:
		if len(matches) == 0 {
			return nil, rejectf("claim not verified: %s occurs nowhere in the %d artifact(s) checked (%s)", setting, len(checked), strings.Join(checked, ", "))
		}
		support = matches
		detail = fmt.Sprintf("present: %s occurs %d time(s)", setting, len(matches))
	case domain.ClaimValue:
		var want any
		if err := yaml.Unmarshal([]byte(in.Value), &want); err != nil {
			return nil, rejectf("value %q is not a YAML/JSON scalar: %v", in.Value, err)
		}
		value = canonical(want)
		var keyed, differ []claimMatch
		for _, m := range matches {
			if !m.isKey {
				continue
			}
			keyed = append(keyed, m)
			if canonical(m.value) != value {
				differ = append(differ, m)
			}
		}
		switch {
		case len(keyed) == 0:
			return nil, rejectf("claim not verified: no key %s with a value in the input (a value claim needs a YAML key path)%s", setting, func() string {
				if len(matches) > 0 {
					return "; it occurs as text at " + cite(matches)
				}
				return ""
			}())
		case len(differ) > 0:
			return nil, rejectf("claim not verified: %s has another value — %s", setting, cite(differ))
		}
		support = keyed
		detail = fmt.Sprintf("value: every key %s in the input (%d) has the value %s", setting, len(keyed), value)
	}
	var evs []domain.Evidence
	for i, m := range support {
		if i == maxClaimEvidence {
			break
		}
		evs = append(evs, domain.NewEvidence(evidenceKind(m.art), m.art.ID, m.art.URI, fmt.Sprintf("L%d", m.line), m.text, m.art.Digest, time.Time{}))
	}
	id := domain.SettingClaimID(item.ID, in.Kind, setting, inputID, value, s.call.ID)
	for i := range s.claims {
		if s.claims[i].ID == id {
			c := s.claims[i]
			return &outcome{value: &c, view: claimView(c)}, nil
		}
	}
	inputsEv := evidenceIDs(evs)
	if item.ID != "" {
		inputsEv = append(inputsEv, item.Evidence...)
	} else if len(inputsEv) == 0 {
		inputsEv = s.taskEvidence()
	}
	c := domain.SettingClaim{ID: id, ChangeItem: item.ID, Kind: in.Kind, Setting: setting, Input: inputID, Value: value,
		Verifier: ClaimVerifier, Checked: checked, Evidence: evidenceIDs(evs), Detail: detail, Provenance: s.aiProvenance(inputsEv)}
	if err := invalid(func() error { x := c; x.Provenance = placeholder(x.Provenance); return x.Validate() }()); err != nil {
		return nil, err
	}
	return &outcome{value: &c, view: claimView(c), commit: func() {
		s.claims = append(s.claims, c)
		for _, e := range evs {
			s.addRunEvidence(e)
		}
	}}, nil
}

type claimViewT struct {
	ID       string              `json:"id"`
	Verified bool                `json:"verified"`
	Kind     domain.ClaimKind    `json:"kind"`
	Detail   string              `json:"detail"`
	Evidence []domain.EvidenceID `json:"evidence,omitempty"`
	Checked  []string            `json:"checked"`
}

func claimView(c domain.SettingClaim) claimViewT {
	return claimViewT{ID: c.ID, Verified: true, Kind: c.Kind, Detail: c.Detail, Evidence: c.Evidence, Checked: c.Checked}
}

// claimRefs checks the claim ids a record names: verified claims of this
// session, about the record's item or about no item.
func (s *Session) claimRefs(item string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if !s.cfg.Prompt.Features.Claims {
		return nil, rejectf("this prompt version (%s) has no setting claims", s.cfg.Prompt.Version)
	}
	out := uniqSorted(ids)
	for _, id := range out {
		found := false
		for _, c := range s.claims {
			if c.ID == id {
				found = true
				if c.ChangeItem != "" && c.ChangeItem != item {
					return nil, rejectf("claim %s is about change item %s, not %s", id, c.ChangeItem, item)
				}
			}
		}
		if !found {
			return nil, rejectf("unknown claim %q (record it with record_claim first)", id)
		}
	}
	return out, nil
}

// proseKind labels agent free text when the prompt version has claims.
func (s *Session) proseKind() string {
	if s.cfg.Prompt.Features.Claims {
		return domain.StatementUnverifiedProse
	}
	return ""
}
