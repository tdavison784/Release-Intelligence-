package app

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestSampleReleases(t *testing.T) {
	var vs []domain.Version
	for _, s := range []string{"1.16.0", "1.16.5", "1.17.0", "1.17.4", "1.18.0", "1.18.1", "1.18.6", "1.19.0"} {
		vs = append(vs, domain.MustVersion("v"+s, s))
	}
	got := SampleReleases(vs, 3)
	want := []string{"1.17.0", "1.17.4", "1.18.0", "1.18.6", "1.19.0"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i].Semver != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
