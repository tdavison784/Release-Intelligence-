package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Phase 5 contract additions (docs/phase5/learning-loop/PROTOCOL.md §8, lane
// verdict): the agent's independent provisional verdict per investigated item
// and the structured mitigation a hypothesis may carry. Both are AI output.
// Neither is ever an input of the classification policy: verdicts are stored
// beside the policy's findings so the two can be compared, and a mitigation
// reaches policy only after independent verification and a human promotion
// turn it into a trusted knowledge fact.

// AgentVerdictIDPrefix prefixes agent verdict ids. CONTRACT-CHANGE(verdict): additive.
const AgentVerdictIDPrefix = "avd-"

// AgentVerdict is the agent's provisional impact assessment of one change
// item (MISSION §2). Class uses the four finding classes so it can be scored
// against the same expectations as the policy, but it is never a finding: the
// agent may say not-affected where the policy cannot. Prose fields are
// unverified AI text (StatementKind); Citations, VerifiedFacts and
// Mitigations are the parts the tool layer resolved — whatever did not
// resolve was dropped and counted. CONTRACT-CHANGE(verdict): additive.
type AgentVerdict struct {
	ID         string       `json:"id"` // AgentRecordID(AgentVerdictIDPrefix, item, class‖mechanism, callId)
	ChangeItem string       `json:"changeItem"`
	Class      FindingClass `json:"class"`
	// Confidence is the agent's own; never a policy input.
	Confidence           Confidence `json:"confidence"`
	Mechanism            string     `json:"mechanism"`
	EnvironmentReasoning string     `json:"environmentReasoning"`
	// Citations are checked citations (evidence ids in the run pool).
	Citations []EvidenceID `json:"citations,omitempty"`
	// CitationsFromEntry: the verdict stated no citations, so Citations are
	// its closing entry's checked citations (prompt v7 onwards).
	// CONTRACT-CHANGE(fixes5): additive.
	CitationsFromEntry bool `json:"citationsFromEntry,omitempty"`
	// VerifiedFacts are the ids of verified setting claims (clm-…) and
	// validation records (vr-…) of this run the verdict rests on.
	VerifiedFacts []string `json:"verifiedFacts,omitempty"`
	// Assumptions are what the agent inferred without verification.
	Assumptions []string `json:"assumptions,omitempty"`
	// Mitigations are hypothesis ids (hyp-…) of this run, usually ones
	// carrying a MitigationSpec.
	Mitigations         []string `json:"mitigations,omitempty"`
	Uncertainty         string   `json:"uncertainty,omitempty"`
	ProposedValidations []string `json:"proposedValidations,omitempty"`
	WouldOverturn       string   `json:"wouldOverturn"`
	// DroppedCitations / DroppedVerifiedFacts count what did not resolve;
	// Dropped says why (bounded).
	DroppedCitations     int      `json:"droppedCitations,omitempty"`
	DroppedVerifiedFacts int      `json:"droppedVerifiedFacts,omitempty"`
	Dropped              []string `json:"dropped,omitempty"`
	// Dossier is the item-dossier artifact the item's session started from.
	Dossier       string     `json:"dossier,omitempty"`
	StatementKind string     `json:"statementKind,omitempty"` // StatementUnverifiedProse
	Provenance    Provenance `json:"provenance"`              // method ai
}

// AgentVerdictID derives a verdict id.
func AgentVerdictID(changeItem string, class FindingClass, mechanism, callID string) string {
	return AgentRecordID(AgentVerdictIDPrefix, changeItem, string(class)+"\x00"+mechanism, callID)
}

// Validate checks the verdict's shape.
func (v AgentVerdict) Validate() error {
	errs := validateAI("agent verdict "+v.ID, v.Provenance)
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("agent verdict %s: %s", v.ID, fmt.Sprintf(format, args...)))
	}
	if v.ID != AgentVerdictID(v.ChangeItem, v.Class, v.Mechanism, v.Provenance.CallID) {
		bad("id is not derived")
	}
	if !strings.HasPrefix(v.ChangeItem, ChangeItemIDPrefix) {
		bad("changeItem is required")
	}
	if !oneOf(v.Class, FindingClasses) {
		bad("unknown class %q", v.Class)
	}
	switch v.Confidence {
	case ConfidenceLow, ConfidenceMedium, ConfidenceHigh:
	default:
		bad("unknown confidence %q", v.Confidence)
	}
	if strings.TrimSpace(v.Mechanism) == "" || strings.TrimSpace(v.EnvironmentReasoning) == "" || strings.TrimSpace(v.WouldOverturn) == "" {
		bad("mechanism, environmentReasoning and wouldOverturn are required")
	}
	if v.Class != FindingInsufficientEvidence && len(v.Citations) == 0 {
		bad("a %s verdict cites evidence", v.Class)
	}
	for _, id := range v.VerifiedFacts {
		if !strings.HasPrefix(id, SettingClaimIDPrefix) && !strings.HasPrefix(id, ValidationRecordIDPrefix) {
			bad("verified fact %q is neither a setting claim nor a validation record", id)
		}
	}
	for _, id := range v.Mitigations {
		if !strings.HasPrefix(id, HypothesisIDPrefix) {
			bad("mitigation %q is not a hypothesis id", id)
		}
	}
	if v.DroppedCitations < 0 || v.DroppedVerifiedFacts < 0 {
		bad("negative dropped counts")
	}
	return errors.Join(errs...)
}

