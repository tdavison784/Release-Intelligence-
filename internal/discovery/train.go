package discovery

import (
	"fmt"
	"regexp"
	"strings"

)

// selectTagTrain picks the product's tag train when a repository publishes
// several dot-separated version trains (kubernetes/ingress-nginx tags both
// controller-v1.x.y and helm-chart-4.x.z; the chart train typically has more
// tags). Numerosity is the default; scan evidence outranks it:
//
//   - a CI release pipeline triggered on tags of one prefix (on: push:
//     tags: ["controller-v*"]) names the train that releases the product;
//   - a chart's appVersion equal to the newest version of one family names
//     the train the chart packages.
//
// It returns the family to switch to (nil when the most numerous family
// stands) and the human rationale.
func selectTagTrain(ta *TagAnalysis, cands []Candidate) (*TagFamily, string) {
	if len(ta.Families) < 2 || ta.Scheme == SchemeComponentGroups {
		return nil, ""
	}
	top := ta.Families[0]
	if top.Count >= 2*ta.Families[1].Count {
		return nil, "" // dominant family, no ambiguity worth evidence
	}
	score := func(f TagFamily) (int, []string) {
		s := 0
		var why []string
		for _, c := range cands {
			if c.Kind == KindReleaseTrigger && triggerNamesFamily(c.Value, f.Prefix) {
				s += 3
				why = append(why, fmt.Sprintf("release workflow triggers on tags %q (%s)", c.Value, evidenceWhere(c)))
			}
		}
		for _, c := range cands {
			if c.Kind != KindHelmChart || c.Attr("repo") != "" {
				continue
			}
			if appVersionNamesTrain(c.Attr("appVersion"), f) {
				s += 3
				why = append(why, fmt.Sprintf("chart %s appVersion %q equals the newest %s-family version (%s)", c.Value, c.Attr("appVersion"), f.Prefix, f.Latest))
			}
		}
		return s, why
	}
	topScore, _ := score(top)
	for _, f := range ta.Families[1:] {
		s, why := score(f)
		if s > topScore && s >= 3 {
			return &f, fmt.Sprintf("%s; the most numerous family %q (%d tags) carries no such evidence", strings.Join(why, "; "), top.Prefix, top.Count)
		}
	}
	return nil, ""
}

// triggerNamesFamily reports whether a CI tag-trigger pattern (glob such as
// "controller-v*") selects one prefix family.
func triggerNamesFamily(pattern, prefix string) bool {
	lit := pattern
	if i := strings.IndexAny(pattern, "*?(["); i >= 0 {
		lit = pattern[:i]
	}
	if lit == "" {
		return false
	}
	return strings.HasPrefix(prefix, lit)
}

var appVerRe = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)`)

// appVersionNamesTrain reports whether a chart appVersion value is the
// newest release of a tag family.
func appVersionNamesTrain(appVersion string, f TagFamily) bool {
	m := appVerRe.FindStringSubmatch(strings.TrimSpace(appVersion))
	if m == nil {
		return false
	}
	return m[1] == f.LatestVersion
}

// evidenceWhere names the file of the first evidence of a candidate.
func evidenceWhere(c Candidate) string {
	for _, e := range c.Evidence {
		if e.URI != "" {
			if i := strings.LastIndex(e.URI, "/blob/"); i > 0 {
				return e.URI[i+len("/"):]
			}
			return e.URI
		}
	}
	for _, f := range splitList(c.Attr("files")) {
		return f
	}
	return "no evidence"
}

// chartTrainFamily returns the tag family that versions a chart published by
// chart-releaser (the "helm-chart-" convention or any other non-product
// family), when one exists.
func chartTrainFamily(ta *TagAnalysis, chartName string) *TagFamily {
	productPrefix := ta.Prefix
	for i := range ta.Families {
		f := ta.Families[i]
		if f.Prefix == productPrefix && ta.Scheme != SchemeComponentGroups {
			continue
		}
		if strings.HasPrefix(f.Prefix, chartName) || f.Prefix == "helm-chart-" || strings.Contains(f.Prefix, "chart") {
			return &f
		}
	}
	return nil
}
