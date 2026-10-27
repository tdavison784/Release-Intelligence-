package semantic

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestCheck(t *testing.T) {
	const allowedList = "Paths may contain only alphanumeric characters, `.`, `_`, `~`, `/` and `-`."
	const allowedRegex = "var validPathType = regexp.MustCompile(`^[A-Za-z0-9\\-\\._~/]+$`)"
	cases := []struct {
		name     string
		claim    string
		passages []string
		want     Verdict
		why      string // substring of the detail
	}{
		// a character-class claim against a passage listing the allowed characters
		{"character the allowed list lacks", "The validating webhook accepts paths containing `*`.", []string{allowedList}, Unsupported, "`*` in no character set"},
		{"characters the allowed list has", "The validating webhook accepts paths containing `.` and `_`.", []string{allowedList}, Supported, ""},
		{"character the regex class lacks", "With strict validation the webhook accepts `*` in a path.", []string{allowedRegex}, Unsupported, "`*` in no character set"},
		{"rejection of a character outside an exhaustive class", "The webhook rejects paths containing `*`.", []string{allowedRegex}, Supported, ""},
		{"rejection of an allowed character (flip)", "The webhook rejects paths containing `~`.", []string{allowedRegex}, Unsupported, "opposite polarity"},
		{"claimed class wider than the passage's", "Paths may use the characters `[A-Za-z0-9*]`.", []string{allowedRegex}, Unsupported, "`*` in no character set"},
		{"claimed class inside the passage's", "Paths may use the characters `[a-z0-9]`.", []string{allowedRegex}, Supported, ""},
		{"complement claim", "The webhook rejects path characters other than `[A-Za-z0-9._~/-]`.", []string{allowedRegex}, Supported, ""},
		// a token the citation lacks
		{"default the passage lacks", "The rotation policy defaults to `Always`.", []string{"If unset, a default of `Never` is used."}, Unsupported, "does not occur"},
		{"default the passage states", "The rotation policy defaults to `Never`.", []string{"If unset, a default of `Never` is used."}, Supported, ""},
		{"enumerated value added", "pathType must be one of `Exact`, `Prefix` or `Glob`.", []string{"The supported path types are `Exact`, `Prefix` and `ImplementationSpecific`."}, Unsupported, "`Glob`"},
		{"enumerated values agree", "pathType must be one of `Exact` or `Prefix`.", []string{"The supported path types are `Exact`, `Prefix` and `ImplementationSpecific`."}, Supported, ""},
		{"before/after values", "The default changed from `Always` to `Never`.", []string{"The default rotationPolicy changes to `Never`; it was `Always` before."}, Supported, ""},
		{"after value the passage lacks", "The default changed from `Always` to `OnDemand`.", []string{"The default rotationPolicy changes to `Never`; it was `Always` before."}, Unsupported, "`OnDemand`"},
		// polarity flips
		{"accepts vs rejects", "The webhook accepts `ImplementationSpecific` paths.", []string{"The admission webhook rejects `ImplementationSpecific` paths when strict validation is on."}, Unsupported, "opposite polarity"},
		{"rejects vs rejects", "The webhook rejects `ImplementationSpecific` paths.", []string{"The admission webhook rejects `ImplementationSpecific` paths when strict validation is on."}, Supported, ""},
		{"true vs false", "Strict path validation defaults to true.", []string{"--strict-validate-path-type: validate the path type (default false)"}, Unsupported, "opposite"},
		{"enabled vs disabled", "Strict path validation is enabled by default.", []string{"Strict path validation is disabled by default."}, Unsupported, "opposite"},
		{"boolean agrees", "`strict-validate-path-type` defaults to `false`.", []string{"--strict-validate-path-type: validate the path type (default false)"}, Supported, ""},
		{"negated rejection", "The webhook does not reject `Prefix` paths.", []string{"`Prefix` paths are always accepted."}, Supported, ""},
		// undecidable: routed to a human
		{"no value token", "The controller re-validates every existing object on upgrade.", []string{"The webhook validates incoming ingresses."}, Undetermined, "no value token"},
		{"polarity unreadable in the passage", "The webhook accepts `Exact` paths.", []string{"pathType `Exact` matches the URL path exactly."}, Undetermined, "polarity"},
		{"no passage", "The webhook accepts `Exact` paths.", nil, Undetermined, "no cited passage"},
		// setting names are not values
		{"names are not values", "When `pathType` is `Exact` the `nginx.ingress.kubernetes.io/use-regex` annotation is ignored.", []string{"Exact matching is used for pathType `Exact`."}, Supported, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Check(c.claim, c.passages)
			if got.Verdict != c.want || !strings.Contains(got.Detail, c.why) {
				t.Errorf("Check(%q) = %s: %s\n tokens %+v\n want %s (detail containing %q)", c.claim, got.Verdict, got.Detail, got.Tokens, c.want, c.why)
			}
		})
	}
}

