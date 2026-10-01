package stats

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

// Default input locations, relative to the repository root.
const (
	DefaultProductsDir = "products"
	DefaultRecordsDir  = "docs/onboarding/records"
	DefaultChecksDir   = "docs/onboarding/checks"
)

// Inputs is everything the report is computed from.
type Inputs struct {
	// Products are the computed stats of every loaded definition.
	Products []ProductStats
	// Records are the onboarding records.
	Records []*Record
	// Checks are the saved relationship reports keyed by product id.
	Checks map[string]*CheckSummary
	// Warnings collects what could not be read (bad files, load errors).
	Warnings []string
}

// Collect reads the definitions, records and check reports. A missing
// records or checks directory is not an error (nothing has been recorded
// yet); a definition, record or report that cannot be read is reported as a
// warning and skipped.
func Collect(productsDir, recordsDir, checksDir string) (*Inputs, error) {
	in := &Inputs{}
	cat, err := catalog.LoadDir(productsDir)
	if err != nil {
		return nil, fmt.Errorf("products: %w", err)
	}
	for _, d := range cat.List() {
		in.Products = append(in.Products, Analyze(d, nil))
	}
	errs := cat.LoadErrors()
	for _, p := range cat.LoadErrorPaths() {
		in.Warnings = append(in.Warnings, fmt.Sprintf("%s: definition not loaded: %v", p, errs[p]))
	}
	recs, warns, err := LoadRecords(recordsDir)
	if err != nil {
		return nil, fmt.Errorf("records: %w", err)
	}
	in.Records, in.Warnings = recs, append(in.Warnings, warns...)
	checks, warns, err := LoadChecks(checksDir)
	if err != nil {
		return nil, fmt.Errorf("checks: %w", err)
	}
	in.Checks, in.Warnings = checks, append(in.Warnings, warns...)
	return in, nil
}

// DiscoveryShare counts where the sources and artifacts of a definition came
// from, according to its record.
type DiscoveryShare struct {
	Discovered         int `json:"discovered"`         // proposed by discovery, kept as proposed
	DiscoveredModified int `json:"discoveredModified"` // proposed, then corrected by hand
	Manual             int `json:"manual"`             // found by research
	Other              int `json:"other,omitempty"`    // an origin the report does not know
	Unclassified       int `json:"unclassified,omitempty"`
}

// Total is the number of classified items.
func (d DiscoveryShare) Total() int {
	return d.Discovered + d.DiscoveredModified + d.Manual + d.Other
}

// Found is the number of items discovery proposed (kept or corrected).
func (d DiscoveryShare) Found() int { return d.Discovered + d.DiscoveredModified }

// Percent is the share of items discovery proposed, or -1 when nothing is
// classified.
func (d DiscoveryShare) Percent() float64 {
	if d.Total() == 0 {
		return -1
	}
	return 100 * float64(d.Found()) / float64(d.Total())
}

func (d *DiscoveryShare) add(o DiscoveryShare) {
	d.Discovered += o.Discovered
	d.DiscoveredModified += o.DiscoveredModified
	d.Manual += o.Manual
	d.Other += o.Other
	d.Unclassified += o.Unclassified
}

