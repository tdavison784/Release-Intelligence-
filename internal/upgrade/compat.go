package upgrade

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Compatibility rules.
const (
	RuleCompatNarrowed   = "compat:narrowed"
	RuleCompatChanged    = "compat:changed"
	RuleCompatChangedRaw = "compat:changed-raw"
	RuleCompatTargetOnly = "compat:target-only"
)

type compatKey struct{ platform, kind string }

func normKind(k string) string {
	if k == "" {
		return "supported"
	}
	return k
}

// PlatformName returns the display name of a platform id.
func PlatformName(p string) string {
	switch strings.ToLower(p) {
	case "kubernetes", "k8s":
		return "Kubernetes"
	case "openshift":
		return "OpenShift"
	case "helm":
		return "Helm"
	}
	return capitalize(p)
}

var kindOrder = map[string]int{"supported": 0, "minimum": 1, "tested": 2, "chart-kubeVersion": 3}
var platformOrder = map[string]int{"kubernetes": 0, "openshift": 1}

func orderIndex(m map[string]int, k string) int {
	if i, ok := m[k]; ok {
		return i
	}
	return len(m)
}

// versionSet is the set of platform minor versions a constraint admits,
// evaluated over a finite list of candidate minors.
type versionSet struct {
	ok      bool // membership could be computed
	members map[lineKey]bool
	open    bool // also admits versions beyond the candidate range
}

const maxMinorSpan = 200

// candidateLines collects every major.minor mentioned by the constraints and
// fills the gaps within each major.
func candidateLines(cs ...*domain.CompatibilityConstraint) []lineKey {
	found := map[lineKey]bool{}
	for _, c := range cs {
		if c == nil {
			continue
		}
		for _, v := range c.Versions {
			if k, ok := parseLine(v); ok {
				found[k] = true
			}
		}
		for _, s := range []string{c.Constraint, c.Raw} {
			for _, m := range minorRe.FindAllString(s, -1) {
				if k, ok := parseLine(m); ok {
					found[k] = true
				}
			}
		}
	}
	span := map[uint64][2]uint64{}
	for k := range found {
		mm, ok := span[k.major]
		if !ok {
			mm = [2]uint64{k.minor, k.minor}
		}
		if k.minor < mm[0] {
			mm[0] = k.minor
		}
		if k.minor > mm[1] {
			mm[1] = k.minor
		}
		span[k.major] = mm
	}
	var out []lineKey
	for major, mm := range span {
		if mm[1]-mm[0] > maxMinorSpan {
			for k := range found {
				if k.major == major {
					out = append(out, k)
				}
			}
			continue
		}
		for m := mm[0]; m <= mm[1]; m++ {
			out = append(out, lineKey{major, m})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}

func evalSet(c *domain.CompatibilityConstraint, cands []lineKey) versionSet {
	if c == nil {
		return versionSet{}
	}
	if len(c.Versions) > 0 {
		s := versionSet{ok: true, members: map[lineKey]bool{}}
		for _, v := range c.Versions {
			if k, ok := parseLine(v); ok {
				s.members[k] = true
			}
		}
		if len(s.members) > 0 {
			return s
		}
	}
	if strings.TrimSpace(c.Constraint) == "" {
		return versionSet{}
	}
	cs, err := semver.NewConstraint(c.Constraint)
	if err != nil {
		return versionSet{}
	}
	check := func(major, minor, patch uint64) bool {
		return cs.Check(semver.New(major, minor, patch, "", ""))
	}
	s := versionSet{ok: true, members: map[lineKey]bool{}}
	for _, k := range cands {
		if check(k.major, k.minor, 0) || check(k.major, k.minor, 999) {
			s.members[k] = true
		}
	}
	if len(cands) > 0 {
		last := cands[len(cands)-1]
		s.open = check(last.major, last.minor+100, 0)
	}
	return s
}

type run struct{ from, to lineKey }

// runs groups members into consecutive ranges over the candidate list.
func runs(members map[lineKey]bool, cands []lineKey) []run {
	var out []run
	for i, k := range cands {
		if !members[k] {
			continue
		}
		if n := len(out); n > 0 && i > 0 && cands[i-1] == out[n-1].to && members[cands[i-1]] && cands[i-1].major == k.major {
			out[n-1].to = k
			continue
		}
		out = append(out, run{k, k})
	}
	return out
}

func (r run) String() string {
	if r.from == r.to {
		return r.from.String()
	}
	return r.from.String() + "–" + r.to.String()
}

func formatRuns(rs []run) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.String()
	}
	return strings.Join(parts, ", ")
}

