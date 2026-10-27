package impact

import (
	"fmt"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// A synthetic kind (Badge) and its CRD: spec.mode and spec.key.policy carry
// schema defaults, spec.signer and spec.ports[].tag do not.
const defaultBadges = `apiVersion: acme.example/v1
kind: Badge
metadata: {name: plain, namespace: ns}
spec:
  secretName: plain-tls
---
apiVersion: acme.example/v1
kind: Badge
metadata: {name: signer, namespace: ns}
spec:
  secretName: signer-tls
  signer: true
  key: {policy: Renew}
---
apiVersion: acme.example/v1
kind: Badge
metadata: {name: withkey, namespace: ns}
spec:
  secretName: withkey-tls
  key: {size: 2048}
  ports:
  - {name: a, protocol: UDP, tag: x}
  - {name: b}
`

const defaultBadgeCRD = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: badges.acme.example}
spec:
  group: acme.example
  names: {kind: Badge, plural: badges}
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              secretName: {type: string}
              signer: {type: boolean}
              mode: {type: string, default: Strict}
              key:
                type: object
                properties:
                  size: {type: integer}
                  policy: {type: string, default: Keep}
              ports:
                type: array
                items:
                  type: object
                  properties:
                    name: {type: string}
                    tag: {type: string}
                    protocol: {type: string, default: TCP}