// ProductRow is one product in the report.
type ProductRow struct {
	Product       string `json:"product"`
	Name          string `json:"name,omitempty"`
	Order         int    `json:"order,omitempty"` // 0: unknown (no record, or none given)
	Wave          *int   `json:"wave,omitempty"`
	HasDefinition bool   `json:"hasDefinition"`
	HasRecord     bool   `json:"hasRecord"`

	// Size and complexity of the definition (zero without a definition).
	Size

	// Constructs. Used is computed from the definition; New comes from the
	// record (authoritative). Reused are used constructs introduced by an
	// earlier product; RetroAdopted ones introduced by a later product (the
	// definition was migrated after the construct appeared); Unaccounted
	// ones no record introduces. Cumulative is the number of distinct
	// constructs introduced up to and including this product.
	Used         int      `json:"used"`
	New          int      `json:"new"`
	Reused       int      `json:"reused"`
	RetroAdopted int      `json:"retroAdopted,omitempty"`
	Unaccounted  int      `json:"unaccounted,omitempty"`
	Cumulative   int      `json:"cumulative"`
	ZeroNew      bool     `json:"zeroNew"` // has a record and introduced nothing
	NewList      []string `json:"newConstructs,omitempty"`
	// UnaccountedList names the used constructs that no record introduces:
	// for a product being onboarded, the candidates for its constructs.new.
	UnaccountedList []string `json:"unaccountedConstructs,omitempty"`
	// NewDetails are the record's justifications of new constructs, by name.
	NewDetails map[string]ConstructRef `json:"newConstructDetails,omitempty"`
	UsedList   []string                `json:"usedConstructs,omitempty"`

	// FirstUse and CumulativeUsed are computed from definitions alone (no
	// records): how many used constructs no earlier product used, and the
	// number of distinct constructs used up to this product.
	FirstUse       int `json:"firstUse"`
	CumulativeUsed int `json:"cumulativeUsed"`

	GoGeneric         int      `json:"goGeneric"`
	GoProductSpecific int      `json:"goProductSpecific"`
	Minutes           *float64 `json:"minutes,omitempty"`

	Discovery DiscoveryShare `json:"discovery"`

	Check               *CheckSummary `json:"check,omitempty"` // nil: no saved report
	InitialFailures     *int          `json:"initialFailures,omitempty"`
	FinalFailures       *int          `json:"finalFailures,omitempty"`
	ManualInterventions int           `json:"manualInterventions"`

	UnreachableSources int      `json:"unreachableSources"`
	UnreachableHosts   []string `json:"unreachableHosts,omitempty"`
	Representability   string   `json:"representability,omitempty"`
	Gaps               int      `json:"gaps"`
	Notes              string   `json:"notes,omitempty"`
}

// CurvePoint is one step of the construct-introduction curve.
type CurvePoint struct {
	Order          int    `json:"order"`
	Product        string `json:"product"`
	HasRecord      bool   `json:"hasRecord"`
	New            int    `json:"new"`
	Cumulative     int    `json:"cumulative"`
	Used           int    `json:"used"`
	Reused         int    `json:"reused"`
	FirstUse       int    `json:"firstUse"`
	CumulativeUsed int    `json:"cumulativeUsed"`
}

// WaveRow aggregates the recorded products of one onboarding wave.
type WaveRow struct {
	Wave     *int     `json:"wave,omitempty"` // nil: records without a wave
	Products []string `json:"products"`

	ZeroNew         int      `json:"zeroNew"` // products that needed no new construct
	ZeroNewProducts []string `json:"zeroNewProducts,omitempty"`
	NewConstructs   int      `json:"newConstructs"`
	NewPerProduct   float64  `json:"newPerProduct"`
	Used            int      `json:"used"`
	Reused          int      `json:"reused"`

	GoGeneric         int      `json:"goGeneric"`
	GoProductSpecific int      `json:"goProductSpecific"`
	MedianMinutes     *float64 `json:"medianMinutes,omitempty"`
	MinutesKnown      int      `json:"minutesKnown"`

	Discovery DiscoveryShare `json:"discovery"`

	Reports      int `json:"checkReports"` // products with a saved relationship report
	Validated    int `json:"validated"`
	Failing      int `json:"failing"`
	Insufficient int `json:"insufficient"`
	Unverifiable int `json:"unverifiable"`

	UnreachableSources int            `json:"unreachableSources"`
	Representability   map[string]int `json:"representability,omitempty"`
}

// Summary is the headline of the report.
type Summary struct {
	Definitions  int `json:"definitions"`
	Records      int `json:"records"`
	CheckReports int `json:"checkReports"`

	// DistinctUsed is the number of distinct constructs any definition uses;
	// DistinctIntroduced the number any record introduces.
	DistinctUsed       int `json:"distinctUsed"`
	DistinctIntroduced int `json:"distinctIntroduced"`

	ZeroNewProducts int `json:"zeroNewProducts"`
	// LastNewOrder is the order of the latest product that introduced a
	// construct (0: none).
	LastNewOrder int `json:"lastNewOrder"`
	// GoProductSpecific is the total number of product-specific Go changes
	// (must stay 0).
	GoProductSpecific int `json:"goProductSpecific"`
}

