package discovery

import (
	"testing"
)

func remoteTags(names ...string) []RemoteTag {
	out := make([]RemoteTag, 0, len(names))
	for _, n := range names {
		out = append(out, RemoteTag{Name: n})
	}
	return out
}

// postgresLikeTags approximates the PostgreSQL tag list: 3-group tags for
// majors 8/9, 2-group tags for majors >= 10, plus prerelease and other junk.
func postgresLikeTags() []string {
	var tags []string
	for maj := 8; maj <= 9; maj++ {
		for line := 4; line <= 6; line++ {
			for patch := 0; patch <= 24; patch++ {
				tags = append(tags, "REL"+itoa4(maj)+"_"+itoa4(line)+"_"+itoa4(patch))
			}
		}
	}
	for maj := 10; maj <= 17; maj++ {
		for patch := 0; patch <= 23; patch++ {
			tags = append(tags, "REL_"+itoa4(maj)+"_"+itoa4(patch))
		}
	}
	tags = append(tags, "REL9_6_24", "REL_17_RC1", "REL_17_BETA1", "PG95-1_01", "master", "REL_18_0")
	// keep only the REL-prefixed ones for the 8/9 lines
	var filtered []string
	for _, t := range tags {
		if len(t) > 3 && t[:3] == "REL" {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func itoa4(n int) string { return itoa(n) }

func TestAnalyzeTagsComponentScheme(t *testing.T) {
	repo := mustRepo(t, "github.com/postgres/postgres")
	tags := remoteTags(postgresLikeTags()...)
	ta := AnalyzeTags(repo, tags)
	if ta.StableCount == 0 {
		t.Fatalf("no stable releases parsed; scheme %q pattern %q junk %v", ta.Scheme, ta.TagPattern, ta.Junk[:4])
	}
	if ta.Scheme != SchemeComponentGroups {
		t.Fatalf("scheme = %q, want %q", ta.Scheme, SchemeComponentGroups)
	}
	// REL_17_2 must parse as 17.0.2 (the dropped middle component), not 17.2.0
	v, ok := ta.Find("REL_17_2")
	if !ok {
		t.Fatal("REL_17_2 not parsed")
	}
	if v.Semver != "17.0.2" {
		t.Fatalf("REL_17_2 = %s, want 17.0.2 (drop-middle)", v.Semver)
	}
	// REL9_6_24 keeps all three components
	v9, ok := ta.Find("REL9_6_24")
	if !ok || v9.Semver != "9.6.24" {
		t.Fatalf("REL9_6_24 = %+v, want 9.6.24", v9)
	}
	if ta.Latest == "" {
		t.Fatal("no latest stable tag")
	}
	// the proposed pattern was tested against the real tag list
	if ta.PatternCoverage == "" {
		t.Fatal("pattern coverage not recorded")
	}
}

func TestAnalyzeTagsComponentSchemeGain(t *testing.T) {
	// 2-group tags are all older than 3-group tags: the second number stays
	// the minor (v1_2 → 1.2.0, v1_2_3 → 1.2.3).
	repo := mustRepo(t, "github.com/example/rubylike")
	var names []string
	for i := 0; i <= 20; i++ {
		names = append(names, "v1_"+itoa(i))
	}
	for minor := 0; minor <= 9; minor++ {
		for patch := 0; patch <= 5; patch++ {
			names = append(names, "v2_"+itoa(minor)+"_"+itoa(patch))
		}
	}
	ta := AnalyzeTags(repo, remoteTags(names...))
	if ta.Scheme != SchemeComponentGroups {
		t.Fatalf("scheme = %q, want component-groups", ta.Scheme)
	}
	if v, ok := ta.Find("v1_12"); !ok || v.Semver != "1.12.0" {
		t.Fatalf("v1_12 = %+v, want 1.12.0", v)
	}
	if v, ok := ta.Find("v2_3_4"); !ok || v.Semver != "2.3.4" {
		t.Fatalf("v2_3_4 = %+v, want 2.3.4", v)
	}
}

func TestAnalyzeTagsNoBogusComponentScheme(t *testing.T) {
	// A semver-tagged repository must not fall back to component groups.
	repo := mustRepo(t, "github.com/example/plain")
	tags := remoteTags("v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0", "chore", "docs")
	ta := AnalyzeTags(repo, tags)
	if ta.Scheme == SchemeComponentGroups || ta.StableCount != 4 {
		t.Fatalf("scheme %q stable %d; want plain semver with 4 stable", ta.Scheme, ta.StableCount)
	}
	// A handful of underscore tags among hundreds of junk tags must not
	// produce a scheme either (the postgresql "0 of 693" failure mode in
	// reverse: never propose a pattern that matches almost nothing).
	junk := []string{"PG95-1_01", "master", "HEAD", "win-builds"}
	for i := 0; i < 200; i++ {
		junk = append(junk, "refs_"+itoa(i))
	}
	ta = AnalyzeTags(repo, remoteTags(junk...))
	if ta.StableCount != 0 || ta.Scheme == SchemeComponentGroups {
		t.Fatalf("junk-only list: stable %d scheme %q; want 0 stable, no component scheme", ta.StableCount, ta.Scheme)
	}
}

func TestTagFamilies(t *testing.T) {
	repo := mustRepo(t, "github.com/kubernetes/ingress-nginx-like")
	tags := remoteTags("helm-chart-4.11.0", "helm-chart-4.12.0", "helm-chart-4.13.0", "helm-chart-4.14.0", "helm-chart-4.15.0",
		"controller-v1.12.0", "controller-v1.13.0", "controller-v1.14.0", "controller-v1.15.0", "v1.0.0", "junk")
	ta := AnalyzeTags(repo, tags)
	if len(ta.Families) != 2 {
		t.Fatalf("families = %+v, want 2 (helm-chart-, controller-v; lone v1.0.0 is not a family)", ta.Families)
	}
	if ta.Families[0].Prefix != "helm-chart-" || ta.Families[0].Latest != "helm-chart-4.15.0" {
		t.Fatalf("top family = %+v", ta.Families[0])
	}
}

func mustRepo(t *testing.T, s string) RepoRef {
	t.Helper()
	r, err := ParseRepo(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
