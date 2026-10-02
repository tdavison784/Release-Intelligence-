package semvalidate

import (
	"context"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// restatementValidator confirms the subject and change of a (typically
// note-derived) proposal against a COMPUTED diff of the same edge that states
// the same thing: a note saying "the logConfig section was removed" next to
// the edge's `values:section-removed`. The computed change already carries the
// artifact evidence of both sides, which the result cites. It never refutes
// on silence; it refutes only when the computed diff states an exclusive
// alternative (added vs removed vs default-changed, or other before/after).
//
// The computed diffs are read through their deterministic rule ids, subjects
// and the fixed title/detail shapes internal/upgrade renders; a shape this
// package does not know is simply not a match.
type restatementValidator struct{}

func (restatementValidator) Name() string { return ProducerRestatement }

// stated is what one computed change says, in the vocabulary of an assertion.
type stated struct {
	change      domain.Change
	kind        domain.ChangeKind
	before      *string
	after       *string
	replacement string // renamed/moved: the new name
}

var (
	crdTitleRe   = regexp.MustCompile("^(\\S+) (\\S+) schema:")
	crdDetailRe  = regexp.MustCompile(`in the (?:(\S+)/)?(\S+) schema of (\S+?)[:;. ]`)
	apiTitleRe   = regexp.MustCompile("API version `(?:([^/`]*)/)?([^`]+)` of (\\S+)")
	storageRe    = regexp.MustCompile("^Storage version of `([^`]+)` changes (\\S+) → (\\S+)")
	storageKind  = regexp.MustCompile(`Existing (\S+) objects`)
	crdAttrLines = regexp.MustCompile(`(?m)^(\S+): (.+) → (.+)$`)
)

func (restatementValidator) Validate(_ context.Context, in knowledge.ValidationInput) ([]domain.ValidationResult, error) {
	if !applies(in, domain.SubjectHelmValue, domain.SubjectCRDField, domain.SubjectGVK, domain.SubjectImage) || in.Edge == nil {
		return nil, nil
	}
	subj, chg := in.Assertion.Subject, in.Assertion.Change
	if subj.Family == domain.SubjectHelmValue && valuesDriftMentions(in, subj) {
		v := inconclusive("restatement:key-root", "the values of a chart holding %s are rooted differently at the two releases, so the computed key diff is not reliable", subj.Path)
		return build(in, ProducerRestatement, map[domain.Aspect]verdict{domain.AspectSubject: v, domain.AspectChange: v}, evidenceSet{}), nil
	}
	var mentions []stated
	for _, c := range in.Edge.Changes {
		if c.Provenance.Method != domain.MethodComputed {
			continue
		}
		mentions = append(mentions, restated(subj, c)...)
	}
	if len(mentions) == 0 {
		v := inconclusive("restatement:none", "no computed diff of the edge mentions this subject")
		return build(in, ProducerRestatement, map[domain.Aspect]verdict{domain.AspectSubject: v, domain.AspectChange: v}, evidenceSet{}), nil
	}
	var ev evidenceSet
	vs := map[domain.Aspect]verdict{}
	var match *stated
	var contra []string
	for i, m := range mentions {
		agree, why := agrees(chg, m)
		switch {
		case agree && match == nil:
			match = &mentions[i]
		case !agree && why != "":
			contra = append(contra, why)
		}
	}
	first := mentions[0]
	if match != nil && len(contra) > 0 && subj.Name == "" {
		ev.add(resolve(match.change.Evidence, in.Edge.Evidence)...)
		v := inconclusive("restatement:ambiguous", "computed diffs disagree about this subject (%s); name the chart in the subject", strings.Join(first3(contra), "; "))
		return build(in, ProducerRestatement, map[domain.Aspect]verdict{domain.AspectSubject: confirmed("restatement:"+match.change.Provenance.Rule, "computed change %s names %s", match.change.ID, subj.Key()), domain.AspectChange: v}, ev), nil
	}
	if match != nil {
		ev.add(resolve(match.change.Evidence, in.Edge.Evidence)...)
		rule := "restatement:" + match.change.Provenance.Rule
		vs[domain.AspectSubject] = confirmed(rule, "computed change %s names %s", match.change.ID, subj.Key())
		vs[domain.AspectChange] = confirmed(rule, "computed change %s states the same %s: %q", match.change.ID, chg.Type, match.change.Title)
		return build(in, ProducerRestatement, vs, ev), nil
	}
	ev.add(resolve(first.change.Evidence, in.Edge.Evidence)...)
	rule := "restatement:" + first.change.Provenance.Rule
	vs[domain.AspectSubject] = confirmed(rule, "computed change %s names %s", first.change.ID, subj.Key())
	if len(contra) > 0 {
		vs[domain.AspectChange] = refuted(rule, "the edge's computed diff says otherwise: %s", strings.Join(first3(contra), "; "))
	} else {
		vs[domain.AspectChange] = inconclusive(rule, "computed change %s concerns the subject but not as a %s", first.change.ID, chg.Type)
	}
	return build(in, ProducerRestatement, vs, ev), nil
}

func first3(xs []string) []string {
	if len(xs) > 3 {
		return xs[:3]
	}
	return xs
}

// exclusive kinds contradict one another when stated about one subject.
var exclusive = map[domain.ChangeKind]bool{
	domain.ChangeKindAdded: true, domain.ChangeKindRemoved: true, domain.ChangeKindDefaultChanged: true,
}

// agrees reports whether the computed statement restates the asserted change;
// when it does not, why is non-empty iff it contradicts it.
func agrees(chg *domain.ChangeSpec, m stated) (agree bool, why string) {
	want := chg.Type
	if want == domain.ChangeKindValueChanged && m.kind == domain.ChangeKindDefaultChanged {
		want = m.kind // a default is the value of a values key
	}
	if m.kind != want {
		if exclusive[chg.Type] && exclusive[m.kind] {
			return false, "\"" + m.change.Title + "\" is " + string(m.kind) + ", not " + string(chg.Type)
		}
		return false, ""
	}
	if m.kind == domain.ChangeKindDefaultChanged || (m.kind == domain.ChangeKindValueChanged && m.before != nil) {
		if chg.Before != nil && m.before != nil && !sameValue(*chg.Before, *m.before) {
			return false, "the computed before is " + *m.before + ", not " + *chg.Before
		}
		if chg.After != nil && m.after != nil && !sameValue(*chg.After, *m.after) {
			return false, "the computed after is " + *m.after + ", not " + *chg.After
		}
	}
	if m.kind == domain.ChangeKindRenamed && (chg.ReplacedBy == nil || !sameName(chg.ReplacedBy, m.replacement)) {
		return false, ""
	}
	return true, ""
}

func sameName(s *domain.Subject, name string) bool {
	switch s.Family {
	case domain.SubjectHelmValue:
		return s.Path == name
	case domain.SubjectImage:
		return normRepo(s.Name) == normRepo(name)
	}
	return false
}

// restated translates one computed change into statements about subj.
func restated(subj *domain.Subject, c domain.Change) []stated {
	switch subj.Family {
	case domain.SubjectHelmValue:
		return restatedValues(subj, c)
	case domain.SubjectCRDField:
		return restatedField(subj, c)
	case domain.SubjectGVK:
		return restatedGVK(subj, c)
	case domain.SubjectImage:
		return restatedImage(subj, c)
	}
	return nil
}

// valuesGroupRe finds the section a values change names: "Helm values section
// `a.*` removed", "New Helm values section `a.*` (5 values)". Changes that
// only add keys UNDER an existing section ("3 new Helm values under `a.*`")
// are deliberately not matched by it.
var (
	removedGroupRe = regexp.MustCompile("^Helm values?(?: section)? `([^`]+?)(?:\\.\\*)?` removed")
	addedGroupRe   = regexp.MustCompile("^New Helm values section `([^`]+?)\\.\\*`")
	chartSuffixRe  = regexp.MustCompile(`\(chart ([^)]+)\)$`)
)

func exact(subjects []string, path string) bool {
	for _, s := range subjects {
		if s == path {
			return true
		}
	}
	return false
}

func restatedValues(subj *domain.Subject, c domain.Change) []stated {
	if m := chartSuffixRe.FindStringSubmatch(strings.TrimSpace(c.Title)); m != nil && subj.Name != "" && m[1] != subj.Name {
		return nil // another chart's change
	}
	switch c.Provenance.Rule {
	case upgrade.RuleValuesRemoved, upgrade.RuleValuesSectionRemoved:
		// the key itself, or a section that vanished as a whole; a computed
		// removal of keys BELOW the path leaves the path itself present
		if exact(c.Subjects, subj.Path) {
			return []stated{{change: c, kind: domain.ChangeKindRemoved}}
		}
		if m := removedGroupRe.FindStringSubmatch(c.Title); m != nil && m[1] == subj.Path {
			return []stated{{change: c, kind: domain.ChangeKindRemoved}}
		}
	case upgrade.RuleValuesAdded:
		if exact(c.Subjects, subj.Path) {
			return []stated{{change: c, kind: domain.ChangeKindAdded}}
		}
		if m := addedGroupRe.FindStringSubmatch(c.Title); m != nil && m[1] == subj.Path {
			return []stated{{change: c, kind: domain.ChangeKindAdded}}
		}
	case upgrade.RuleValuesDefaultChanged:
		if len(c.Subjects) != 1 || c.Subjects[0] != subj.Path {
			return nil
		}
		b, a, ok := strings.Cut(c.Detail, " → ")
		if !ok {
			return nil
		}
		return []stated{{change: c, kind: domain.ChangeKindDefaultChanged, before: &b, after: &a}}
	}
	return nil
}

// crdIdentity reads the group, version and kind a schema change names from
// the title/detail shapes of internal/upgrade/crds.go.
func crdIdentity(c domain.Change) (group, version, kind string, ok bool) {
	t := crdTitleRe.FindStringSubmatch(c.Title)
	d := crdDetailRe.FindStringSubmatch(c.Detail)
	if t == nil || d == nil {
		return "", "", "", false
	}
	return d[1], d[2], t[1], true
}

func restatedField(subj *domain.Subject, c domain.Change) []stated {
	rule := c.Provenance.Rule
	switch rule {
	case upgrade.RuleCRDFieldsRemoved, upgrade.RuleCRDFieldsAdded, upgrade.RuleCRDFieldRequired,
		upgrade.RuleCRDDefaultChanged, upgrade.RuleCRDEnumChanged, upgrade.RuleCRDFieldTypeChange:
	default:
		return nil
	}
	g, v, k, ok := crdIdentity(c)
	if !ok || g != subj.Group || k != subj.Kind || (subj.Version != "" && subj.Version != v) {
		return nil
	}
	hit := false
	want := strings.ReplaceAll(subj.Path, "[]", "")
	for _, s := range c.Subjects {
		if strings.ReplaceAll(s, "[]", "") == want {
			hit = true
		}
	}
	if !hit {
		return nil
	}
	switch rule {
	case upgrade.RuleCRDFieldsRemoved:
		return []stated{{change: c, kind: domain.ChangeKindRemoved}}
	case upgrade.RuleCRDFieldsAdded:
		return []stated{{change: c, kind: domain.ChangeKindAdded}}
	case upgrade.RuleCRDFieldRequired:
		return []stated{{change: c, kind: domain.ChangeKindNowRequired}}
	case upgrade.RuleCRDFieldTypeChange:
		return []stated{{change: c, kind: domain.ChangeKindValidationTightened}}
	case upgrade.RuleCRDEnumChanged:
		return []stated{{change: c, kind: domain.ChangeKindValidationTightened}}
	case upgrade.RuleCRDDefaultChanged:
		for _, m := range crdAttrLines.FindAllStringSubmatch(c.Detail, -1) {
			if strings.ReplaceAll(m[1], "[]", "") == want {
				b, a := m[2], m[3]
				return []stated{{change: c, kind: domain.ChangeKindDefaultChanged, before: &b, after: &a}}
			}
		}
	}
	return nil
}

func restatedGVK(subj *domain.Subject, c domain.Change) []stated {
	switch c.Provenance.Rule {
	case upgrade.RuleCRDVersionRemoved, upgrade.RuleCRDVersionUnserved, upgrade.RuleCRDVersionDeprecated, upgrade.RuleCRDVersionAdded:
		m := apiTitleRe.FindStringSubmatch(c.Title)
		if m == nil || m[1] != subj.Group || m[2] != subj.Version || m[3] != subj.Kind {
			return nil
		}
		kind := map[string]domain.ChangeKind{
			upgrade.RuleCRDVersionRemoved: domain.ChangeKindRemoved, upgrade.RuleCRDVersionUnserved: domain.ChangeKindRemoved,
			upgrade.RuleCRDVersionDeprecated: domain.ChangeKindDeprecated, upgrade.RuleCRDVersionAdded: domain.ChangeKindAdded,
		}[c.Provenance.Rule]
		return []stated{{change: c, kind: kind}}
	case upgrade.RuleCRDStorageChanged:
		m := storageRe.FindStringSubmatch(c.Title)
		k := storageKind.FindStringSubmatch(c.Detail)
		if m == nil || k == nil || k[1] != subj.Kind {
			return nil
		}
		_, group, _ := strings.Cut(m[1], ".")
		if group != subj.Group {
			return nil
		}
		b, a := `"`+m[2]+`"`, `"`+m[3]+`"`
		return []stated{{change: c, kind: domain.ChangeKindValueChanged, before: &b, after: &a}}
	}
	return nil
}

func restatedImage(subj *domain.Subject, c domain.Change) []stated {
	want := normRepo(subj.Name)
	switch c.Provenance.Rule {
	case upgrade.RuleImageRemoved, upgrade.RuleImageAdded:
		if len(c.Subjects) == 1 && normRepo(c.Subjects[0]) == want {
			k := domain.ChangeKindRemoved
			if c.Provenance.Rule == upgrade.RuleImageAdded {
				k = domain.ChangeKindAdded
			}
			return []stated{{change: c, kind: k}}
		}
	case upgrade.RuleImageMoved:
		if len(c.Subjects) == 2 && normRepo(c.Subjects[0]) == want {
			return []stated{{change: c, kind: domain.ChangeKindRenamed, replacement: c.Subjects[1]}}
		}
	}
	return nil
}

// valuesDriftMentions reports whether a chart whose values are rooted
// differently at the two releases mentions the subject path (as is, or with
// the wrapper added/removed): the computed key diff of such a chart would
// read a mere re-rooting as a removal and an addition.
func valuesDriftMentions(in knowledge.ValidationInput, subj *domain.Subject) bool {
	for _, p := range valuesPairs(in, subj.Name) {
		seg := rootDrift(p.from.Entries, p.to.Entries)
		if seg == "" {
			continue
		}
		for _, e := range []map[string]string{p.from.Entries, p.to.Entries} {
			if hasPath(e, subj.Path) || hasPath(e, seg+"."+subj.Path) || (strings.HasPrefix(subj.Path, seg+".") && hasPath(e, strings.TrimPrefix(subj.Path, seg+"."))) {
				return true
			}
		}
	}
	return false
}
