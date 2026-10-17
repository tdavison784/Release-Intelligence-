package catalog

import (
	"fmt"
	"net/url"
	"strings"
)

// Reconcile declares object kinds the product's controller reconciles: every
// object of those kinds in an environment is acted on by the controller the
// product ships, so a change compiled into that controller (a behaviour
// default, server-side defaulting) reaches each of them. The investigation's
// exposure mapper roots controller-behaviour items there
// (internal/investigate/relations). A release-level fact: References are
// required and must be upstream documentation, never an evaluation case.
type Reconcile struct {
	ID string `yaml:"id" json:"id"`
	// Kinds are the reconciled object kinds (group "" is the core group).
	Kinds   []ObjectType `yaml:"kinds" json:"kinds"`
	Summary string       `yaml:"summary" json:"summary"`
	// References are the upstream documentation the entry rests on.
	References []ConfigReference `yaml:"references" json:"references"`
}

func (v *validator) reconciles(d *ProductDefinition) {
	seen := map[string]bool{}
	kinds := map[string]string{}
	for i, r := range d.Reconciles {
		p := fmt.Sprintf("reconciles[%d]", i)
		switch {
		case !configSourceIDRe.MatchString(r.ID):
			v.errf(p+".id", "must be a lowercase DNS label, got %q", r.ID)
		case seen[r.ID]:
			v.errf(p+".id", "duplicate reconciles id %q", r.ID)
		}
		seen[r.ID] = true
		if len(r.Kinds) == 0 {
			v.errf(p+".kinds", "at least one kind is required")
		}
		for j, k := range r.Kinds {
			kp := fmt.Sprintf("%s.kinds[%d]", p, j)
			if strings.TrimSpace(k.Kind) == "" {
				v.errf(kp+".kind", "required")
				continue
			}
			key := k.Group + "/" + k.Kind
			if prev, dup := kinds[key]; dup {
				v.errf(kp, "kind %s is already declared by %s", key, prev)
			}
			kinds[key] = p
		}
		if strings.TrimSpace(r.Summary) == "" {
			v.errf(p+".summary", "required")
		}
		v.upstreamReferences(p, r.References)
	}
}

// upstreamReferences checks a construct's citations: at least one https URL
// of upstream documentation (never evaluation data), each with a short
// verbatim quote.
func (v *validator) upstreamReferences(p string, refs []ConfigReference) {
	if len(refs) == 0 {
		v.errf(p+".references", "at least one upstream documentation reference is required")
	}
	for j, ref := range refs {
		rp := fmt.Sprintf("%s.references[%d]", p, j)
		u, err := url.Parse(ref.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			v.errf(rp+".url", "must be an https URL of upstream documentation, got %q", ref.URL)
		}
		if strings.Contains(ref.URL, "eval/cases") || strings.Contains(ref.URL, "eval/results") || strings.Contains(ref.URL, "eval/investigations") {
			v.errf(rp+".url", "must not cite evaluation data")
		}
		if q := strings.TrimSpace(ref.Quote); q == "" || len(q) > maxConfigQuote {
			v.errf(rp+".quote", "a short verbatim quote (1-%d characters) is required", maxConfigQuote)
		}
	}
}

// ReconciledKinds returns the kinds the product's controller reconciles.
func (d *ProductDefinition) ReconciledKinds() []ObjectType {
	var out []ObjectType
	for _, r := range d.Reconciles {
		out = append(out, r.Kinds...)
	}
	return out
}
