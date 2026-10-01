package enrich

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// recorded loads an edge from the offline end-to-end goldens (real upstream
// data, see internal/app/testdata/e2e).
func recorded(t *testing.T, name string) *domain.UpgradeEdge {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "app", "testdata", "e2e", "golden", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var e domain.UpgradeEdge
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	return &e
}

// byTitle returns the ids of the changes whose title contains every fragment.
func byTitle(t *testing.T, e *domain.UpgradeEdge, fragments ...string) []string {
	t.Helper()
	var out []string
	for _, c := range e.Changes {
		ok := true
		for _, f := range fragments {
			ok = ok && strings.Contains(c.Title, f)
		}
		if ok {
			out = append(out, c.ID)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no change titled %q", fragments)
	}
	return out
}

func groupOf(cs []Candidate, id string) []string {
	for _, c := range cs {
		for _, x := range c.Changes {
			if x == id {
				return c.Changes
			}
		}
	}
	return nil
}

func sameSet(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

// The semantic duplicates the deterministic merge leaves in the real
// cert-manager 1.17 → 1.18 edge become candidate groups, without mixing
// unrelated defaults.
func TestCandidatesOnRecordedCertManager(t *testing.T) {
	e := recorded(t, "cert-manager_v1.17.0_v1.18.0")
	cs := Candidates(e, CandidateOptions{})

	rotation := byTitle(t, e, "RotationPolicy")
	if len(rotation) != 3 {
		t.Fatalf("fixture: expected the RotationPolicy change in 3 sources, got %v", rotation)
	}
	if g := groupOf(cs, rotation[0]); !sameSet(g, rotation) {
		t.Errorf("RotationPolicy group = %v, want %v", g, rotation)
	}
	history := byTitle(t, e, "Certificate.Spec.RevisionHistoryLimit")
	history = append(history, byTitle(t, e, "default `revisionHistoryLimit`")...)
	if len(history) != 3 {
		t.Fatalf("fixture: expected the RevisionHistoryLimit change in 3 sources, got %v", history)
	}
	if g := groupOf(cs, history[0]); !sameSet(g, history) {
		t.Errorf("RevisionHistoryLimit group = %v, want %v", g, history)
	}
	port := append(byTitle(t, e, "prometheus.servicemonitor.targetPort"), byTitle(t, e, "port names instead of numbers")...)
	if g := groupOf(cs, port[0]); !sameSet(g, port) {
		t.Errorf("targetPort diff ↔ release note group = %v, want %v", g, port)
	}
	helmValue := byTitle(t, e, "disableHTTPChallengesRole")
	if g := groupOf(cs, helmValue[0]); !sameSet(g, helmValue) || len(helmValue) != 2 {
		t.Errorf("new Helm value ↔ release note group = %v, want %v", g, helmValue)
	}
	for _, c := range cs {
		if len(c.Signals) == 0 || len(c.Hints) == 0 || !strings.HasPrefix(c.ID, "cand-") {
			t.Errorf("candidate without signals or hints: %+v", c)
		}
	}
}

// On every recorded edge the groups are disjoint, bounded and few.
func TestCandidatesAreBoundedOnRecordedEdges(t *testing.T) {
	for _, name := range []string{"cert-manager_v1.17.0_v1.18.0", "istio_1.29.2_1.30.1", "argo-cd_v2.14.11_v3.0.6"} {
		t.Run(name, func(t *testing.T) {
			e := recorded(t, name)
			cs := Candidates(e, CandidateOptions{})
			seen := map[string]string{}
			for _, c := range cs {
				if len(c.Changes) < 2 || len(c.Changes) > 6 {
					t.Errorf("group size %d: %+v", len(c.Changes), c)
				}
				for _, id := range c.Changes {
					if other, dup := seen[id]; dup {
						t.Errorf("change %s in groups %s and %s", id, other, c.ID)
					}
					seen[id] = c.ID
				}
			}
			if len(cs) == 0 || len(cs) > len(e.Changes)/4 {
				t.Errorf("%d groups for %d changes", len(cs), len(e.Changes))
			}
			again := Candidates(e, CandidateOptions{})
			if !reflect.DeepEqual(cs, again) {
				t.Error("candidate generation is not deterministic")
			}
		})
	}
}

func TestTextHelpers(t *testing.T) {
	if got := splitCamel("disableHTTPChallengesRole"); !reflect.DeepEqual(got, []string{"disable", "HTTP", "Challenges", "Role"}) {
		t.Errorf("splitCamel = %v", got)
	}
	for raw, want := range map[string]bool{
		"Certificate.Spec.PrivateKey.RotationPolicy": true, "PathType": true, "--extra-certificate-annotations": true,
		"Never": false, "v1.24.4": false, "#7723": false, "@wallrj": false, "p, role:dev": false, "https://x.io/a": false,
	} {
		if got := keyLike(normKey(raw), raw); got != want {
			t.Errorf("keyLike(%q) = %v", raw, got)
		}
	}
	if !prefixMatch("servicemon", "servicemonitor") || prefixMatch("port", "portal") || !prefixMatch("port", "port") {
		t.Error("prefixMatch")
	}
	if got := keyTokens("prometheus.servicemonitor.targetPort"); !reflect.DeepEqual(got, []string{"prometheu", "servicemonitor", "target", "port"}) {
		t.Errorf("keyTokens = %v", got)
	}
	if !containsWord("set `global.rbac.x` now", "global.rbac.x") || containsWord("miniredis", "redis") {
		t.Error("containsWord")
	}
}
