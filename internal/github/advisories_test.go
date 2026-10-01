package github

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

func TestConvertVersionRange(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: ">= 1.0.0, < 1.17.2", want: ">= 1.0.0, < 1.17.2"},
		{in: "<= 1.2.3", want: "<= 1.2.3"},
		{in: "< 1.2.3", want: "< 1.2.3"},
		{in: "> 1.2.3", want: "> 1.2.3"},
		{in: ">= 1.2.3", want: ">= 1.2.3"},
		{in: "= 1.2.3", want: "= 1.2.3"},
		{in: "== 1.2.3", want: "= 1.2.3"},
		{in: "1.2.3", want: "= 1.2.3"},
		{in: ">=1.0.0,<1.2", want: ">= 1.0.0, < 1.2"},
		{in: "  >= 1.0.0 ,  < 2.0.0  ", want: ">= 1.0.0, < 2.0.0"},
		{in: ">= 0", want: ">= 0"},
		{in: ">= 0, < 1.5.0", want: ">= 0, < 1.5.0"},
		{in: "< v1.2.3", want: "< 1.2.3"},
		{in: ">= 2.0.0-rc.1, < 2.0.3", want: ">= 2.0.0-rc.1, < 2.0.3"},
		{in: ">= 1.0.0-alpha, < 1.0.0", want: ">= 1.0.0-alpha, < 1.0.0"},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: ">= 1.0.0,", wantErr: true},
		{in: "some builds of 1.x", wantErr: true},
		{in: ">= abc", wantErr: true},
		{in: "~> 1.2", wantErr: true},
		{in: "!= 1.2.3", wantErr: true},
		{in: ">= 1.0.0 < 2.0.0", wantErr: true},
		{in: "<>", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ConvertVersionRange(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ConvertVersionRange(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ConvertVersionRange(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
			}
			if _, err := semver.NewConstraint(got); err != nil {
				t.Errorf("result %q does not parse: %v", got, err)
			}
		})
	}
}

func TestConvertedRangesMatchVersions(t *testing.T) {
	tests := []struct {
		ranges []string
		match  map[string]bool
	}{
		{
			ranges: []string{">= 1.0.0, < 1.17.2"},
			match:  map[string]bool{"0.9.9": false, "1.0.0": true, "1.17.1": true, "1.17.2": false, "1.18.0": false},
		},
		{
			ranges: []string{"<= 1.2.3", ">= 1.3.0, < 1.3.5"},
			match:  map[string]bool{"1.2.3": true, "1.2.4": false, "1.3.0": true, "1.3.4": true, "1.3.5": false, "0.1.0": true},
		},
		{
			ranges: []string{"= 3.0.0"},
			match:  map[string]bool{"2.9.9": false, "3.0.0": true, "3.0.1": false},
		},
		{
			ranges: []string{">= 0"},
			match:  map[string]bool{"0.0.1": true, "99.0.0": true},
		},
	}
	for _, tc := range tests {
		c, skipped := CombineVersionRanges(tc.ranges)
		if len(skipped) != 0 || c == "" {
			t.Fatalf("CombineVersionRanges(%v) = %q, %v", tc.ranges, c, skipped)
		}
		cons, err := semver.NewConstraint(c)
		if err != nil {
			t.Fatalf("%q: %v", c, err)
		}
		for v, want := range tc.match {
			if got := cons.Check(semver.MustParse(v)); got != want {
				t.Errorf("%q matches %s = %v, want %v", c, v, got, want)
			}
		}
	}
}

func TestCombineVersionRanges(t *testing.T) {
	tests := []struct {
		name        string
		in          []string
		want        string
		wantSkipped int
	}{
		{"single", []string{">= 1.0.0, < 1.17.2"}, ">= 1.0.0, < 1.17.2", 0},
		{"two ranges are united", []string{"<= 1.2.3", ">=1.3.0,<1.3.5"}, "<= 1.2.3 || >= 1.3.0, < 1.3.5", 0},
		{"duplicates collapse (also after normalisation)", []string{">= 1.0.0, < 1.17.2", ">=1.0.0,<1.17.2", ">= 1.0.0, < 1.17.2"}, ">= 1.0.0, < 1.17.2", 0},
		{"order of first appearance is kept", []string{"< 1.0.0", "> 2.0.0", "< 1.0.0"}, "< 1.0.0 || > 2.0.0", 0},
		{"unconvertible ranges are skipped and reported", []string{"some builds of 1.x", "< 1.0.0", "!= 3"}, "< 1.0.0", 2},
		{"nothing convertible", []string{"garbage"}, "", 1},
		{"empty and blank entries are ignored", []string{"", "  "}, "", 0},
		{"no ranges", nil, "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, skipped := CombineVersionRanges(tc.in)
			if got != tc.want || len(skipped) != tc.wantSkipped {
				t.Errorf("got %q, %d skipped (%v); want %q, %d", got, len(skipped), skipped, tc.want, tc.wantSkipped)
			}
			if got != "" {
				if _, err := semver.NewConstraint(got); err != nil {
					t.Errorf("%q does not parse: %v", got, err)
				}
			}
		})
	}
}

