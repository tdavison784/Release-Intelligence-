package catalog

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Lifecycle states.
const (
	// LifecycleDeprecated: still maintained, but upstream tells users to move
	// on (a successor exists, or the project is winding down).
	LifecycleDeprecated = "deprecated"
	// LifecycleEndOfLife: no further releases, bug fixes or security fixes.
	LifecycleEndOfLife = "end-of-life"
)

// Lifecycle declares that the product, or the releases matching Versions, is
// no longer (fully) maintained upstream. It is the one piece of knowledge
// that no release artifact carries: the absence of future releases cannot be
// ingested, only stated, so the statement lives in the definition, like an
// exception, and must cite sources whose retrieved text evidences it.
//
// An upgrade whose target is affected reports an action-required deprecation
// (end-of-life) or an informational one (deprecated); an upgrade that leaves
// an affected release line reports that it does. Both cite the evidence of
// Sources, whose note items are reported through the statement instead of as
// ordinary per-release notes (a banner is not a change of any one release).
//
// Typical uses: a retired project (ingress-nginx), a discontinued release
// channel, or the end-of-life of old release lines (Versions: "< 3.3.0").
type Lifecycle struct {
	State string `yaml:"state" json:"state"`
	// Since is when the state took effect: YYYY-MM-DD or YYYY-MM.
	Since string `yaml:"since,omitempty" json:"since,omitempty"`
	// Versions is a semver constraint on the release versions the statement
	// applies to; empty means every release of the product.
	Versions string `yaml:"versions,omitempty" json:"versions,omitempty"`
	// Summary is the statement, in one or two sentences.
	Summary string `yaml:"summary" json:"summary"`
	// Sources are ids of release-notes / changelog / upgrade-guide sources
	// whose items are the evidence of the statement.
	Sources []string `yaml:"sources" json:"sources"`
}

var lifecycleSinceRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])(-(0[1-9]|[12]\d|3[01]))?$`)

// Affects reports whether the statement applies to release version v.
func (l Lifecycle) Affects(v domain.Version) (bool, error) {
	return v.Satisfies(l.Versions)
}

// HasSource reports whether id is one of the statement's evidence sources.
func (l Lifecycle) HasSource(id string) bool {
	for _, s := range l.Sources {
		if s == id {
			return true
		}
	}
	return false
}

// IsLifecycleSource reports whether any lifecycle statement cites the source.
func (d *ProductDefinition) IsLifecycleSource(id string) bool {
	for _, l := range d.Lifecycle {
		if l.HasSource(id) {
			return true
		}
	}
	return false
}

func (v *validator) lifecycle(d *ProductDefinition) {
	for i, l := range d.Lifecycle {
		p := fmt.Sprintf("lifecycle[%d]", i)
		switch l.State {
		case LifecycleDeprecated, LifecycleEndOfLife:
		default:
			v.errf(p+".state", "must be %s or %s, got %q", LifecycleDeprecated, LifecycleEndOfLife, l.State)
		}
		if l.Since != "" && !lifecycleSinceRe.MatchString(l.Since) {
			v.errf(p+".since", "must be YYYY-MM-DD or YYYY-MM, got %q", l.Since)
		}
		if strings.TrimSpace(l.Versions) != "" {
			if _, err := semver.NewConstraint(l.Versions); err != nil {
				v.errf(p+".versions", "invalid semver constraint %q: %v", l.Versions, err)
			}
		}
		if strings.TrimSpace(l.Summary) == "" {
			v.errf(p+".summary", "required")
		}
		if len(l.Sources) == 0 {
			v.errf(p+".sources", "at least one source required: a lifecycle statement without evidence is not reported")
		}
		for j, id := range l.Sources {
			sp := fmt.Sprintf("%s.sources[%d]", p, j)
			s, ok := d.Source(id)
			switch {
			case !ok:
				v.errf(sp, "unknown source %q", id)
			case !s.HasRole(domain.RoleReleaseNotes) && !s.HasRole(domain.RoleChangelog) && !s.HasRole(domain.RoleUpgradeGuide):
				v.errf(sp, "source %q must have a release-notes, changelog or upgrade-guide role: its note items are the evidence", id)
			}
		}
	}
}
