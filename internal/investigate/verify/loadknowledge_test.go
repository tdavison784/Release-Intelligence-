package verify

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
)

// Lane join5 (D2): a fact's observed and role-membership defaults are usable
// only through the confirmed proof that records them. The knowledge the
// investigation, `ri impact` and `ri eval` evaluate is read by
// app.LoadKnowledge from the files on disk, so the proofs must be read with
// it; before the fix they were not, impact.VerifyFacts found no proof, and
// every such default was ignored ("does not resolve"): the join and the pin
// scope then left the objects it decides undecidable.

// promotedMembershipStore runs the whole path on the synthetic Badge product:
// a confirmed proof with a role-membership default, promoted by a human.
func promotedMembershipStore(t *testing.T) *proofStore {
	t.Helper()
	s := newProofStore(t, refinedRule(nil))
	o := s.opts(nil, t0.Add(time.Hour))
	stop := map[string]shape{probe1u: refused, probe1o: refused}
	o.Runner = formationRunner{build: func(spec runtime.FixtureSpec) *runtime.FixtureResult { return observedRun(spec, probeOutcomes, stop) }}
	rs, err := Hypotheses(context.Background(), o)
	if err != nil || len(rs) != 1 || rs[0].Proof == nil || rs[0].Proof.Outcome != domain.ProofConfirmed || len(rs[0].Proof.RoleMembershipDefaults) != 1 {
		t.Fatalf("confirmed proof with a membership default expected: %+v %v", rs, err)
	}
	if err := s.promote(t, t0.Add(2*time.Hour)); err != nil {
		t.Fatalf("promotion refused: %v", err)
	}
	return s
}

// badgeEnv is an environment with one Badge that leaves the role field
// (spec.signer) and keyPolicy unset.
func badgeEnv(t *testing.T) *env.Environment {
	t.Helper()
	p := filepath.Join(t.TempDir(), "badge.yaml")
	doc := "apiVersion: acme.example/v1\nkind: Badge\nmetadata:\n  name: plain\n  namespace: ns\nspec:\n  size: 2\n"
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := env.Load(env.Inputs{Manifests: []string{p}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func loadedFact(t *testing.T, dir string, rule domain.SemanticAssertion) (domain.VerifiedFact, []string) {
	t.Helper()
	ks, err := app.LoadKnowledge(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ks.Facts {
		if f.Assertion.Digest() == rule.Digest() {
			return f, ks.Warnings
		}
	}
	t.Fatalf("the promoted fact was not loaded (warnings %v)", ks.Warnings)
	return domain.VerifiedFact{}, nil
}

func exposureOf(f domain.VerifiedFact, e *env.Environment) impact.ConditionResult {
	opts := impact.ConditionOptions{Evidence: f.Evidence}
	opts.Observed, _ = f.UsableObservedDefaults()
	opts.Membership, _ = f.UsableRoleMembershipDefaults()
	return impact.EvaluateConditionWith(f.Assertion.Applicability.Exposure, e, nil, opts)
}

func TestMembershipDefaultSurvivesLoadKnowledge(t *testing.T) {
	s := promotedMembershipStore(t)
	rule := refinedRule(nil)

	f, warnings := loadedFact(t, s.dir, rule)
	if len(f.RoleMembershipDefaults) != 1 || f.RoleMembershipDefaults[0].Proof == "" {
		t.Fatalf("the loaded fact lost its role-membership default: %+v (warnings %v)", f.RoleMembershipDefaults, warnings)
	}
	if w := strings.Join(warnings, "|"); strings.Contains(w, "role-membership default") {
		t.Fatalf("the membership default was refused: %s", w)
	}
	// the join decides the object that leaves the role field unset: outside
	// the role, so not exposed
	r := exposureOf(f, badgeEnv(t))
	if r.Value != impact.False {
		t.Fatalf("exposure %s (%v), want false through the membership default", r.Value, r.Needed)
	}

	// without its confirmed proof on disk the default is ignored (and said
	// so), the fact stays, and the object is undecidable again
	if err := filepath.WalkDir(s.dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && d.Name() == "proofs" {
			if err := os.RemoveAll(p); err != nil {
				return err
			}
			return filepath.SkipDir
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f, warnings = loadedFact(t, s.dir, rule)
	if len(f.RoleMembershipDefaults) != 0 || !strings.Contains(strings.Join(warnings, "|"), "does not resolve") {
		t.Fatalf("without the proof: defaults %+v, warnings %v", f.RoleMembershipDefaults, warnings)
	}
	if r := exposureOf(f, badgeEnv(t)); r.Value != impact.Unknown {
		t.Fatalf("exposure without the proof: %s, want unknown", r.Value)
	}
}

// The same holds for a PD-5 observed default (lane verify3).
func TestObservedDefaultSurvivesLoadKnowledge(t *testing.T) {
	s := newProofStore(t, refinedRule(nil))
	p := s.verify(t, refinementRun(probesFalse, nil), t0.Add(time.Hour))
	if p.Outcome != domain.ProofConfirmed || len(p.ObservedDefaults) != 1 {
		t.Fatalf("confirmed: %s %+v", p.Outcome, p.ObservedDefaults)
	}
	if err := s.promote(t, t0.Add(2*time.Hour)); err != nil {
		t.Fatalf("promotion refused: %v", err)
	}
	f, warnings := loadedFact(t, s.dir, refinedRule(nil))
	if len(f.ObservedDefaults) != 1 || strings.Contains(strings.Join(warnings, "|"), "observed default") {
		t.Fatalf("the loaded fact lost its observed default: %+v (warnings %v)", f.ObservedDefaults, warnings)
	}
	if r := exposureOf(f, badgeEnv(t)); r.Value != impact.False {
		t.Fatalf("exposure %s (%v), want false through the observed default", r.Value, r.Needed)
	}
}