func TestParsePatchedVersions(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"1.17.2", "1.17.2"},
		{"1.16.4, 1.17.2", "1.16.4,1.17.2"},
		{"1.16.4,1.17.2,1.16.4", "1.16.4,1.17.2"},
		{"v2.0.3", "v2.0.3"},
		{"", ""},
		{"see the advisory text", ""},
		{"3.0.1, none, 3.0.2", "3.0.1,3.0.2"},
	}
	for _, tc := range tests {
		if got := strings.Join(ParsePatchedVersions(tc.in), ","); got != tc.want {
			t.Errorf("ParsePatchedVersions(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

const advisoriesURI = "/repos/example/project/security-advisories?per_page=100&state=published"

func advisoriesAPI(t *testing.T) *fakeAPI {
	api := newFakeAPI(t)
	api.serve(advisoriesURI, fixture(t, "advisories_page1.json"),
		`<{base}`+advisoriesURI+`&page=2>; rel="next", <{base}`+advisoriesURI+`&page=2>; rel="last"`)
	api.serve(advisoriesURI+"&page=2", fixture(t, "advisories_page2.json"),
		`<{base}`+advisoriesURI+`&page=1>; rel="prev", <{base}`+advisoriesURI+`&page=1>; rel="first"`)
	return api
}

func advisoryLoc() catalog.Locator {
	return catalog.Locator{Kind: catalog.LocatorGitHubAdvisories, Repository: "example/project"}
}

func TestListAdvisories(t *testing.T) {
	api := advisoriesAPI(t)
	c, _ := newClient(t, api, t.TempDir(), "tok", fetch.ModeOnline)
	advs, evs, err := NewAdvisories(c).ListAdvisories(context.Background(), advisoryLoc())
	if err != nil {
		t.Fatal(err)
	}
	if got := api.paths(); len(got) != 2 || got[0] != advisoriesURI {
		t.Errorf("requests = %v", got)
	}
	if got := api.requests()[0].Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("Authorization = %q", got)
	}

	// The withdrawn advisory is gone; the rest is ordered newest first.
	var ids []string
	for _, a := range advs {
		ids = append(ids, a.ID)
	}
	wantIDs := []string{"GHSA-kkkk-llll-mmmm", "GHSA-aaaa-bbbb-cccc", "GHSA-dddd-eeee-ffff", "GHSA-nnnn-pppp-qqqq"}
	if strings.Join(ids, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("ids = %v, want %v", ids, wantIDs)
	}
	if len(evs) != len(advs) {
		t.Fatalf("%d evidence records for %d advisories", len(evs), len(advs))
	}

	critical, high, medium, bare := advs[0], advs[1], advs[2], advs[3]

	// Fully populated advisory.
	if high.Summary != "Denial of service when parsing crafted input" || high.Severity != "high" ||
		high.URL != "https://github.com/example/project/security/advisories/GHSA-aaaa-bbbb-cccc" ||
		high.SourceID != "" {
		t.Errorf("high = %+v", high)
	}
	if high.PublishedAt == nil || !high.PublishedAt.Equal(time.Date(2025, 3, 5, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("PublishedAt = %v", high.PublishedAt)
	}
	if strings.Join(high.Aliases, ",") != "CVE-2025-00001" {
		t.Errorf("aliases = %v (the GHSA id itself is not an alias)", high.Aliases)
	}
	// Two vulnerable packages with the same range collapse into one.
	if high.Vulnerable != ">= 1.0.0, < 1.17.2" || strings.Join(high.Patched, ",") != "1.17.2" {
		t.Errorf("vulnerable/patched = %q / %v", high.Vulnerable, high.Patched)
	}

	// Several ranges are united; no CVE means no alias.
	if medium.Vulnerable != "<= 1.2.3 || >= 1.3.0, < 1.3.5" || strings.Join(medium.Patched, ",") != "1.2.4,1.3.5" || len(medium.Aliases) != 0 {
		t.Errorf("medium = %+v", medium)
	}

	// Pre-release bound, an exact version, comma separated patched versions and several CVEs.
	if critical.Vulnerable != ">= 2.0.0-rc.1, < 2.0.3 || = 3.0.0" || strings.Join(critical.Patched, ",") != "2.0.3,3.0.1,3.0.2" ||
		strings.Join(critical.Aliases, ",") != "CVE-2025-00003,CVE-2025-00004" || critical.Severity != "critical" {
		t.Errorf("critical = %+v", critical)
	}

	// Nothing machine-readable: no constraint, no patched versions, but still an advisory.
	if bare.Vulnerable != "" || len(bare.Patched) != 0 || bare.ID != "GHSA-nnnn-pppp-qqqq" {
		t.Errorf("bare = %+v", bare)
	}

	// Every constraint parses with Masterminds and matches what it should.
	for _, a := range advs {
		if a.Vulnerable == "" {
			continue
		}
		if _, err := semver.NewConstraint(a.Vulnerable); err != nil {
			t.Errorf("%s: constraint %q does not parse: %v", a.ID, a.Vulnerable, err)
		}
	}
	v := domain.MustVersion("v1.17.1", "1.17.1")
	if ok, err := v.Satisfies(high.Vulnerable); err != nil || !ok {
		t.Errorf("1.17.1 must be vulnerable to %q: %v %v", high.Vulnerable, ok, err)
	}
	v = domain.MustVersion("v1.17.2", "1.17.2")
	if ok, err := v.Satisfies(high.Vulnerable); err != nil || ok {
		t.Errorf("1.17.2 is patched (%q): %v %v", high.Vulnerable, ok, err)
	}
}

// One range that cannot be converted must not leave the others behind as if
// they were the whole story: Vulnerable stays empty and the raw ranges stay in
// the evidence.
func TestConvertAdvisoryPartiallyConvertibleRangesLeaveVulnerableEmpty(t *testing.T) {
	var ga advisory
	ga.GHSAID, ga.Summary, ga.Severity = "GHSA-pppp-aaaa-rrrr", "Partly described", "high"
	for _, r := range []string{">= 1.0.0, < 1.17.2", "some builds of 1.x"} {
		ga.Vulnerabilities = append(ga.Vulnerabilities, struct {
			VulnerableVersionRange string `json:"vulnerable_version_range"`
			PatchedVersions        string `json:"patched_versions"`
		}{r, "1.17.2"})
	}
	adv, ev := convertAdvisory("example", "project", ga, []byte(`{}`), time.Time{})
	if adv.Vulnerable != "" {
		t.Errorf("Vulnerable = %q, want empty when any range is unconvertible", adv.Vulnerable)
	}
	for _, want := range []string{">= 1.0.0, < 1.17.2", "some builds of 1.x", "patched: 1.17.2"} {
		if !strings.Contains(ev.Excerpt, want) {
			t.Errorf("evidence excerpt %q must keep %q", ev.Excerpt, want)
		}
	}
	if strings.Join(adv.Patched, ",") != "1.17.2" || len(adv.Evidence) != 1 || adv.Evidence[0] != ev.ID {
		t.Errorf("patched and evidence are unaffected: %+v", adv)
	}

	// All ranges convertible: unchanged behaviour.
	ga.Vulnerabilities = ga.Vulnerabilities[:1]
	if adv, _ := convertAdvisory("example", "project", ga, []byte(`{}`), time.Time{}); adv.Vulnerable != ">= 1.0.0, < 1.17.2" {
		t.Errorf("Vulnerable = %q", adv.Vulnerable)
	}
}

func TestListAdvisoriesEvidence(t *testing.T) {
	api := advisoriesAPI(t)
	c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline)
	advs, evs, err := NewAdvisories(c).ListAdvisories(context.Background(), advisoryLoc())
	if err != nil {
		t.Fatal(err)
	}
	set := domain.EvidenceSet{}
	set.AddAll(evs)
	if set.Len() != len(evs) {
		t.Fatalf("evidence ids are not unique: %d of %d", set.Len(), len(evs))
	}
	for i, a := range advs {
		if len(a.Evidence) != 1 || a.Evidence[0] != evs[i].ID || !set.Has(a.Evidence[0]) {
			t.Errorf("%s: evidence ids %v do not reference %s", a.ID, a.Evidence, evs[i].ID)
		}
		e := evs[i]
		if e.Kind != domain.EvidenceAdvisory || e.URI != a.URL || e.Locator != a.ID || e.ContentDigest == "" ||
			!strings.HasPrefix(e.ContentDigest, "sha256:") || e.RetrievedAt.IsZero() || e.SourceID != "" {
			t.Errorf("evidence %d = %+v", i, e)
		}
	}
	// The excerpt records the original ranges, including the ones that could not be converted.
	var bareEv, highEv domain.Evidence
	for i, a := range advs {
		switch a.ID {
		case "GHSA-nnnn-pppp-qqqq":
			bareEv = evs[i]
		case "GHSA-aaaa-bbbb-cccc":
			highEv = evs[i]
		}
	}
	if want := "GHSA-aaaa-bbbb-cccc (high): Denial of service when parsing crafted input; vulnerable: >= 1.0.0, < 1.17.2 | >= 1.0.0, < 1.17.2; patched: 1.17.2"; highEv.Excerpt != want {
		t.Errorf("excerpt = %q, want %q", highEv.Excerpt, want)
	}
	if !strings.Contains(bareEv.Excerpt, "some builds of 1.x") {
		t.Errorf("excerpt must keep the unconvertible range: %q", bareEv.Excerpt)
	}
	if highEv.ContentDigest == bareEv.ContentDigest {
		t.Error("digests must differ per advisory")
	}

	// Idempotent: a second run (new client, new cache) yields the same ids.
	c2, _ := newClient(t, advisoriesAPI(t), t.TempDir(), "", fetch.ModeOnline)
	_, evs2, err := NewAdvisories(c2).ListAdvisories(context.Background(), advisoryLoc())
	if err != nil {
		t.Fatal(err)
	}
	for i := range evs {
		if evs[i].ID != evs2[i].ID {
			t.Errorf("evidence id %d changed between runs", i)
		}
	}
}

func TestListAdvisoriesOfflineReplay(t *testing.T) {
	api := advisoriesAPI(t)
	dir := t.TempDir()
	c, _ := newClient(t, api, dir, "", fetch.ModeOnline)
	first, firstEv, err := NewAdvisories(c).ListAdvisories(context.Background(), advisoryLoc())
	if err != nil {
		t.Fatal(err)
	}
	api.Close()
	off, _ := newClient(t, api, dir, "", fetch.ModeOffline)
	again, againEv, err := NewAdvisories(off).ListAdvisories(context.Background(), advisoryLoc())
	if err != nil || len(again) != len(first) {
		t.Fatalf("offline: %d advisories, %v", len(again), err)
	}
	for i := range again {
		if again[i].ID != first[i].ID || againEv[i].ID != firstEv[i].ID || again[i].Vulnerable != first[i].Vulnerable {
			t.Errorf("advisory %d differs after replay", i)
		}
	}
}

func TestListAdvisoriesErrors(t *testing.T) {
	ctx := context.Background()
	api := newFakeAPI(t)
	c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline)
	adv := NewAdvisories(c)

	// Repository without advisories (or not visible): not found.
	if _, _, err := adv.ListAdvisories(ctx, advisoryLoc()); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("404: %v", err)
	}
	if _, _, err := adv.ListAdvisories(ctx, catalog.Locator{Repository: "nope"}); err == nil {
		t.Error("invalid repository: want error")
	}
	// Garbage payloads are errors, not empty results.
	apiObj := newFakeAPI(t)
	apiObj.serve(advisoriesURI, `{"message":"x"}`, "")
	cObj, _ := newClient(t, apiObj, t.TempDir(), "", fetch.ModeOnline)
	if _, _, err := NewAdvisories(cObj).ListAdvisories(ctx, advisoryLoc()); err == nil || errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("object instead of list: %v", err)
	}
	api2 := newFakeAPI(t)
	api2.serve(advisoriesURI, `["not an object"]`, "")
	c2, _ := newClient(t, api2, t.TempDir(), "", fetch.ModeOnline)
	if _, _, err := NewAdvisories(c2).ListAdvisories(ctx, advisoryLoc()); err == nil {
		t.Error("string element: want error")
	}
	// An empty list is a valid, empty answer.
	api3 := newFakeAPI(t)
	api3.serve(advisoriesURI, `[]`, "")
	c3, _ := newClient(t, api3, t.TempDir(), "", fetch.ModeOnline)
	advs, evs, err := NewAdvisories(c3).ListAdvisories(ctx, advisoryLoc())
	if err != nil || len(advs) != 0 || len(evs) != 0 {
		t.Errorf("empty list: %v %v %v", advs, evs, err)
	}
	// Blocked or unauthorised API.
	api4 := newFakeAPI(t)
	api4.handlers[advisoriesURI] = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }
	c4, _ := newClient(t, api4, t.TempDir(), "", fetch.ModeOnline)
	if _, _, err := NewAdvisories(c4).ListAdvisories(ctx, advisoryLoc()); !errors.Is(err, fetch.ErrUnavailable) || fetch.StateFor(err) != domain.SourceUnavailable {
		t.Errorf("403: %v", err)
	}
}