func (s versionSet) display(cands []lineKey) string {
	rs := runs(s.members, cands)
	if len(rs) == 0 {
		if s.open {
			return "any"
		}
		return "none"
	}
	if s.open && len(cands) > 0 && rs[len(rs)-1].to == cands[len(cands)-1] {
		head := formatRuns(rs[:len(rs)-1])
		tail := "≥ " + rs[len(rs)-1].from.String()
		if head == "" {
			return tail
		}
		return head + ", " + tail
	}
	return formatRuns(rs)
}

func maxMember(s versionSet, cands []lineKey) string {
	for i := len(cands) - 1; i >= 0; i-- {
		if s.members[cands[i]] {
			return cands[i].String()
		}
	}
	return ""
}

func rawOf(c *domain.CompatibilityConstraint) string {
	switch {
	case strings.TrimSpace(c.Raw) != "":
		return strings.TrimSpace(c.Raw)
	case c.Constraint != "":
		return c.Constraint
	}
	return strings.Join(c.Versions, ", ")
}

// compatResult is the comparison of one (platform, kind) between endpoints.
type compatResult struct {
	computable bool
	fromDisp   string
	toDisp     string
	drops      string
	adds       string
	narrowed   bool
	unchanged  bool
}

func compareConstraints(f, t *domain.CompatibilityConstraint) compatResult {
	cands := candidateLines(f, t)
	fs, ts := evalSet(f, cands), evalSet(t, cands)
	var r compatResult
	if f != nil {
		r.fromDisp = rawOf(f)
	}
	if t != nil {
		r.toDisp = rawOf(t)
	}
	if fs.ok {
		r.fromDisp = fs.display(cands)
	}
	if ts.ok {
		r.toDisp = ts.display(cands)
	}
	if f == nil || t == nil {
		return r
	}
	if !fs.ok || !ts.ok {
		r.unchanged = rawOf(f) == rawOf(t)
		return r
	}
	r.computable = true
	dropped, added := map[lineKey]bool{}, map[lineKey]bool{}
	for k := range fs.members {
		if !ts.members[k] {
			dropped[k] = true
		}
	}
	for k := range ts.members {
		if !fs.members[k] {
			added[k] = true
		}
	}
	var drops, adds []string
	if s := formatRuns(runs(dropped, cands)); s != "" {
		drops = append(drops, s)
	}
	if fs.open && !ts.open {
		drops = append(drops, "> "+maxMember(ts, cands))
	}
	if s := formatRuns(runs(added, cands)); s != "" {
		adds = append(adds, s)
	}
	if ts.open && !fs.open {
		adds = append(adds, "> "+maxMember(fs, cands))
	}
	r.drops, r.adds = strings.Join(drops, ", "), strings.Join(adds, ", ")
	r.narrowed = r.drops != ""
	r.unchanged = r.drops == "" && r.adds == ""
	return r
}

// compatLabel names a (platform, kind) for titles: "Kubernetes support".
func compatLabel(platform, kind string) string {
	p := PlatformName(platform)
	switch kind {
	case "supported":
		return p + " support"
	case "tested":
		return p + " tested versions"
	case "minimum":
		return p + " minimum version"
	case "chart-kubeVersion":
		return p + " chart kubeVersion"
	}
	return p + " " + kind
}

func verifyHint(platform, kind, versions string) string {
	word := "supported"
	if kind == "tested" {
		word = "tested"
	}
	switch strings.ToLower(platform) {
	case "kubernetes", "k8s", "openshift":
		return fmt.Sprintf("Verify the cluster runs a %s %s version (%s) before upgrading.", word, PlatformName(platform), versions)
	}
	return fmt.Sprintf("Verify you use a %s %s version (%s) before upgrading.", word, PlatformName(platform), versions)
}