// MitigationSpec is the structured form of a mitigation hypothesis: a
// proposed release-level rule in the existing fact grammar, the environment
// basis that shows this environment meets the rule's conditions, and the
// verification requested. The tool layer stores it on a Hypothesis only when
// it is well-formed (Validate, plus the bundle checks of the agent's tool
// layer); otherwise the hypothesis is free text and MitigationRejected says
// why. The agent never chooses a pin rule: a spec is exported as a proposal
// and a review item, and becomes knowledge only through verification and a
// human decision. CONTRACT-CHANGE(verdict): additive.
type MitigationSpec struct {
	Item         string       `json:"item"`
	ProposedRule ProposedRule `json:"proposedRule"`
	// EnvironmentBasis are setting claim ids (clm-…), validation record ids
	// (vr-…) or object references the tool layer resolved in the run: the
	// verified evidence of this environment's state with respect to the
	// rule's conditions, whether met or not (commander decision 2026-10-17,
	// W-0029). Non-empty is required. It is not an applicability decision:
	// whether the environment meets the rule is decided per object by the
	// deterministic validator; only the recorder and storage read this field.
	EnvironmentBasis      []string `json:"environmentBasis"`
	RequestedVerification string   `json:"requestedVerification"`
}

// ProposedRule is a SemanticAssertion-shaped rule (all four aspects) with
// its release bounds.
type ProposedRule struct {
	Assertion SemanticAssertion `json:"assertion"`
	Release   ReleaseBounds     `json:"release"`
}

// ReleaseBounds bound the transition a rule is about: From is the last
// version with the old behaviour (exclusive), To the first version known to
// have the new one (inclusive). Both are semver versions.
type ReleaseBounds struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// releaseVersionRe finds the version in a release tag with a prefix
// ("controller-v1.12.0", "chart-4.12.0").
var releaseVersionRe = regexp.MustCompile(`v?(\d+\.\d+(\.\d+)?(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?)$`)

// ParseReleaseVersion parses a version or a release tag that ends in one.
func ParseReleaseVersion(s string) (*semver.Version, error) {
	if v, err := semver.NewVersion(s); err == nil {
		return v, nil
	}
	if m := releaseVersionRe.FindStringSubmatch(s); m != nil {
		return semver.NewVersion(m[1])
	}
	return nil, fmt.Errorf("%q is not a version or a release tag ending in one", s)
}

// Versions parses the bounds.
func (r ReleaseBounds) Versions() (*semver.Version, *semver.Version, error) {
	from, err := ParseReleaseVersion(r.From)
	if err != nil {
		return nil, nil, fmt.Errorf("release bounds: from %q: %v", r.From, err)
	}
	to, err := ParseReleaseVersion(r.To)
	if err != nil {
		return nil, nil, fmt.Errorf("release bounds: to %q: %v", r.To, err)
	}
	if !from.LessThan(to) {
		return nil, nil, fmt.Errorf("release bounds: from %s is not before to %s", r.From, r.To)
	}
	return from, to, nil
}

// Contains reports whether version v lies in (From, To].
func (r ReleaseBounds) Contains(v string) bool {
	from, to, err := r.Versions()
	if err != nil {
		return false
	}
	x, err := ParseReleaseVersion(v)
	if err != nil {
		return false
	}
	return from.LessThan(x) && !to.LessThan(x)
}

// Digest identifies the proposed rule: its release bounds and the assertion
// digest (prose excluded, as for facts).
func (r ProposedRule) Digest() string {
	return ShortHash("proposed-rule", r.Release.From, r.Release.To, r.Assertion.Digest())
}

// Validate checks the spec's shape: a complete, well-formed assertion whose
// applicability is decidable (no undecidable leaf), parsable release bounds,
// a non-empty environment basis and a requested verification.
func (m MitigationSpec) Validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if !strings.HasPrefix(m.Item, ChangeItemIDPrefix) {
		bad("mitigation: item is required")
	}
	if err := m.ProposedRule.Assertion.Validate(true); err != nil {
		bad("mitigation: proposed rule: %v", err)
	}
	if ap := m.ProposedRule.Assertion.Applicability; ap != nil {
		if hasUndecidable(ap.Exposure) || (ap.Overlap != nil && hasUndecidable(*ap.Overlap)) {
			bad("mitigation: proposed rule: an applicability condition is undecidable; a rule must be decidable on an environment")
		}
	}
	if _, _, err := m.ProposedRule.Release.Versions(); err != nil {
		bad("mitigation: %v", err)
	}
	if len(m.EnvironmentBasis) == 0 {
		bad("mitigation: environmentBasis is required (claims, validations or objects showing this environment meets the conditions)")
	}
	if strings.TrimSpace(m.RequestedVerification) == "" {
		bad("mitigation: requestedVerification is required")
	}
	return errors.Join(errs...)
}

func hasUndecidable(c Condition) bool {
	if c.Op == OpUndecidable {
		return true
	}
	for _, x := range c.Of {
		if hasUndecidable(x) {
			return true
		}
	}
	return false
}

// HypothesisRuleKind prefixes the provenance rule of a knowledge proposal
// exported from an agent's mitigation hypothesis.
const HypothesisRuleKind = "hypothesis:"

// HypothesisRule is the provenance rule that links an exported proposal to
// its hypothesis and run: "hypothesis:<hyp-id>@<inv-id>".
func HypothesisRule(hypothesis, investigation string) string {
	return HypothesisRuleKind + hypothesis + "@" + investigation
}

// ParseHypothesisRule reads HypothesisRule back.
func ParseHypothesisRule(rule string) (hypothesis, investigation string, ok bool) {
	rest, ok := strings.CutPrefix(rule, HypothesisRuleKind)
	if !ok {
		return "", "", false
	}
	hypothesis, investigation, ok = strings.Cut(rest, "@")
	if !ok || !strings.HasPrefix(hypothesis, HypothesisIDPrefix) || !strings.HasPrefix(investigation, InvestigationIDPrefix) {
		return "", "", false
	}
	return hypothesis, investigation, true
}
