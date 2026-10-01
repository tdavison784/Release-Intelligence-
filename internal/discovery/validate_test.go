package discovery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
)

func TestValidatorPaths(t *testing.T) {
	checker := &fakeChecker{outcome: func(kind, subject, release string) (string, string) {
		switch {
		case subject == "release-notes-docs" && release == "v1.0.1":
			return ingest.OutcomeFail, "404 release-notes-1.0.md"
		case subject == "compatibility":
			return ingest.OutcomeUnverifiable, "host blocked"
		case subject == "certmgr-webhook" && (release == "v1.2.1" || release == "v1.1.2"):
			return ingest.OutcomeFail, "manifest unknown"
		case subject == "advisories" && release == "v1.2.1":
			return ingest.OutcomeFail, "not yet published"
		case subject == "upgrade-guide" && release != "v1.2.0":
			return ingest.OutcomeNotApplicable, "patch release"
		}
		return ingest.OutcomePass, ""
	}}
	res := runFixture(t, certFixture, &Discoverer{Checker: checker}, Request{})
	def, rep := res.Definition, res.Report
	want := []string{"v1.0.1", "v1.1.2", "v1.2.0", "v1.2.1"}
	if checker.calls != 1 || strings.Join(rep.Validation.Releases, ",") != strings.Join(want, ",") {
		t.Fatalf("validation releases %v (calls %d)", rep.Validation.Releases, checker.calls)
	}
	if len(rep.Validation.Releases) < ingest.MinValidations {
		t.Fatal("fewer releases than MinValidations")
	}
	if def.Provenance.Method != "discovery" || strings.Join(def.Provenance.ValidatedReleases, ",") != strings.Join(want, ",") {
		t.Errorf("provenance: %+v", def.Provenance)
	}
	if tags := source(t, def, "tags"); strings.Join(tags.ValidatedAgainst, ",") != strings.Join(want, ",") {
		t.Errorf("validatedAgainst: %v", tags.ValidatedAgainst)
	}
	notes := source(t, def, "release-notes-docs")
	if notes.Availability != ">= 1.1.0" || len(notes.ValidatedAgainst) != 0 || !strings.Contains(notes.Notes, "Availability inferred") {
		t.Errorf("failing only on old releases → availability inferred: %+v", notes)
	}
	compat := source(t, def, "compatibility")
	if !strings.Contains(compat.Notes, "unverifiable") || !strings.Contains(compat.Notes, "host blocked") {
		t.Errorf("unverifiable source kept and marked: %q", compat.Notes)
	}
	up := source(t, def, "upgrade-guide")
	if !strings.Contains(up.Notes, "insufficient") {
		t.Errorf("one pass only → insufficient: %q", up.Notes)
	}
	if _, ok := def.Artifact("certmgr-webhook"); ok {
		t.Error("an artifact failing on several recent releases must be dropped")
	}
	dropped := false
	for _, d := range rep.Dropped {
		if d.Key == "artifact:certmgr-webhook" && d.Origin == OriginDeterministic && strings.Contains(d.Reason, "manifest unknown") {
			dropped = true
		}
	}
	if !dropped {
		t.Errorf("dropped element should be reported: %+v", rep.Dropped)
	}
	adv := source(t, def, "advisories")
	if !strings.Contains(adv.Notes, "not yet published") || len(adv.ValidatedAgainst) != 0 {
		t.Errorf("a single failure on the newest release is kept with a comment: %+v", adv)
	}
	for _, rule := range []string{"validate.availability-inferred", "validate.failed", "validate.unverifiable", "validate.insufficient", "validate.single-failure-kept"} {
		if !hasDecision(rep, rule) {
			t.Errorf("missing decision %s", rule)
		}
	}
	if v := rep.Validation.Verdicts; v["source:tags"] != VerdictValidated || v["source:compatibility"] != VerdictUnverifiable || v["artifact:certmgr-webhook"] != VerdictFailing {
		t.Errorf("verdicts: %v", v)
	}
	md := rep.Markdown()
	if !strings.Contains(md, "## Validation matrix") || !strings.Contains(md, "unverifiable: host blocked") {
		t.Error("markdown should render the validation matrix")
	}
	if !catalog.Validate(def).OK() {
		t.Error("definition invalid after validation")
	}
}

type errChecker struct{}

func (errChecker) CheckRelationships(context.Context, *catalog.ProductDefinition, []domain.Version, *ingest.VersionList) (*ingest.RelationshipReport, error) {
	return nil, ingest.ErrNotImplemented
}

func TestValidatorCheckerError(t *testing.T) {
	res := runFixture(t, certFixture, &Discoverer{Checker: errChecker{}}, Request{})
	v := res.Report.Validation
	if v.Status != ValidationError || !strings.Contains(v.Detail, "not implemented") {
		t.Errorf("validation: %+v", v)
	}
	if len(res.Definition.Provenance.ValidatedReleases) != 0 || !strings.Contains(res.Definition.Provenance.Notes, "not implemented") {
		t.Errorf("provenance must say the definition is not validated: %+v", res.Definition.Provenance)
	}
	if _, ok := res.Definition.Artifact("certmgr-webhook"); !ok {
		t.Error("deterministic elements are kept when validation cannot run")
	}
}

func TestValidatorTooFewReleases(t *testing.T) {
	repo, _ := ParseRepo("example/x")
	ta := AnalyzeTags(repo, []RemoteTag{{Name: "v1.0.0"}, {Name: "v1.0.1"}})
	d := &Draft{Definition: &catalog.ProductDefinition{ID: "x"}}
	vr := (&Validator{Checker: &fakeChecker{}}).Validate(context.Background(), d, ta)
	if vr.Status != ValidationSkipped || !strings.Contains(vr.Detail, "required") {
		t.Errorf("got %+v", vr)
	}
	if vr := (&Validator{}).Validate(context.Background(), d, ta); vr.Status != ValidationSkipped {
		t.Errorf("no checker: %+v", vr)
	}
	if errors.Is(nil, ingest.ErrNotImplemented) {
		t.Fatal("unreachable")
	}
}

func TestSelectValidationReleases(t *testing.T) {
	ta := argoFixture.tagAnalysis(t)
	var got []string
	for _, v := range ta.SelectValidationReleases(4) {
		got = append(got, v.Tag)
	}
	// newest patch of each of the newest lines + X.Y.0 of the newest line; no prereleases
	if strings.Join(got, ",") != "v2.9.5,v3.0.3,v3.1.0,v3.1.2" {
		t.Errorf("got %v", got)
	}
}