// compatChanges compares constraints of From and To per (platform, kind).
func (b *builder) compatChanges() {
	first := func(cs []domain.CompatibilityConstraint) map[compatKey]*domain.CompatibilityConstraint {
		out := map[compatKey]*domain.CompatibilityConstraint{}
		for i := range cs {
			k := compatKey{strings.ToLower(cs[i].Platform), normKind(cs[i].Kind)}
			if _, ok := out[k]; !ok {
				c := cs[i]
				out[k] = &c
			}
		}
		return out
	}
	fromC, toC := first(b.from.Compat), first(b.to.Compat)
	var keys []compatKey
	seen := map[compatKey]bool{}
	for _, m := range []map[compatKey]*domain.CompatibilityConstraint{fromC, toC} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, c := keys[i], keys[j]
		if pa, pc := orderIndex(platformOrder, a.platform), orderIndex(platformOrder, c.platform); pa != pc {
			return pa < pc
		}
		if a.platform != c.platform {
			return a.platform < c.platform
		}
		if ka, kc := orderIndex(kindOrder, a.kind), orderIndex(kindOrder, c.kind); ka != kc {
			return ka < kc
		}
		return a.kind < c.kind
	})

	fromTag, toTag := b.from.Version.String(), b.to.Version.String()
	for _, k := range keys {
		f, t := fromC[k], toC[k]
		r := compareConstraints(f, t)
		prefix := fmt.Sprintf("%s (%s): ", PlatformName(k.platform), k.kind)
		cc := domain.CompatibilityChange{Platform: k.platform, From: f, To: t, Narrowed: r.narrowed}
		switch {
		case f == nil:
			cc.Summary = prefix + fmt.Sprintf("%s requires %s (not stated for %s)", toTag, r.toDisp, fromTag)
		case t == nil:
			cc.Summary = prefix + fmt.Sprintf("%s → not stated for %s", r.fromDisp, toTag)
		case r.unchanged:
			cc.Summary = prefix + r.toDisp + " (unchanged)"
		default:
			cc.Summary = prefix + r.fromDisp + " → " + r.toDisp
			var parts []string
			if r.drops != "" {
				parts = append(parts, "drops "+r.drops)
			}
			if r.adds != "" {
				parts = append(parts, "adds "+r.adds)
			}
			if len(parts) > 0 {
				cc.Summary += "; " + strings.Join(parts, ", ")
			}
		}
		b.edge.Compatibility = append(b.edge.Compatibility, cc)

		var ev []domain.EvidenceID
		if f != nil {
			ev = append(ev, f.Evidence...)
		}
		if t != nil {
			ev = append(ev, t.Evidence...)
		}
		label := compatLabel(k.platform, k.kind)
		subject := k.platform + "/" + k.kind
		switch {
		case t == nil:
			b.warnf("No %s (%s) compatibility data for target %s (was %s for %s)", PlatformName(k.platform), k.kind, toTag, r.fromDisp, fromTag)
		case f == nil:
			title := fmt.Sprintf("%s requires %s %s (%s)", toTag, PlatformName(k.platform), r.toDisp, k.kind)
			if k.kind == "tested" {
				title = fmt.Sprintf("%s is tested on %s %s", toTag, PlatformName(k.platform), r.toDisp)
			}
			b.addChange(domain.Change{
				Category:       domain.CategoryCompatibility,
				ActionRequired: k.kind != "tested",
				Title:          title,
				Detail:         fmt.Sprintf("No %s constraint was stated for %s, so it is unknown whether support narrowed. %s", k.kind, fromTag, verifyHint(k.platform, k.kind, r.toDisp)),
				Subjects:       []string{subject},
				Provenance:     computed(RuleCompatTargetOnly),
				Evidence:       ev,
			}, RuleCompatTargetOnly, subject)
		case r.unchanged:
		case !r.computable:
			p := computed(RuleCompatChangedRaw)
			p.Confidence = domain.ConfidenceMedium
			b.addChange(domain.Change{
				Category:       domain.CategoryCompatibility,
				ActionRequired: true,
				Title:          fmt.Sprintf("%s changed: %s → %s", label, r.fromDisp, r.toDisp),
				Detail:         "The ranges could not be compared automatically. " + verifyHint(k.platform, k.kind, r.toDisp),
				Subjects:       []string{subject},
				Provenance:     p,
				Evidence:       ev,
			}, RuleCompatChangedRaw, subject)
		case r.narrowed:
			b.addChange(domain.Change{
				Category:       domain.CategoryCompatibility,
				ActionRequired: true,
				Title:          fmt.Sprintf("%s narrowed: %s → %s (drops %s)", label, r.fromDisp, r.toDisp, r.drops),
				Detail:         verifyHint(k.platform, k.kind, r.toDisp),
				Subjects:       []string{subject},
				Provenance:     computed(RuleCompatNarrowed),
				Evidence:       ev,
			}, RuleCompatNarrowed, subject)
		default:
			b.addChange(domain.Change{
				Category:   domain.CategoryCompatibility,
				Title:      fmt.Sprintf("%s extended: %s → %s (adds %s)", label, r.fromDisp, r.toDisp, r.adds),
				Subjects:   []string{subject},
				Provenance: computed(RuleCompatChanged),
				Evidence:   ev,
			}, RuleCompatChanged, subject)
		}
	}
}
