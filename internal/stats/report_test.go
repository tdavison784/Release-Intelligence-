package stats

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixtureInputs(t *testing.T) *Inputs {
	t.Helper()
	in, err := Collect(filepath.Join("testdata", "products"), filepath.Join("testdata", "records"), filepath.Join("testdata", "checks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Warnings) != 0 {
		t.Fatalf("fixture warnings: %v", in.Warnings)
	}
	return in
}

func rowFor(t *testing.T, rep *Report, id string) ProductRow {
	t.Helper()
	for _, r := range rep.Products {
		if r.Product == id {
			return r
		}
	}
	t.Fatalf("no row for %s", id)
	return ProductRow{}
}

func TestBuildConvergence(t *testing.T) {
	rep := Build(fixtureInputs(t))
	var order []string
	for _, r := range rep.Products {
		order = append(order, r.Product)
	}
	if !reflect.DeepEqual(order, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("order: %v", order)
	}
	a, b, g := rowFor(t, rep, "alpha"), rowFor(t, rep, "beta"), rowFor(t, rep, "gamma")

	// new comes from the records, used from the definitions
	if a.New != a.Used || a.Reused != 0 || a.Cumulative != a.New {
		t.Errorf("alpha: used %d new %d reused %d cumulative %d", a.Used, a.New, a.Reused, a.Cumulative)
	}
	if b.New != 20 || b.Used != b.New+b.Reused || b.Reused < 10 || b.Cumulative != a.New+b.New {
		// beta reuses the base constructs alpha introduced
		t.Errorf("beta: used %d new %d reused %d cumulative %d", b.Used, b.New, b.Reused, b.Cumulative)
	}
	if !g.ZeroNew || g.New != 0 || g.Reused != g.Used || g.Cumulative != b.Cumulative {
		t.Errorf("gamma: %+v", g)
	}
	if a.ZeroNew || b.ZeroNew {
		t.Error("alpha and beta introduced constructs")
	}
	if a.Unaccounted+b.Unaccounted+g.Unaccounted+a.RetroAdopted+b.RetroAdopted+g.RetroAdopted != 0 {
		t.Errorf("clean fixture must have no anomalies: %+v %+v %+v", a, b, g)
	}

	// the computed curve needs no records: gamma uses nothing new
	var curve []int
	for _, c := range rep.Curve {
		curve = append(curve, c.FirstUse)
	}
	if curve[0] != a.Used || curve[2] != 0 || rep.Curve[2].CumulativeUsed != rep.Curve[1].CumulativeUsed {
		t.Errorf("computed curve: %v", curve)
	}

	// effort
	if a.Minutes == nil || *a.Minutes != 120 || a.GoGeneric != 1 || b.GoGeneric != 2 || a.GoProductSpecific != 0 {
		t.Errorf("effort: %+v %+v", a, b)
	}
	if a.ManualInterventions != 1 || a.UnreachableSources != 1 || a.UnreachableHosts[0] != "quay.io" ||
		a.InitialFailures == nil || *a.InitialFailures != 3 || *a.FinalFailures != 0 {
		t.Errorf("relationship effort: %+v", a)
	}

	// discovery: alpha has 2 discovered, 1 modified... of 5 elements
	if want := (DiscoveryShare{Discovered: 2, DiscoveredModified: 1, Manual: 2}); a.Discovery != want {
		t.Errorf("alpha discovery: %+v", a.Discovery)
	}
	if got := a.Discovery.Percent(); got != 60 {
		t.Errorf("alpha discovery share = %v", got)
	}
	if b.Discovery.Percent() != 75 || g.Discovery.Percent() != 100 {
		t.Errorf("beta/gamma discovery share: %v %v", b.Discovery.Percent(), g.Discovery.Percent())
	}

	// relationship reports
	if a.Check == nil || a.Check.Validated != 1 || b.Check == nil || b.Check.Validated != 1 || g.Check != nil {
		t.Errorf("checks: %+v %+v %+v", a.Check, b.Check, g.Check)
	}

	// summary
	s := rep.Summary
	if s.Definitions != 3 || s.Records != 3 || s.CheckReports != 2 || s.ZeroNewProducts != 1 || s.LastNewOrder != 2 ||
		s.DistinctIntroduced != a.New+b.New || s.DistinctUsed != s.DistinctIntroduced || s.GoProductSpecific != 0 {
		t.Errorf("summary: %+v", s)
	}
}

func TestBuildWaves(t *testing.T) {
	rep := Build(fixtureInputs(t))
	if len(rep.Waves) != 2 {
		t.Fatalf("waves: %+v", rep.Waves)
	}
	w0, w1 := rep.Waves[0], rep.Waves[1]
	if *w0.Wave != 0 || !reflect.DeepEqual(w0.Products, []string{"alpha"}) || w0.ZeroNew != 0 {
		t.Errorf("wave 0: %+v", w0)
	}
	if *w1.Wave != 1 || !reflect.DeepEqual(w1.Products, []string{"beta", "gamma"}) {
		t.Fatalf("wave 1: %+v", w1)
	}
	if w1.ZeroNew != 1 || !reflect.DeepEqual(w1.ZeroNewProducts, []string{"gamma"}) {
		t.Errorf("zero-new: %+v", w1)
	}
	if w1.NewConstructs != 20 || w1.NewPerProduct != 10 {
		t.Errorf("new constructs: %d (%.1f per product)", w1.NewConstructs, w1.NewPerProduct)
	}
	// median of 60 and 20
	if w1.MedianMinutes == nil || *w1.MedianMinutes != 40 || w1.MinutesKnown != 2 {
		t.Errorf("median minutes: %v", w1.MedianMinutes)
	}
	if w1.GoGeneric != 2 || w1.GoProductSpecific != 0 {
		t.Errorf("go changes: %+v", w1)
	}
	if w1.Discovery.Discovered != 6 || w1.Discovery.Manual != 1 {
		t.Errorf("discovery: %+v", w1.Discovery)
	}
	if w1.Reports != 1 || w1.Validated != 1 || w1.Representability["full"] != 2 {
		t.Errorf("reports / representability: %+v", w1)
	}
	if w0.UnreachableSources != 1 || w1.UnreachableSources != 0 {
		t.Errorf("unreachable: %d %d", w0.UnreachableSources, w1.UnreachableSources)
	}
	if w0.MedianMinutes == nil || *w0.MedianMinutes != 120 {
		t.Errorf("wave 0 median: %v", w0.MedianMinutes)
	}
}

func TestMedian(t *testing.T) {
	for _, tc := range []struct {
		in   []float64
		want float64
	}{{[]float64{5}, 5}, {[]float64{3, 1, 2}, 2}, {[]float64{4, 1, 3, 2}, 2.5}} {
		if got := median(tc.in); got != tc.want {
			t.Errorf("median(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func rec(product string, order int, wave *int, newNames ...string) *Record {
	r := &Record{Product: product, Order: order, Wave: wave}
	for _, n := range newNames {
		r.Constructs.New = append(r.Constructs.New, ConstructRef{Name: n})
	}
	return r
}

func ps(id string, constructs ...string) ProductStats {
	return ProductStats{Product: id, Constructs: constructs, SourceIDs: []string{"s"}, ArtifactIDs: []string{"a"}}
}

func joinWarnings(rep *Report) string { return strings.Join(rep.Warnings, "\n") }

func TestBuildAnomalies(t *testing.T) {
	w0 := 0
	in := &Inputs{
		Products: []ProductStats{
			ps("p1", "x:a", "x:b", "x:late"),
			ps("p2", "x:b", "x:c", "x:ghost"),
			ps("p3", "x:a", "x:c"),
			ps("norecord", "x:a", "x:z"),
		},
		Records: []*Record{
			rec("p2", 2, &w0, "x:c", "x:b"),           // x:b was introduced by p1 already: duplicate claim
			rec("p1", 1, &w0, "x:a", "x:b", "x:gone"), // x:gone is not used by the definition
			rec("p3", 3, nil),
			// no definition; x:late is introduced here, after p1 already uses it
			rec("onlyrecord", 4, &w0, "x:q", "x:late"),
		},
	}
	rep := Build(in)

	var order []string
	for _, r := range rep.Products {
		order = append(order, r.Product)
	}
	if !reflect.DeepEqual(order, []string{"p1", "p2", "p3", "onlyrecord", "norecord"}) {
		t.Fatalf("order (recorded by order, then the rest): %v", order)
	}
	p1, p2 := rowFor(t, rep, "p1"), rowFor(t, rep, "p2")
	if !reflect.DeepEqual(p1.NewList, []string{"x:a", "x:b", "x:gone"}) || p1.New != 3 {
		t.Errorf("p1 new: %v", p1.NewList)
	}
	if !reflect.DeepEqual(p2.NewList, []string{"x:c"}) || p2.Reused != 1 || p2.Unaccounted != 1 {
		t.Errorf("p2: new %v reused %d unaccounted %d", p2.NewList, p2.Reused, p2.Unaccounted)
	}
	if p1.RetroAdopted != 1 { // x:late is introduced by "onlyrecord" (order 4)
		t.Errorf("p1 retro: %d", p1.RetroAdopted)
	}
	if !reflect.DeepEqual(p2.UnaccountedList, []string{"x:ghost"}) {
		t.Errorf("p2 unaccounted list: %v", p2.UnaccountedList)
	}
	nr := rowFor(t, rep, "norecord")
	if !reflect.DeepEqual(nr.UnaccountedList, []string{"x:z"}) {
		t.Errorf("the candidates for a new record: %v", nr.UnaccountedList)
	}
	if nr.HasRecord || nr.Reused != 1 || nr.Unaccounted != 1 || nr.New != 0 || nr.ZeroNew {
		t.Errorf("a product without a record is neither new nor zero-new: %+v", nr)
	}
	or := rowFor(t, rep, "onlyrecord")
	if or.HasDefinition || or.New != 2 || or.Used != 0 {
		t.Errorf("record without a definition: %+v", or)
	}
	if got := rep.Curve[len(rep.Curve)-1].Cumulative; got != 6 { // a b gone c late q
		t.Errorf("cumulative = %d", got)
	}

	w := joinWarnings(rep)
	for _, want := range []string{
		`p2: construct "x:b" is listed as new, but "p1" introduced it first`,
		`p1: listed as a new construct but the definition does not use "x:gone"`,
		`p1 uses constructs introduced by a later product`,
		`p2 uses 1 construct(s) that no record introduces: x:ghost`,
	} {
		if !strings.Contains(w, want) {
			t.Errorf("missing warning %q in:\n%s", want, w)
		}
	}
}

func TestBuildWithoutRecords(t *testing.T) {
	in := &Inputs{Products: []ProductStats{ps("b", "x:1", "x:2"), ps("a", "x:1")}}
	rep := Build(in)
	if len(rep.Products) != 2 || rep.Products[0].Product != "a" || len(rep.Waves) != 0 {
		t.Fatalf("%+v", rep)
	}
	if rep.Curve[1].CumulativeUsed != 2 || rep.Curve[1].FirstUse != 1 || rep.Summary.Records != 0 || rep.Summary.DistinctUsed != 2 {
		t.Errorf("computed curve without records: %+v %+v", rep.Curve, rep.Summary)
	}
	if rep := Build(&Inputs{}); len(rep.Products) != 0 || len(rep.Waves) != 0 {
		t.Errorf("empty inputs: %+v", rep)
	}
}

func TestBuildDuplicateRecordsAndOrders(t *testing.T) {
	in := &Inputs{
		Products: []ProductStats{ps("a", "x:1"), ps("b", "x:2")},
		Records:  []*Record{rec("a", 1, nil, "x:1"), rec("b", 1, nil, "x:2"), rec("a", 5, nil, "x:9")},
		Checks:   map[string]*CheckSummary{"nobody": {Product: "nobody"}},
	}
	in.Records[2].File = "second.yaml"
	rep := Build(in)
	w := joinWarnings(rep)
	for _, want := range []string{`second record for "a"`, `both have order 1`, `check report for "nobody"`} {
		if !strings.Contains(w, want) {
			t.Errorf("missing warning %q in:\n%s", want, w)
		}
	}
	if len(rep.Waves) != 1 || rep.Waves[0].Wave != nil {
		t.Errorf("records without a wave are grouped as unassigned: %+v", rep.Waves)
	}
}

func TestProductSpecificGoIsFlagged(t *testing.T) {
	r := rec("a", 1, nil, "x:1")
	r.GoChanges.ProductSpecific = 2
	rep := Build(&Inputs{Products: []ProductStats{ps("a", "x:1")}, Records: []*Record{r}})
	if rep.Summary.GoProductSpecific != 2 || !strings.Contains(joinWarnings(rep), "2 product-specific Go change(s)") {
		t.Errorf("%+v / %s", rep.Summary, joinWarnings(rep))
	}
}

func TestDiscoveryShareCrossChecksTheDefinition(t *testing.T) {
	r := rec("a", 1, nil)
	r.Sources = []OriginEntry{{ID: "s", Origin: "discovered"}, {ID: "stale", Origin: "manual"}}
	// artifact "a" is not classified; an unknown origin counts as other
	in := &Inputs{
		Products: []ProductStats{{Product: "a", SourceIDs: []string{"s", "t"}, ArtifactIDs: []string{"x", "y"}}},
		Records:  []*Record{r},
	}
	r.Artifacts = []OriginEntry{{ID: "x", Origin: "Discovered-Modified"}, {ID: "y", Origin: "guessed"}}
	rep := Build(in)
	d := rep.Products[0].Discovery
	if d.Discovered != 1 || d.DiscoveredModified != 1 || d.Other != 1 || d.Manual != 0 || d.Unclassified != 1 {
		t.Errorf("discovery: %+v", d)
	}
	if !strings.Contains(joinWarnings(rep), `lists source "stale" which is not in the definition`) {
		t.Errorf("warnings: %s", joinWarnings(rep))
	}
	if (DiscoveryShare{}).Percent() != -1 {
		t.Error("nothing classified has no share")
	}
}

func TestFilter(t *testing.T) {
	rep := Build(fixtureInputs(t))
	f := rep.Filter([]string{"gamma", "alpha", "nope"})
	if len(f.Products) != 2 || f.Products[0].Product != "alpha" || f.Products[1].Product != "gamma" || len(f.Curve) != 2 {
		t.Fatalf("filtered: %+v", f.Products)
	}
	// cumulative numbers still come from the full ordering
	if f.Products[1].Cumulative != rep.Products[2].Cumulative {
		t.Errorf("cumulative changed by the filter")
	}
	if len(f.Waves) != 2 || !reflect.DeepEqual(f.Waves[1].Products, []string{"gamma"}) {
		t.Errorf("waves cover the selection only: %+v", f.Waves)
	}
	if !strings.Contains(joinWarnings(f), `unknown product "nope"`) {
		t.Errorf("warnings: %v", f.Warnings)
	}
	if rep.Filter(nil) != rep {
		t.Error("no ids: unchanged")
	}
}