// Report is the full measurement.
type Report struct {
	Summary  Summary      `json:"summary"`
	Products []ProductRow `json:"products"`
	Curve    []CurvePoint `json:"curve"`
	Waves    []WaveRow    `json:"waves"`
	Warnings []string     `json:"warnings,omitempty"`
}

// Build computes the report. Products are ordered by their record's order
// (products without a record or order follow, by id); the construct curve and
// the cumulative counts follow that order.
func Build(in *Inputs) *Report {
	rep := &Report{Warnings: append([]string(nil), in.Warnings...)}
	warn := func(format string, a ...any) { rep.Warnings = append(rep.Warnings, fmt.Sprintf(format, a...)) }

	defs := map[string]ProductStats{}
	for _, p := range in.Products {
		defs[p.Product] = p
	}
	recs := map[string]*Record{}
	for _, r := range in.Records {
		if prev, dup := recs[r.Product]; dup {
			warn("%s: second record for %q (first: %s), ignored", r.File, r.Product, prev.File)
			continue
		}
		recs[r.Product] = r
	}

	// every known product id
	ids := map[string]bool{}
	for id := range defs {
		ids[id] = true
	}
	for id := range recs {
		ids[id] = true
	}
	for id := range in.Checks {
		if defs[id].Product == "" && recs[id] == nil {
			warn("check report for %q has no definition and no record", id)
		}
	}
	order := make([]string, 0, len(ids))
	for id := range ids {
		order = append(order, id)
	}
	sort.Slice(order, func(i, j int) bool {
		oi, oj := orderOf(recs[order[i]]), orderOf(recs[order[j]])
		if (oi == 0) != (oj == 0) {
			return oi != 0 // ordered products first
		}
		if oi != oj {
			return oi < oj
		}
		return order[i] < order[j]
	})
	seenOrder := map[int]string{}
	for _, id := range order {
		if o := orderOf(recs[id]); o != 0 {
			if other, dup := seenOrder[o]; dup {
				warn("products %q and %q both have order %d", other, id, o)
			}
			seenOrder[o] = id
		}
	}

	// who introduced each construct (the first record in order wins)
	type intro struct {
		product string
		order   int
	}
	introducedBy := map[string]intro{}
	newBy := map[string][]string{} // product → constructs it truly introduced
	for _, id := range order {
		r := recs[id]
		if r == nil {
			continue
		}
		seen := map[string]bool{}
		for _, c := range r.Constructs.New {
			name := strings.TrimSpace(c.Name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			if first, dup := introducedBy[name]; dup {
				warn("%s: construct %q is listed as new, but %q introduced it first; counted once", id, name, first.product)
				continue
			}
			introducedBy[name] = intro{id, orderOf(r)}
			newBy[id] = append(newBy[id], name)
		}
		sort.Strings(newBy[id])
	}

	cumulative := 0
	usedSoFar := map[string]bool{}
	allUsed := map[string]bool{}
	for _, id := range order {
		def, hasDef := defs[id]
		r := recs[id]
		row := ProductRow{Product: id, HasDefinition: hasDef, HasRecord: r != nil}
		if hasDef {
			row.Name, row.Size, row.UsedList = def.Name, def.Size, def.Constructs
			row.Used = len(def.Constructs)
		}
		if r != nil {
			row.Order = orderOf(r)
			row.Wave = r.Wave
			row.Notes = strings.TrimSpace(r.Notes)
			row.Minutes = r.Onboarding.Minutes
			row.GoGeneric = len(r.GoChanges.Generic)
			row.GoProductSpecific = r.GoChanges.ProductSpecific
			row.Representability = strings.ToLower(strings.TrimSpace(r.Representability))
			row.Gaps = len(r.Gaps)
			row.InitialFailures, row.FinalFailures = r.Relationships.InitialFailures, r.Relationships.FinalFailures
			row.ManualInterventions = len(r.Relationships.ManualInterventions)
			for _, u := range r.UnreachableSources {
				row.UnreachableSources++
				if u.Host != "" {
					row.UnreachableHosts = append(row.UnreachableHosts, u.Host)
				}
			}
			row.NewList = newBy[id]
			for _, c := range r.Constructs.New {
				name := strings.TrimSpace(c.Name)
				if introducedBy[name].product != id || (c.Justification == "" && len(c.OtherProducts) == 0) {
					continue
				}
				if row.NewDetails == nil {
					row.NewDetails = map[string]ConstructRef{}
				}
				row.NewDetails[name] = c
			}
			row.New = len(row.NewList)
			row.ZeroNew = row.New == 0
			row.Discovery = discoveryShare(r, def, hasDef, warn)
			if r.GoChanges.ProductSpecific > 0 {
				warn("%s: %d product-specific Go change(s) recorded (the rule is 0)", id, r.GoChanges.ProductSpecific)
			}
			if hasDef {
				used := toSet(def.Constructs)
				for _, c := range r.Constructs.New {
					if name := strings.TrimSpace(c.Name); name != "" && !used[name] {
						warn("%s: listed as a new construct but the definition does not use %q (typo, renamed, or retired?)", id, name)
					}
				}
			}
		}
		if cs, ok := in.Checks[id]; ok {
			row.Check = cs
		}
		// classify the used constructs
		myOrder := row.Order
		var unaccounted, retro []string
		for _, c := range row.UsedList {
			by, ok := introducedBy[c]
			switch {
			case !ok:
				row.Unaccounted++
				unaccounted = append(unaccounted, c)
			case by.product == id:
				// introduced here
			case myOrder == 0 || by.order < myOrder:
				row.Reused++
			default:
				row.RetroAdopted++
				retro = append(retro, c+" (introduced by "+by.product+")")
			}
		}
		row.UnaccountedList = unaccounted
		if r != nil && len(retro) > 0 {
			warn("%s uses constructs introduced by a later product (the definition was migrated after they appeared): %s", id, strings.Join(retro, ", "))
		}
		if r != nil && len(unaccounted) > 0 {
			warn("%s uses %d construct(s) that no record introduces: %s", id, len(unaccounted), summarizeList(unaccounted, 8))
		}
		// curves
		cumulative += row.New
		row.Cumulative = cumulative
		for _, c := range row.UsedList {
			if !usedSoFar[c] {
				usedSoFar[c] = true
				row.FirstUse++
			}
			allUsed[c] = true
		}
		row.CumulativeUsed = len(usedSoFar)
		rep.Products = append(rep.Products, row)
		rep.Curve = append(rep.Curve, CurvePoint{
			Order: row.Order, Product: id, HasRecord: row.HasRecord, New: row.New,
			Cumulative: row.Cumulative, Used: row.Used, Reused: row.Reused,
			FirstUse: row.FirstUse, CumulativeUsed: row.CumulativeUsed,
		})
	}

	rep.Waves = waves(rep.Products)
	rep.Summary = summarize(rep, allUsed, len(introducedBy))
	return rep
}

func orderOf(r *Record) int {
	if r == nil || r.Order < 0 {
		return 0
	}
	return r.Order
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

func summarizeList(ss []string, max int) string {
	if len(ss) <= max {
		return strings.Join(ss, ", ")
	}
	return strings.Join(ss[:max], ", ") + fmt.Sprintf(", ... (%d more)", len(ss)-max)
}

// discoveryShare classifies the sources and artifacts of the definition by
// the origins the record lists. Items of the definition the record does not
// mention count as unclassified; listed items missing from the definition are
// ignored (with a warning).
func discoveryShare(r *Record, def ProductStats, hasDef bool, warn func(string, ...any)) DiscoveryShare {
	var ds DiscoveryShare
	count := func(kind string, entries []OriginEntry, defIDs []string) {
		inDef := toSet(defIDs)
		listed := map[string]bool{}
		for _, e := range entries {
			if hasDef && !inDef[e.ID] {
				warn("%s: record lists %s %q which is not in the definition", r.Product, kind, e.ID)
				continue
			}
			if listed[e.ID] {
				continue
			}
			listed[e.ID] = true
			switch strings.ToLower(strings.TrimSpace(e.Origin)) {
			case OriginDiscovered:
				ds.Discovered++
			case OriginDiscoveredModified:
				ds.DiscoveredModified++
			case OriginManual:
				ds.Manual++
			default:
				ds.Other++
			}
		}
		if hasDef {
			for _, id := range defIDs {
				if !listed[id] {
					ds.Unclassified++
				}
			}
		}
	}
	count("source", r.Sources, def.SourceIDs)
	count("artifact", r.Artifacts, def.ArtifactIDs)
	return ds
}

func waves(rows []ProductRow) []WaveRow {
	byWave := map[int]*WaveRow{} // key: wave, -1 = none
	keys := []int{}
	minutes := map[int][]float64{}
	for _, row := range rows {
		if !row.HasRecord {
			continue
		}
		k := -1
		if row.Wave != nil {
			k = *row.Wave
		}
		w := byWave[k]
		if w == nil {
			w = &WaveRow{Representability: map[string]int{}}
			if k >= 0 {
				kk := k
				w.Wave = &kk
			}
			byWave[k] = w
			keys = append(keys, k)
		}
		w.Products = append(w.Products, row.Product)
		if row.ZeroNew {
			w.ZeroNew++
			w.ZeroNewProducts = append(w.ZeroNewProducts, row.Product)
		}
		w.NewConstructs += row.New
		w.Used += row.Used
		w.Reused += row.Reused
		w.GoGeneric += row.GoGeneric
		w.GoProductSpecific += row.GoProductSpecific
		if row.Minutes != nil {
			minutes[k] = append(minutes[k], *row.Minutes)
		}
		w.Discovery.add(row.Discovery)
		if row.Check != nil {
			w.Reports++
			w.Validated += row.Check.Validated
			w.Failing += row.Check.Failing
			w.Insufficient += row.Check.Insufficient
			w.Unverifiable += row.Check.Unverifiable
		}
		w.UnreachableSources += row.UnreachableSources
		repr := row.Representability
		if repr == "" {
			repr = "unknown"
		}
		w.Representability[repr]++
	}
	sort.Ints(keys)
	var out []WaveRow
	for _, k := range keys {
		w := byWave[k]
		w.NewPerProduct = float64(w.NewConstructs) / float64(len(w.Products))
		if m := minutes[k]; len(m) > 0 {
			med := median(m)
			w.MedianMinutes = &med
			w.MinutesKnown = len(m)
		}
		out = append(out, *w)
	}
	return out
}

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func summarize(rep *Report, allUsed map[string]bool, introduced int) Summary {
	s := Summary{DistinctUsed: len(allUsed), DistinctIntroduced: introduced}
	for _, row := range rep.Products {
		if row.HasDefinition {
			s.Definitions++
		}
		if row.Check != nil {
			s.CheckReports++
		}
		if !row.HasRecord {
			continue
		}
		s.Records++
		if row.ZeroNew {
			s.ZeroNewProducts++
		}
		if row.New > 0 && row.Order > s.LastNewOrder {
			s.LastNewOrder = row.Order
		}
		s.GoProductSpecific += row.GoProductSpecific
	}
	return s
}

// Filter returns a copy of the report restricted to the given products. The
// order, the cumulative counts and the introduced-by relations always come
// from the full set; the curve, the table and the wave aggregates show only
// the selected products. An unknown id is reported as a warning.
func (r *Report) Filter(ids []string) *Report {
	if len(ids) == 0 {
		return r
	}
	want := toSet(ids)
	out := &Report{Summary: r.Summary, Warnings: append([]string(nil), r.Warnings...)}
	known := map[string]bool{}
	for _, row := range r.Products {
		known[row.Product] = true
		if want[row.Product] {
			out.Products = append(out.Products, row)
		}
	}
	for _, c := range r.Curve {
		if want[c.Product] {
			out.Curve = append(out.Curve, c)
		}
	}
	for _, id := range ids {
		if !known[id] {
			out.Warnings = append(out.Warnings, fmt.Sprintf("unknown product %q (no definition, record or check report)", id))
		}
	}
	out.Waves = waves(out.Products)
	return out
}