func TestExtractTokens(t *testing.T) {
	got := ExtractTokens("When `spec.privateKey.rotationPolicy` is `Never` and `isCA` is true, the key defaults to `RSA`; the flag `--enable-x` accepts `[a-z]`.")
	var s []string
	for _, tk := range got {
		s = append(s, tk.String())
	}
	want := []string{"value `Never`", "default `RSA`", "character-class `[a-z]` (positive)", "boolean `true`"}
	if strings.Join(s, ", ") != strings.Join(want, ", ") {
		t.Errorf("tokens = %v, want %v", s, want)
	}
}

func TestAssertionTokens(t *testing.T) {
	before, after := `"Always"`, `"Never"`
	a := domain.SemanticAssertion{
		Change: &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: &before, After: &after},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
			{Op: domain.OpField, Path: "spec.isCA", State: domain.StateEquals, Values: []string{"true"}},
			{Op: domain.OpField, Path: "spec.usages", State: domain.StateHasToken, Values: []string{"Feature=Strict"}},
		}}},
		Statement: "The default rotation policy becomes `Never`.",
	}
	var s []string
	for _, tk := range AssertionTokens(a) {
		s = append(s, tk.String())
	}
	want := "before `Always`, after `Never`, value `Never`"
	if strings.Join(s, ", ") != want {
		t.Errorf("tokens = %s, want %s", strings.Join(s, ", "), want)
	}
	if r := CheckAssertion(a, []string{"The default rotation policy changes from `Always` to `Never`."}); r.Verdict != Supported {
		t.Errorf("CheckAssertion = %s: %s", r.Verdict, r.Detail)
	}
	if r := CheckAssertion(a, []string{"The default rotation policy changes from `Always` to `OnDemand`."}); r.Verdict != Unsupported || !strings.Contains(r.Detail, "`Never`") {
		t.Errorf("CheckAssertion = %s: %s", r.Verdict, r.Detail)
	}
}

func TestCheckBehaviours(t *testing.T) {
	pool := map[domain.EvidenceID]domain.Evidence{
		"ev-a": {ID: "ev-a", Excerpt: "var validPathType = regexp.MustCompile(`^[A-Za-z0-9\\-\\._~/]+$`)…"},
	}
	as := []domain.AgentAssessment{{ID: "aas-1", ChangeItem: "ci-1", Behaviour: []domain.AssessmentBehaviour{
		{Behaviour: "The webhook accepts `*` in Prefix paths", Source: domain.BehaviourUpstream, Citations: []domain.EvidenceID{"ev-a"}},
		{Behaviour: "The webhook rejects `*`", Source: domain.BehaviourUpstream, Citations: []domain.EvidenceID{"ev-a", "ev-missing"}},
		{Behaviour: "Defaults apply", Source: domain.BehaviourNotChecked},
		{Behaviour: "The webhook accepts `Exact`", Source: domain.BehaviourUpstream, Citations: []domain.EvidenceID{"ev-missing"}},
	}}}
	got := CheckBehaviours(as, pool)
	if len(got) != 3 {
		t.Fatalf("got %d checks, want 3 (upstream only)", len(got))
	}
	for i, want := range []Verdict{Unsupported, Supported, Undetermined} {
		if got[i].Verdict != want {
			t.Errorf("check %d (%s) = %s: %s, want %s", i, got[i].Behaviour, got[i].Verdict, got[i].Detail, want)
		}
	}
	if len(got[1].Unresolved) != 1 || got[1].Unresolved[0] != "ev-missing" {
		t.Errorf("unresolved = %v", got[1].Unresolved)
	}
}
