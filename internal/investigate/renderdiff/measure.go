package renderdiff

import (
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Measurement is the render coverage of a set of environment pairs (what
// the lane reports per groundtruth environment).
type Measurement struct {
	Targets       int `json:"targets"`
	OK            int `json:"ok"`
	Partial       int `json:"partial"`
	Unavailable   int `json:"unavailable"`
	NotApplicable int `json:"notApplicable"`
	// Reasons counts the non-ok pairs by reason class (the text before the
	// first ':', e.g. "values-incomplete", "renderer-unavailable").
	Reasons      map[string]int `json:"reasons,omitempty"`
	Changes      int            `json:"changes"`
	Correlated   int            `json:"correlated"`
	Uncorrelated int            `json:"uncorrelated"`
}

// Measure computes the coverage of pairs.
func Measure(pairs []domain.ManifestPair) Measurement {
	var m Measurement
	for _, p := range pairs {
		m.Targets++
		switch p.Status {
		case domain.CollectionOK:
			m.OK++
		case domain.CollectionPartial:
			m.Partial++
		case domain.CollectionUnavailable:
			m.Unavailable++
		case domain.CollectionNotApplicable:
			m.NotApplicable++
		}
		if p.Status != domain.CollectionOK {
			if m.Reasons == nil {
				m.Reasons = map[string]int{}
			}
			r, _, _ := strings.Cut(p.Reason, ":")
			m.Reasons[r]++
		}
		for _, c := range p.Changes {
			m.Changes++
			if len(c.ChangeItems) > 0 {
				m.Correlated++
			} else {
				m.Uncorrelated++
			}
		}
	}
	return m
}
