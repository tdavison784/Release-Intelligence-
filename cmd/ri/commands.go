package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

func (c *cli) products(args []string) error {
	fs := c.flags("products", "")
	output := fs.String("o", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cat, err := catalog.LoadDir(c.g.products)
	if err != nil {
		return err
	}
	if *output == "json" {
		return c.writeJSON(cat.List())
	}
	tw := tabwriter.NewWriter(c.out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSOURCES\tARTIFACTS\tVALID")
	for _, d := range cat.List() {
		rep := catalog.Validate(d)
		valid := "yes"
		if !rep.OK() {
			valid = fmt.Sprintf("no (%d errors)", len(rep.Errors()))
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n", d.ID, d.Name, len(d.Sources), len(d.Artifacts), valid)
	}
	return tw.Flush()
}

func (c *cli) validate(args []string) error {
	fs := c.flags("validate", "[product|file.yaml ...]")
	output := fs.String("o", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var defs []*catalog.ProductDefinition
	targets := fs.Args()
	if len(targets) == 0 {
		cat, err := catalog.LoadDir(c.g.products)
		if err != nil {
			return err
		}
		defs = cat.List()
	}
	for _, t := range targets {
		if strings.HasSuffix(t, ".yaml") || strings.HasSuffix(t, ".yml") {
			d, err := catalog.LoadFile(t)
			if err != nil {
				return err
			}
			defs = append(defs, d)
			continue
		}
		cat, err := catalog.LoadDir(c.g.products)
		if err != nil {
			return err
		}
		d, ok := cat.Get(t)
		if !ok {
			return fmt.Errorf("unknown product %q", t)
		}
		defs = append(defs, d)
	}
	var reports []catalog.ValidationReport
	failed := 0
	for _, d := range defs {
		rep := catalog.Validate(d)
		reports = append(reports, rep)
		if !rep.OK() {
			failed++
		}
	}
	if *output == "json" {
		if err := c.writeJSON(reports); err != nil {
			return err
		}
	} else {
		for i, rep := range reports {
			status := "✓ valid"
			if !rep.OK() {
				status = "✗ invalid"
			}
			fmt.Fprintf(c.out, "%s  %s (%s)\n", status, rep.Product, defs[i].Path())
			for _, is := range rep.Issues {
				fmt.Fprintf(c.out, "    %s %s: %s\n", is.Severity, is.Path, is.Message)
			}
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d definition(s) invalid", failed)
	}
	return nil
}

func (c *cli) versions(args []string) error {
	fs := c.flags("versions", "<product>")
	output := fs.String("o", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product>", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	def, err := a.Product(fs.Arg(0))
	if err != nil {
		return err
	}
	vl, err := a.Versions(c.ctx, def)
	if err != nil {
		return err
	}
	if *output == "json" {
		return c.writeJSON(vl)
	}
	printStatuses(c.out, vl.Sources)
	byLine := map[string][]string{}
	var lines []string
	for _, v := range vl.Versions {
		if _, ok := byLine[v.Line()]; !ok {
			lines = append(lines, v.Line())
		}
		byLine[v.Line()] = append(byLine[v.Line()], v.Tag)
	}
	fmt.Fprintf(c.out, "\n%s: %d releases in %d lines\n", def.ID, len(vl.Versions), len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		fmt.Fprintf(c.out, "  %-6s %s\n", lines[i], strings.Join(byLine[lines[i]], " "))
	}
	return nil
}

func (c *cli) check(args []string) error {
	fs := c.flags("check", "<product>")
	n := fs.Int("n", 3, "number of most recent release lines to check (latest patch of each)")
	versions := fs.String("versions", "", "comma-separated releases to check instead of -n")
	output := fs.String("o", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product>", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	rep, err := a.CheckRelationships(c.ctx, fs.Arg(0), splitList(*versions), *n)
	if err != nil {
		return err
	}
	if *output == "json" {
		return c.writeJSON(rep)
	}
	printRelationshipReport(c.out, rep)
	for _, s := range rep.Summary {
		if s.Verdict == "failing" {
			return fmt.Errorf("%d relationship(s) failing", countVerdict(rep, "failing"))
		}
	}
	return nil
}

func countVerdict(rep *ingest.RelationshipReport, verdict string) int {
	n := 0
	for _, s := range rep.Summary {
		if s.Verdict == verdict {
			n++
		}
	}
	return n
}

func (c *cli) ingest(args []string) error {
	fs := c.flags("ingest", "<product> <version>")
	output := fs.String("o", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product> <version>", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	def, err := a.Product(fs.Arg(0))
	if err != nil {
		return err
	}
	vl, err := a.Versions(c.ctx, def)
	if err != nil {
		return err
	}
	v, err := app.ResolveVersion(vl, fs.Arg(1))
	if err != nil {
		return err
	}
	r, err := a.Release(c.ctx, def, v, vl)
	if err != nil {
		return err
	}
	if *output == "json" {
		return c.writeJSON(r)
	}
	printRelease(c.out, def, r)
	return nil
}

func (c *cli) upgrade(args []string) error {
	fs := c.flags("upgrade", "<product> <from> <to>")
	output := fs.String("o", "text", "output format: text|json")
	verbose := fs.Bool("verbose", false, "include features, bug fixes and evidence excerpts")
	policy := fs.String("policy", "", "path policy override: minor-lineage|all")
	max := fs.Int("max", 25, "maximum items per section in text output (-1 = unlimited)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 3 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product> <from> <to>", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	edge, err := a.Upgrade(c.ctx, fs.Arg(0), fs.Arg(1), fs.Arg(2), app.UpgradeOptions{Policy: *policy})
	if err != nil {
		return err
	}
	if *output == "json" {
		return c.writeJSON(edge)
	}
	return upgrade.RenderText(c.out, edge, upgrade.RenderOptions{
		Verbose:       *verbose,
		MaxPerSection: *max,
		Color:         isTerminal(c.out),
	})
}

func printStatuses(w interface{ Write([]byte) (int, error) }, ss []domain.SourceStatus) {
	for _, s := range ss {
		mark := "✓"
		switch s.State {
		case domain.SourceOK:
		case domain.SourceSkipped:
			mark = "·"
		case domain.SourcePartial:
			mark = "~"
		default:
			mark = "✗"
		}
		line := fmt.Sprintf("%s %-26s %-16s %s", mark, s.SourceID, s.Kind, s.State)
		if s.Detail != "" {
			line += " — " + s.Detail
		}
		fmt.Fprintln(w, line)
	}
}

func printRelationshipReport(w interface{ Write([]byte) (int, error) }, rep *ingest.RelationshipReport) {
	fmt.Fprintf(w, "%s: relationship validation against %s\n\n", rep.Product, strings.Join(rep.Releases, ", "))
	cells := map[string]map[string]ingest.RelationshipCheck{}
	for _, ch := range rep.Checks {
		if cells[ch.Subject] == nil {
			cells[ch.Subject] = map[string]ingest.RelationshipCheck{}
		}
		cells[ch.Subject][ch.Release] = ch
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprint(tw, "SUBJECT\tKIND")
	for _, r := range rep.Releases {
		fmt.Fprintf(tw, "\t%s", r)
	}
	fmt.Fprintln(tw, "\tVERDICT")
	sum := append([]ingest.RelationshipSummary(nil), rep.Summary...)
	sort.SliceStable(sum, func(i, j int) bool {
		if sum[i].SubjectKind != sum[j].SubjectKind {
			return sum[i].SubjectKind > sum[j].SubjectKind
		}
		return false
	})
	glyph := map[string]string{
		ingest.OutcomePass: "✓ pass", ingest.OutcomeFail: "✗ FAIL",
		ingest.OutcomeUnverifiable: "? unverif.", ingest.OutcomeNotApplicable: "· n/a",
	}
	for _, s := range sum {
		fmt.Fprintf(tw, "%s\t%s", s.Subject, s.SubjectKind)
		for _, r := range rep.Releases {
			ch, ok := cells[s.Subject][r]
			if !ok {
				fmt.Fprint(tw, "\t")
				continue
			}
			fmt.Fprintf(tw, "\t%s", glyph[ch.Outcome])
		}
		fmt.Fprintf(tw, "\t%s (%d/%d/%d)\n", s.Verdict, s.Passed, s.Failed, s.Unverifiable)
	}
	tw.Flush()
	fmt.Fprintln(w, "\nverdict counts are pass/fail/unverifiable; 'validated' requires ≥3 passes and no failures.")
	var notes []string
	for _, ch := range rep.Checks {
		if (ch.Outcome == ingest.OutcomeFail || ch.Outcome == ingest.OutcomeUnverifiable) && ch.Detail != "" {
			notes = append(notes, fmt.Sprintf("  %s @ %s: %s — %s", ch.Subject, ch.Release, ch.Outcome, ch.Detail))
		}
	}
	if len(notes) > 0 {
		fmt.Fprintln(w, "\nDetails:")
		for _, n := range notes {
			fmt.Fprintln(w, n)
		}
	}
}

func printRelease(w interface{ Write([]byte) (int, error) }, def *catalog.ProductDefinition, r *domain.Release) {
	fmt.Fprintf(w, "%s %s\n\nSources:\n", def.Name, r.Version.Tag)
	printStatuses(w, r.Sources)
	fmt.Fprintln(w, "\nArtifacts:")
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	for _, a := range r.Artifacts {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", a.ArtifactID, a.Status, a.Coordinate, a.Detail)
	}
	tw.Flush()
	counts := map[domain.Category]int{}
	breaking := 0
	for _, n := range r.Notes {
		counts[n.Category]++
		if n.Breaking {
			breaking++
		}
	}
	fmt.Fprintf(w, "\nRelease-note items: %d (breaking: %d)\n", len(r.Notes), breaking)
	for _, cat := range domain.AllCategories {
		if counts[cat] > 0 {
			fmt.Fprintf(w, "  %-14s %d\n", cat, counts[cat])
		}
	}
	if len(r.Compat) > 0 {
		fmt.Fprintln(w, "\nCompatibility:")
		for _, cc := range r.Compat {
			fmt.Fprintf(w, "  %s (%s): %s  [%s]\n", cc.Platform, cc.Kind, strings.TrimSpace(cc.Raw), cc.Constraint)
		}
	}
	if len(r.Snapshots) > 0 {
		fmt.Fprintln(w, "\nSnapshots:")
		for _, s := range r.Snapshots {
			detail := ""
			switch {
			case s.Values != nil:
				detail = fmt.Sprintf("%d values", len(s.Values.Entries))
			case s.CRDs != nil:
				detail = fmt.Sprintf("%d CRDs", len(s.CRDs.CRDs))
			case s.Images != nil:
				detail = fmt.Sprintf("%d images", len(s.Images.Images))
			}
			fmt.Fprintf(w, "  %s/%s: %s\n", s.ArtifactID, s.Kind, detail)
		}
	}
	fmt.Fprintf(w, "\nFacts: %d   Evidence records: %d\n", len(r.Facts), len(r.Evidence))
}

func isTerminal(w any) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == ""
}