`

func badgeEnv(t *testing.T, withCRD bool) *env.Environment {
	t.Helper()
	extra := map[string]string{"badges.yaml": defaultBadges}
	if withCRD {
		extra["badge-crd.yaml"] = defaultBadgeCRD
	}
	return condEnv(t, "", extra)
}

func badgeScope(name string, of ...domain.Condition) domain.Condition {
	return domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Badge", Name: name, Of: of}
}

// upstreamDoc is release-level upstream evidence (the rule's own evidence).
var upstreamDoc = domain.NewEvidence(domain.EvidenceDocument, "src", "https://example.test/acme/v2.0.0/types.go", "L10",
	"Signer defaults to false when omitted.", "sha256:abc", time.Time{})

// localFile is environment evidence: never a default's citation.
var localFile = domain.NewEvidence(domain.EvidenceLocalFile, "", "manifests/badges.yaml", "L5", "signer: true", "sha256:def", time.Time{})

func withDefault(c domain.Condition, value string, source domain.DefaultSource, cites ...domain.EvidenceID) domain.Condition {
	c.Default = &domain.FieldDefault{Value: value, Source: source, Citations: cites}
	return c
}

func TestUnsetValuePredicates(t *testing.T) {
	withCRD, noCRD := badgeEnv(t, true), badgeEnv(t, false)
	edge := newEdge().edge
	pool := []domain.Evidence{upstreamDoc, localFile}
	signer := func(st domain.FieldState, v string) domain.Condition { return field("spec.signer", st, v) }
	cases := []struct {
		name   string
		e      *env.Environment
		c      domain.Condition
		want   Truth
		reason domain.UnknownReason
	}{
		// no default: undecidable, never false
		{"unset, no default", noCRD, badgeScope("plain", signer(domain.StateEquals, "true")), Unknown, domain.UnknownSemanticAmbiguity},
		{"unset, no default, not-equals", noCRD, badgeScope("plain", signer(domain.StateNotEquals, "true")), Unknown, domain.UnknownSemanticAmbiguity},
		{"unset, the CRD states no default", withCRD, badgeScope("plain", signer(domain.StateEquals, "true")), Unknown, domain.UnknownSemanticAmbiguity},
		{"unset, matches", noCRD, badgeScope("plain", domain.Condition{Op: domain.OpField, Path: "spec.signer", State: domain.StateMatches, Pattern: "^t"}), Unknown, domain.UnknownSemanticAmbiguity},
		{"several objects: one undecidable, none true", noCRD, badgeScope("", signer(domain.StateEquals, "false")), Unknown, domain.UnknownSemanticAmbiguity},
		{"several objects: one true decides", noCRD, badgeScope("", signer(domain.StateEquals, "true")), True, ""},
		{"not of an undecidable leaf stays undecidable", noCRD, badgeScope("plain", domain.Condition{Op: domain.OpNot, Of: []domain.Condition{signer(domain.StateEquals, "true")}}), Unknown, domain.UnknownSemanticAmbiguity},
		// an evidenced rule default decides
		{"rule default holds", noCRD, badgeScope("plain", withDefault(signer(domain.StateEquals, "false"), "false", domain.DefaultController, upstreamDoc.ID)), True, ""},
		{"rule default does not hold", noCRD, badgeScope("plain", withDefault(signer(domain.StateEquals, "true"), "false", domain.DefaultController, upstreamDoc.ID)), False, ""},
		{"rule default, not-equals", noCRD, badgeScope("plain", withDefault(signer(domain.StateNotEquals, "true"), "false", domain.DefaultController, upstreamDoc.ID)), True, ""},
		{"rule default decides a CRD field without a schema default", withCRD, badgeScope("plain", withDefault(signer(domain.StateEquals, "true"), "false", domain.DefaultController, upstreamDoc.ID)), False, ""},
		{"rule default: every object decided", noCRD, badgeScope("", withDefault(signer(domain.StateEquals, "false"), "false", domain.DefaultController, upstreamDoc.ID)), True, ""},
		// an unevidenced rule default is ignored
		{"rule default citing nothing in the pool", noCRD, badgeScope("plain", withDefault(signer(domain.StateEquals, "true"), "false", domain.DefaultController, "ev-000000000000")), Unknown, domain.UnknownSemanticAmbiguity},
		{"rule default citing environment evidence", noCRD, badgeScope("plain", withDefault(signer(domain.StateEquals, "true"), "false", domain.DefaultController, localFile.ID)), Unknown, domain.UnknownSemanticAmbiguity},
		// schema defaults
		{"schema default holds", withCRD, badgeScope("plain", field("spec.mode", domain.StateEquals, `"Strict"`)), True, ""},
		{"schema default does not hold", withCRD, badgeScope("plain", field("spec.mode", domain.StateEquals, `"Lax"`)), False, ""},
		{"schema default needs the CRD", noCRD, badgeScope("plain", field("spec.mode", domain.StateEquals, `"Strict"`)), Unknown, domain.UnknownSemanticAmbiguity},
		{"schema and rule defaults agree", withCRD, badgeScope("plain", withDefault(field("spec.mode", domain.StateEquals, `"Strict"`), `"Strict"`, domain.DefaultCRDSchema, upstreamDoc.ID)), True, ""},
		{"schema and rule defaults contradict", withCRD, badgeScope("plain", withDefault(field("spec.mode", domain.StateEquals, `"Strict"`), `"Lax"`, domain.DefaultController, upstreamDoc.ID)), Unknown, domain.UnknownSemanticAmbiguity},
		{"schema default inside a present parent", withCRD, badgeScope("withkey", field("spec.key.policy", domain.StateEquals, `"Keep"`)), True, ""},
		{"no schema default without the parent", withCRD, badgeScope("plain", field("spec.key.policy", domain.StateEquals, `"Keep"`)), Unknown, domain.UnknownSemanticAmbiguity},
		{"stated value wins over the default", withCRD, badgeScope("signer", field("spec.key.policy", domain.StateEquals, `"Keep"`)), False, ""},
		// list elements that leave the field unset
		{"list: a hit decides", noCRD, badgeScope("withkey", field("spec.ports[].tag", domain.StateEquals, `"x"`)), True, ""},
		{"list: an unset element without a default", noCRD, badgeScope("withkey", field("spec.ports[].tag", domain.StateEquals, `"y"`)), Unknown, domain.UnknownSemanticAmbiguity},
		{"list: an unset element without a default, not-equals", noCRD, badgeScope("withkey", field("spec.ports[].tag", domain.StateNotEquals, `"y"`)), Unknown, domain.UnknownSemanticAmbiguity},
		{"list: an unset element holds the schema default", withCRD, badgeScope("withkey", field("spec.ports[].protocol", domain.StateEquals, `"TCP"`)), True, ""},
		{"list: the schema default does not hold either", withCRD, badgeScope("withkey", field("spec.ports[].protocol", domain.StateEquals, `"SCTP"`)), False, ""},
		{"list: not-equals false through the default", withCRD, badgeScope("withkey", field("spec.ports[].protocol", domain.StateNotEquals, `"TCP"`)), False, ""},
		// presence is not a value: unchanged
		{"unset state", noCRD, badgeScope("plain", field("spec.signer", domain.StateUnset)), True, ""},
		{"set state", noCRD, badgeScope("plain", field("spec.signer", domain.StateSet)), False, ""},
		// flags and environment variables: a rule default decides
		{"flag unset, no default", noCRD, domain.Condition{Op: domain.OpCLIFlag, Name: "--nope", Component: "controller", State: domain.StateEquals, Values: []string{`"true"`}}, Unknown, domain.UnknownSemanticAmbiguity},
		{"flag unset, rule default", noCRD, withDefault(domain.Condition{Op: domain.OpCLIFlag, Name: "--nope", Component: "controller", State: domain.StateEquals, Values: []string{`"true"`}}, `"false"`, domain.DefaultController, upstreamDoc.ID), False, ""},
		{"env var unset, rule default", noCRD, withDefault(domain.Condition{Op: domain.OpEnvVar, Name: "NOPE", Component: "controller", State: domain.StateEquals, Values: []string{`"on"`}}, `"on"`, domain.DefaultController, upstreamDoc.ID), True, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.c.Validate(); err != nil {
				t.Fatal(err)
			}
			r := EvaluateConditionWith(tc.c, tc.e, edge, ConditionOptions{Evidence: pool})
			if r.Value != tc.want {
				t.Fatalf("value = %s, want %s (reason %s, needed %v)", r.Value, tc.want, r.Reason, r.Needed)
			}
			assertResultShape(t, r, tc.e)
			if tc.want == Unknown && r.Reason != tc.reason {
				t.Errorf("reason = %s, want %s (needed %v)", r.Reason, tc.reason, r.Needed)
			}
		})
	}
}

// No fail-open (mission E1, PLAN-M4 §5): over a corpus of conditions on
// fields without an established default (no rule default, and no schema
// default: the environment has no CRD of the kind, or the CRD states none for
// the field), every result is the result before rule 6 or unknown. In
// particular nothing that was true (exposed) or unknown becomes false (not
// affected): Kleene connectives are monotone in the information order, and
// the only change is false leaves becoming unknown. (A schema default is an
// established default: TestUnsetValuePredicates covers what it decides.)
func TestUnsetRuleNeverFailsOpen(t *testing.T) {
	type input struct {
		e     *env.Environment
		paths []string
	}
	all := []string{"spec.signer", "spec.mode", "spec.key.policy", "spec.ports[].tag", "spec.ports[].protocol", "spec.secretName"}
	envs := map[string]input{
		"no CRD":                      {badgeEnv(t, false), all},
		"CRD, fields without default": {badgeEnv(t, true), []string{"spec.signer", "spec.ports[].tag", "spec.secretName"}},
	}
	edge := newEdge().edge
	changed, total := 0, 0
	for envName, in := range envs {
		for _, c := range noDefaultCorpus(in.paths) {
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			total++
			now := EvaluateCondition(c, in.e, edge)
			legacyUnsetFalse = true
			before := EvaluateCondition(c, in.e, edge)
			legacyUnsetFalse = false
			if now.Value == before.Value {
				continue
			}
			changed++
			if now.Value != Unknown {
				t.Errorf("%s: %s: %s before, %s now (only unknown may replace a decided value)", envName, describe(c), before.Value, now.Value)
			}
		}
	}
	if changed == 0 {
		t.Fatal("the corpus exercises no unset value predicate")
	}
	t.Logf("%d of %d evaluations moved to unknown; none moved toward a decision", changed, total)
}

// noDefaultCorpus builds conditions on Badge fields: every value state and
// presence state of each path, under resource scopes (every Badge and each
// one by name), not, any and all.
func noDefaultCorpus(paths []string) []domain.Condition {
	var leaves []domain.Condition
	for _, p := range paths {
		for _, v := range []string{"true", "false", `"Strict"`, `"Keep"`, `"x"`, `"TCP"`, `"plain-tls"`} {
			leaves = append(leaves, field(p, domain.StateEquals, v), field(p, domain.StateNotEquals, v))
		}
		leaves = append(leaves, domain.Condition{Op: domain.OpField, Path: p, State: domain.StateMatches, Pattern: "^[a-z]"},
			field(p, domain.StateSet), field(p, domain.StateUnset))
	}
	var corpus []domain.Condition
	for _, name := range []string{"", "plain", "signer", "withkey"} {
		for i, l := range leaves {
			corpus = append(corpus,
				badgeScope(name, l),
				badgeScope(name, domain.Condition{Op: domain.OpNot, Of: []domain.Condition{l}}),
				domain.Condition{Op: domain.OpNot, Of: []domain.Condition{badgeScope(name, l)}})
			m := leaves[(i*7+3)%len(leaves)]
			corpus = append(corpus,
				badgeScope(name, l, m),
				badgeScope(name, domain.Condition{Op: domain.OpAny, Of: []domain.Condition{l, m}}),
				domain.Condition{Op: domain.OpAll, Of: []domain.Condition{badgeScope(name, l), domain.Condition{Op: domain.OpNot, Of: []domain.Condition{badgeScope(name, m)}}}})
		}
	}
	return corpus
}

func TestConditionDefaultValidation(t *testing.T) {
	cite := []domain.EvidenceID{upstreamDoc.ID}
	bad := []struct {
		name string
		c    domain.Condition
	}{
		{"on a presence state", badgeScope("", withDefault(field("spec.signer", domain.StateUnset), "false", domain.DefaultController, cite...))},
		{"on a resource", withDefault(badgeScope("", field("spec.signer", domain.StateSet)), "false", domain.DefaultController, cite...)},
		{"without citations", badgeScope("", withDefault(field("spec.signer", domain.StateEquals, "true"), "false", domain.DefaultController))},
		{"value not JSON", badgeScope("", withDefault(field("spec.signer", domain.StateEquals, "true"), "fal se", domain.DefaultController, cite...))},
		{"unknown source", badgeScope("", withDefault(field("spec.signer", domain.StateEquals, "true"), "false", "memory", cite...))},
	}
	for _, tc := range bad {
		if err := tc.c.Validate(); err == nil {
			t.Errorf("%s: validated", tc.name)
		}
	}
	ok := badgeScope("", withDefault(field("spec.signer", domain.StateEquals, "true"), "false", domain.DefaultController, cite...))
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	// an assertion without defaults digests as before (omitempty)
	a := domain.SemanticAssertion{Applicability: &domain.Applicability{Exposure: badgeScope("", field("spec.signer", domain.StateEquals, "true"))}}
	b := a
	b.Applicability = &domain.Applicability{Exposure: badgeScope("", field("spec.signer", domain.StateEquals, "true"))}
	b.Applicability.Exposure.Of[0].Default = nil
	if a.Digest() != b.Digest() {
		t.Fatal("digest depends on a nil default")
	}
	if fmt.Sprint(a.AspectDigest(domain.AspectApplicability)) == fmt.Sprint(domain.SemanticAssertion{Applicability: &domain.Applicability{Exposure: ok}}.AspectDigest(domain.AspectApplicability)) {
		t.Fatal("a stated default does not change the applicability digest")
	}
}
