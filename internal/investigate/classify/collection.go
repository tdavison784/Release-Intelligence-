package classify

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// deciding is the declarative R2.1 table (DESIGN.md §2.2 "Family decides the
// deciding inputs and validators"): which change sources, which environment
// inputs and whether the environment render decide an item of the family.
type deciding struct {
	// sources are the change-evidence kinds the family's items are computed
	// from; each must be ok or not-applicable.
	sources []domain.ChangeSourceKind
	// inputs are the environment input kinds that can decide the family: at
	// least one must be present, and every present one ok or not-applicable.
	inputs []domain.SnapshotInputKind
	// render: the family is render-decided — every environment render target
	// of the product must be complete (not-applicable targets do not deploy
	// the product), and at least one must exist.
	render bool
	// validators names the validator kinds that can decide the family (for
	// next steps).
	validators []string
}

var decidingInputs = map[domain.ChangeItemFamily]deciding{
	domain.FamilyValues: {
		sources: []domain.ChangeSourceKind{domain.ChangeSourceChart},
		inputs:  []domain.SnapshotInputKind{domain.InputGitOpsRepo, domain.InputValues},
		render:  true, validators: []string{"render-diff", "override-inspection"},
	},
	domain.FamilyImage: {
		sources: []domain.ChangeSourceKind{domain.ChangeSourceChart},
		inputs:  []domain.SnapshotInputKind{domain.InputGitOpsRepo, domain.InputValues, domain.InputManifests, domain.InputImages},
		render:  true, validators: []string{"render-diff", "override-inspection"},
	},
	domain.FamilyRendered: {
		sources: []domain.ChangeSourceKind{domain.ChangeSourceRenderedManifests},
		render:  true, validators: []string{"render-diff"},
	},
	domain.FamilyAPISchema: {
		sources: []domain.ChangeSourceKind{domain.ChangeSourceAPISchema},
		inputs:  []domain.SnapshotInputKind{domain.InputGitOpsRepo, domain.InputManifests, domain.InputCRDs},
		render:  true, validators: []string{"schema-compat", "static-reference"},
	},
	domain.FamilyCompatibility: {
		sources: []domain.ChangeSourceKind{domain.ChangeSourceCompatibility},
		// inputs depend on the anchor: see compatInputs.
		validators: []string{"schema-compat"},
	},
	// behavior / procedure: no input decides them alone (§2.2); only a
	// clearing validation can (R2.4).
	domain.FamilyBehavior:  {validators: []string{"static-reference", "override-inspection (pinned setting)", "runtime (observed)"}},
	domain.FamilyProcedure: {validators: []string{"static-reference", "override-inspection (pinned setting)", "runtime (observed)"}},
}

// compatInputs: a Kubernetes platform item is decided by the cluster
// version; any other platform (a peer product) by the inventory.
func compatInputs(platform string) []domain.SnapshotInputKind {
	p, _, _ := strings.Cut(platform, "/")
	if strings.EqualFold(p, "kubernetes") {
		return []domain.SnapshotInputKind{domain.InputClusterVersion}
	}
	return []domain.SnapshotInputKind{domain.InputInventory}
}

func (x *index) decidingFor(it domain.ChangeItem) deciding {
	d := decidingInputs[it.Family]
	if it.Family == domain.FamilyCompatibility {
		d.inputs = compatInputs(it.Platform)
	}
	return d
}

// collection is R2.1: every source and input that decides the item's family
// is collected (ok or not-applicable); for render-decided families, every
// environment render target of the product is complete.
func (x *index) collection(e *itemEval) {
	d := x.decidingFor(e.item)
	for _, k := range d.sources {
		s, ok := x.b.Change.SourceStatus(k)
		switch {
		case !ok:
			e.gap(domain.GapCollection, string(k), fmt.Sprintf("change source %s is not stated in the change record", k), domain.UnknownReleaseKnowledgeGap,
				fmt.Sprintf("Collect the upstream %s source.", k))
		case s.Status.Gap():
			e.gap(domain.GapCollection, string(k), fmt.Sprintf("change source %s that decides a %s item is %s: %s", k, e.item.Family, s.Status, orDash(s.Reason)),
				domain.UnknownReleaseKnowledgeGap, fmt.Sprintf("Re-collect the upstream %s source (%s: %s).", k, s.Status, orDash(s.Reason)))
		}
	}
	if len(d.inputs) > 0 {
		present := false
		for _, in := range x.b.Snapshot.Inputs {
			if !kindIn(in.Kind, d.inputs) {
				continue
			}
			present = true
			if in.Status.Gap() {
				what := in.Path + in.Value
				e.gap(domain.GapCollection, string(in.Kind)+":"+what, fmt.Sprintf("environment input %s %s that decides a %s item is %s: %s", in.Kind, what, e.item.Family, in.Status, orDash(in.Reason)),
					domain.UnknownEnvironmentVisibilityGap, fmt.Sprintf("Repair or complete the environment input %s %s (%s: %s).", in.Kind, what, in.Status, orDash(in.Reason)))
			}
		}
		if !present {
			e.gap(domain.GapExposure, kindList(d.inputs), fmt.Sprintf("no environment input that decides a %s item was supplied (one of: %s)", e.item.Family, kindList(d.inputs)),
				domain.UnknownEnvironmentVisibilityGap, fmt.Sprintf("Supply the environment input that decides this %s item (one of: %s).", e.item.Family, kindList(d.inputs)))
		}
	}
	if d.render {
		complete := false
		for _, p := range x.b.Renders {
			switch p.Status {
			case domain.CollectionOK:
				complete = true
			case domain.CollectionNotApplicable:
			default:
				e.gap(domain.GapCollection, "render:"+p.Target, fmt.Sprintf("environment render %s is %s: %s (an incomplete render is an evidence gap, never \"no change\")", p.Target, p.Status, orDash(p.Reason)),
					domain.UnknownEvidenceGap, fmt.Sprintf("Complete the environment render of %s (%s: %s).", p.Target, p.Status, orDash(p.Reason)))
			}
		}
		if !complete {
			detail := "no complete environment render of the product exists"
			if len(x.b.Renders) == 0 {
				detail = "the environment was not rendered (no render pair)"
			}
			e.gap(domain.GapCollection, "render", detail+fmt.Sprintf(": a %s item is decided by the rendered configuration", e.item.Family),
				domain.UnknownEvidenceGap, "Render the environment's from and to configuration completely (environment renders enabled, every values layer available).")
		}
	}
}

func kindIn(k domain.SnapshotInputKind, ks []domain.SnapshotInputKind) bool {
	for _, x := range ks {
		if x == k {
			return true
		}
	}
	return false
}

func kindList(ks []domain.SnapshotInputKind) string {
	var s []string
	for _, k := range ks {
		s = append(s, string(k))
	}
	return strings.Join(s, ", ")
}
