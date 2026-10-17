package validate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// WithRelations gives the reference checks (RenderDiff, StaticReference) the
// relationship rules of the graph builder — relations.WithCatalog(…) — so
// declared references (an Issuer's CA Secret, a monitor's credentials) are
// checked like the built-in ones. Without it only the built-in Kubernetes
// references are known.
func WithRelations(opts ...relations.Option) Option {
	return func(c *config) { c.relations = append(c.relations, opts...) }
}

// views caches the FROM and TO views of the last bundle a validator saw
// (one bundle per run; a validator is called once per item).
type views struct {
	mu       sync.Mutex
	bundle   string
	from, to *relations.View
	err      error
}

// sides returns the environment at both sides of the upgrade (needs a store).
func (c config) sides(ctx context.Context, b *domain.EvidenceBundle) (from, to *relations.View, err error) {
	if c.store == nil {
		return nil, nil, errNoStore
	}
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	if c.cache.bundle != b.ID || (c.cache.err != nil && ctx.Err() == nil) {
		v := relations.NewViewer(c.store, c.relations...)
		c.cache.bundle, c.cache.from, c.cache.to, c.cache.err = b.ID, nil, nil, nil
		if c.cache.from, c.cache.err = v.View(ctx, b, relations.SideFrom); c.cache.err == nil {
			c.cache.to, c.cache.err = v.View(ctx, b, relations.SideTo)
		}
	}
	return c.cache.from, c.cache.to, c.cache.err
}

// refState is what the upgrade does to one reference of the TO render.
type refState string

const (
	refNew         refState = "new"          // dangles at TO, absent at FROM: the upgrade introduces it
	refRegressed   refState = "regressed"    // resolved at FROM, dangles at TO
	refPreexisting refState = "pre-existing" // dangled at FROM too: not the upgrade's doing
	refUnknown     refState = "unknown"      // undecidable at TO (or at FROM for a TO dangle)
	refResolved    refState = "resolved"
)

type refCheck struct {
	ref   relations.Reference // at TO
	from  *relations.Reference
	state refState
}

func (r refCheck) failing() bool { return r.state == refNew || r.state == refRegressed }

func (r refCheck) line() string {
	ref := r.ref
	where := "an environment file"
	if ref.Rendered {
		where = "the TO render"
	}
	s := fmt.Sprintf("%s (%s) %s → %s [%s]: ", ref.Referrer, where, ref.Field, ref.TargetString(), ref.Rule)
	switch r.state {
	case refNew:
		s += "a reference the upgrade introduces to a target nothing referenced at FROM, and nothing at TO provides it (not rendered, not an environment file, no declared producer)"
	case refRegressed:
		s += fmt.Sprintf("resolved at FROM (to %s); at TO nothing provides it (not rendered, not an environment file, no declared producer)", r.from.Target)
	case refPreexisting:
		s += "dangled at FROM too (the environment provides it outside the snapshot — e.g. a hook or controller creates it at runtime — or it was already missing): not the upgrade's doing"
	case refUnknown:
		s += "cannot be decided: " + firstNonEmpty(ref.Reason, fromReason(r.from))
	case refResolved:
		s += "resolves"
		if ref.Reason != "" {
			s += " (" + ref.Reason + ")"
		}
	}
	return s
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// compareRefs classifies every TO reference that relevant selects by what
// the FROM render says about its target (kind, namespace, name — not the
// field or list position, which a reordering upgrade shifts): a target some
// FROM reference resolved and TO no longer provides is regressed; one FROM
// references dangled at too is pre-existing (the environment provides it
// outside the snapshot — a hook or a controller creates it at runtime — or it
// was already missing); one no FROM reference names is new.
func compareRefs(from, to *relations.View, relevant func(relations.Reference) bool) []refCheck {
	rank := map[relations.Resolution]int{relations.RefResolved: 3, relations.RefDangling: 2, relations.RefUnknown: 1}
	atFrom := map[string]*relations.Reference{} // target → the most telling FROM reference to it
	for i := range from.References {
		r := &from.References[i]
		k := r.TargetIdentity()
		if cur := atFrom[k]; cur == nil || rank[r.Resolution] > rank[cur.Resolution] {
			atFrom[k] = r
		}
	}
	var out []refCheck
	for _, r := range to.References {
		if !relevant(r) {
			continue
		}
		c := refCheck{ref: r, from: atFrom[r.TargetIdentity()]}
		switch r.Resolution {
		case relations.RefResolved:
			c.state = refResolved
		case relations.RefUnknown:
			c.state = refUnknown
		default:
			switch {
			case c.from == nil:
				c.state = refNew
			case c.from.Resolution == relations.RefResolved:
				c.state = refRegressed
			case c.from.Resolution == relations.RefDangling:
				c.state = refPreexisting
			default:
				c.state = refUnknown // undecidable at FROM: whether the upgrade broke it is unknown
			}
		}
		out = append(out, c)
	}
	return out
}

// touching selects references whose referrer is one of the objects, or whose
// target names one of them (a removed object keeps its key in the rendered
// change).
func touching(objects []string) func(relations.Reference) bool {
	set := map[string]bool{}
	for _, o := range objects {
		set[o] = true
	}
	return func(r relations.Reference) bool {
		if set[r.Referrer] {
			return true
		}
		for _, o := range objects {
			if r.TargetMatches(o) {
				return true
			}
		}
		return false
	}
}

// objectsOf lists the objects a reference check involves (referrer, and the
// target key when it resolved at FROM).
func (r refCheck) objects() []string {
	out := []string{r.ref.Referrer}
	if r.from != nil && r.from.Target != "" {
		out = append(out, r.from.Target)
	}
	return out
}

type refCheckList []refCheck

func (l refCheckList) by(states ...refState) refCheckList {
	var out refCheckList
	for _, c := range l {
		for _, s := range states {
			if c.state == s {
				out = append(out, c)
			}
		}
	}
	return out
}

func (l refCheckList) lines() string {
	var ls []string
	for _, c := range l {
		ls = append(ls, c.line())
	}
	sort.Strings(ls)
	return "- " + strings.Join(ls, "\n- ")
}

func (l refCheckList) objects() []string {
	var out []string
	for _, c := range l {
		out = append(out, c.objects()...)
	}
	return uniq(out)
}

func fromReason(r *relations.Reference) string {
	if r == nil {
		return ""
	}
	return "at FROM: " + r.Reason
}
