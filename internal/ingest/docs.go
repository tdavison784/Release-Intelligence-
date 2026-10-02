package ingest

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// groupResult is the output of one source group (a standalone source or the
// members of one fallback group).
type groupResult struct {
	statuses      []domain.SourceStatus
	notes         []domain.NoteItem
	compat        []domain.CompatibilityConstraint
	facts         []domain.Fact
	evidence      []domain.Evidence
	checks        []RelationshipCheck
	checkEvidence []domain.Evidence
}

// sourceOutcome is the output of consulting one source.
type sourceOutcome struct {
	status    domain.SourceStatus
	noAdapter bool
	notes     []domain.NoteItem
	compat    []domain.CompatibilityConstraint
	facts     []domain.Fact
	evidence  []domain.Evidence
	// docEvidence points at the retrieved document(s) / selected part.
	docEvidence []domain.Evidence
}

// runGroup consults the members of a group in order. In a fallback group the
// first member whose state is ok satisfies the group; later members are
// recorded as skipped (or, in exhaustive mode, consulted for their status
// only).
func (i *Ingester) runGroup(ctx context.Context, r *run, g sourceGroup) groupResult {
	var out groupResult
	winner := ""
	for _, src := range g.members {
		if winner != "" && !r.exhaustive {
			st := domain.SourceStatus{
				SourceID: src.ID, Kind: src.Locator.Kind, Roles: src.Roles, Version: r.v.Semver,
				State:  domain.SourceSkipped,
				Detail: fmt.Sprintf("fallback group %s satisfied by %s", g.name, winner),
			}
			out.statuses = append(out.statuses, st)
			out.checks = append(out.checks, sourceCheck(r, src, st, false, nil))
			continue
		}
		o := i.runSource(ctx, r, src)
		out.statuses = append(out.statuses, o.status)
		out.checks = append(out.checks, sourceCheck(r, src, o.status, o.noAdapter, o.docEvidence))
		if winner == "" {
			out.notes = append(out.notes, o.notes...)
			out.compat = append(out.compat, o.compat...)
			out.facts = append(out.facts, o.facts...)
			out.evidence = append(out.evidence, o.evidence...)
		} else {
			out.checkEvidence = append(out.checkEvidence, o.docEvidence...)
		}
		if g.name != "" && winner == "" && o.status.State == domain.SourceOK {
			winner = src.ID
		}
	}
	// In a satisfied fallback group, members that did not answer are covered
	// by the winner: their absence is not a failure of the definition.
	if winner != "" {
		for k := range out.checks {
			c := &out.checks[k]
			if c.Subject != winner && (c.Outcome == OutcomeFail || c.Outcome == OutcomeUnverifiable) {
				c.Outcome = OutcomeCovered
				c.Detail = fmt.Sprintf("covered by %s (fallback group %s): %s", winner, g.name, c.Detail)
			}
		}
	}
	return out
}

func sourceCheck(r *run, src catalog.Source, st domain.SourceStatus, noAdapter bool, evs []domain.Evidence) RelationshipCheck {
	return RelationshipCheck{
		Subject:     src.ID,
		SubjectKind: SubjectSource,
		Channel:     st.Kind,
		Release:     r.v.Semver,
		Outcome:     outcomeForState(st.State, noAdapter),
		Coordinate:  st.URI,
		Detail:      st.Detail,
		Evidence:    evidenceIDs(evs),
	}
}

func isNotesRole(src catalog.Source) bool {
	return src.HasRole(domain.RoleReleaseNotes) || src.HasRole(domain.RoleChangelog) || src.HasRole(domain.RoleUpgradeGuide)
}

func isTableExtract(t string) bool {
	return t == catalog.ExtractMarkdownTable || t == catalog.ExtractYAMLRecords
}

// runSource consults one source for the release: availability, locator
// rendering, retrieval, extraction and normalisation. It never fails; every
// problem becomes the source's status.
func (i *Ingester) runSource(ctx context.Context, r *run, src catalog.Source) sourceOutcome {
	role, _ := primaryRole(src)
	o := sourceOutcome{status: domain.SourceStatus{SourceID: src.ID, Kind: src.Locator.Kind, Roles: src.Roles, Version: r.v.Semver}}
	set := func(state domain.SourceState, detail string) sourceOutcome {
		o.status.State, o.status.Detail = state, detail
		return o
	}
	ok, err := catalog.AppliesTo(r.v, src.Availability, src.ReleaseKinds)
	if err != nil {
		return set(domain.SourceError, fmt.Sprintf("availability %q: %v", src.Availability, err))
	}
	if !ok {
		return set(domain.SourceSkipped, notApplicableDetail(r.v, src.Availability, src.ReleaseKinds))
	}
	if reason, ok := catalog.ExceptionFor(r.v, src.Exceptions); ok {
		return set(domain.SourceSkipped, "known exception: "+reason)
	}
	// Sources may follow a pinned artifact's version
	// ({{.ArtifactVersionOf "id"}}); a follow that did not resolve is the
	// source's status, never a half-rendered locator.
	rc, followState, followDetail, ok := r.sourceContext(src)
	if !ok {
		return set(followState, followDetail)
	}
	loc, err := renderLocator(src.Locator, rc)
	if err != nil {
		state, detail, _ := stateFor(err)
		return set(state, detail)
	}
	o.status.URI = locatorURI(loc)
	f := i.fetch(ctx, r.memo, loc, r.now)
	if f.err != nil {
		state, detail, noAdapter := stateFor(f.err)
		o.noAdapter = noAdapter
		return set(state, detail)
	}
	if len(f.docs) == 0 {
		return set(domain.SourceNotFound, "no files matching "+describeLocator(loc))
	}
	where := describeLocator(loc)
	if len(f.docs) == 1 {
		o.status.URI = f.docs[0].URI
		where = f.docs[0].URI
	}

	ex := catalog.Extract{Type: catalog.ExtractWhole}
	if src.Extract != nil {
		ex = *src.Extract
		if ex.Type == "" {
			ex.Type = catalog.ExtractWhole
		}
	}
	// extract.listItems selects the structural reading for markdown too;
	// docbook/rst/adoc conversions always use it
	structuralLists := ex.ListItems
	if ex.Format == catalog.FormatDocBook || ex.Format == catalog.FormatRST || ex.Format == catalog.FormatAsciiDoc || ex.Format == catalog.FormatHTML {
		// render DocBook / reStructuredText / AsciiDoc as markdown (line for
		// line) on a copy: documents are shared with other sources through
		// the memo
		conv := make([]sources.Document, len(f.docs))
		for k, d := range f.docs {
			switch ex.Format {
			case catalog.FormatDocBook:
				d.Content = normalize.DocBookToMarkdown(d.Content)
			case catalog.FormatRST:
				d.Content = normalize.RSTToMarkdown(d.Content)
			case catalog.FormatHTML:
				d.Content = normalize.HTMLToMarkdown(d.Content)
			default:
				d.Content = normalize.ASCIIDocToMarkdown(d.Content)
			}
			conv[k] = d
		}
		f.docs = conv
		// converted structural markup: every list item is one item whatever
		// prose precedes it (an intro paragraph is an item of its own)
		structuralLists = true
	}
	repo := repositoryOf(loc)
	if repo == "" {
		repo = r.repo
	}
	input := func(d sources.Document) normalize.DocInput {
		return normalize.DocInput{
			SourceID:    src.ID,
			Role:        role,
			Release:     r.v.Semver,
			URI:         d.URI,
			Digest:      d.Digest,
			RetrievedAt: d.RetrievedAt,
			Content:     d.Content,
			Repository:  repo,

			LabelPattern: ex.LabelParagraphs,
			ListItems:    structuralLists,
		}
	}
	p := i.parser()
	var (
		inputs  []normalize.DocInput
		docEv   []domain.Evidence
		partEv  []domain.Evidence // evidence returned by table extraction
		extract string            // human description of what was extracted
	)
	switch ex.Type {
	case catalog.ExtractWhole, catalog.ExtractReleaseNoteYAML:
		for _, d := range f.docs {
			inputs = append(inputs, input(d))
			docEv = append(docEv, wholeEvidence(src.ID, d))
		}
		if len(f.docs) == 1 {
			extract = fmt.Sprintf("document (%d lines)", lineCount(f.docs[0].Content))
		} else {
			extract = fmt.Sprintf("%d files", len(f.docs))
		}
	case catalog.ExtractMarkdownSection:
		pattern, re, err := renderRegex(ex.Heading, rc)
		if err != nil {
			return set(domain.SourceError, "extract heading: "+err.Error())
		}
		for _, d := range f.docs {
			sec, err := p.SelectSection(d.Content, re)
			if errors.Is(err, normalize.ErrNoMatch) || (err == nil && sec == nil) {
				continue
			}
			if err != nil {
				return set(domain.SourceError, fmt.Sprintf("select section in %s: %v", d.URI, err))
			}
			in := input(d)
			in.Content = []byte(sec.Body)
			in.LineOffset = sec.StartLine - 1
			inputs = append(inputs, in)
			docEv = append(docEv, domain.NewEvidence(domain.EvidenceDocument, src.ID, d.URI,
				fmt.Sprintf("L%d-L%d", sec.StartLine, sec.EndLine), sec.Heading, d.Digest, d.RetrievedAt))
			extract = fmt.Sprintf("section %q (L%d-L%d)", sec.Heading, sec.StartLine, sec.EndLine)
			break
		}
		if len(inputs) == 0 {
			return set(domain.SourceNotFound, fmt.Sprintf("no section matching %q in %s", pattern, where))
		}
	case catalog.ExtractMarkdownTable, catalog.ExtractYAMLRecords:
		sel, keyPattern, err := tableSelector(ex, rc)
		if err != nil {
			return set(domain.SourceError, "extract: "+err.Error())
		}
		matched := false
		for _, d := range f.docs {
			var row *normalize.TableRow
			if ex.Type == catalog.ExtractMarkdownTable {
				row, err = p.ExtractTableRow(d.Content, sel)
			} else {
				row, err = p.ExtractRecord(d.Content, sel)
			}
			if errors.Is(err, normalize.ErrNoMatch) || (err == nil && row == nil) {
				continue
			}
			if err != nil {
				return set(domain.SourceError, fmt.Sprintf("extract %s from %s: %v", ex.Type, d.URI, err))
			}
			in := input(d)
			cs, evs := p.CompatibilityFromRow(in, row, ex.Columns)
			for k := range cs {
				if cs[k].SourceID == "" {
					cs[k].SourceID = src.ID
				}
			}
			o.compat = cs
			partEv = evs
			if len(evs) > 0 {
				docEv = append(docEv, evs[0])
			} else {
				docEv = append(docEv, domain.NewEvidence(domain.EvidenceDocument, src.ID, d.URI,
					fmt.Sprintf("L%d", row.Line), row.Excerpt, d.Digest, d.RetrievedAt))
			}
			extract = fmt.Sprintf("row L%d, %d constraints", row.Line, len(cs))
			matched = true
			break
		}
		if !matched {
			return set(domain.SourceNotFound, fmt.Sprintf("no row whose %s matches %q in %s",
				strings.Join(ex.KeyColumns, "/"), keyPattern, where))
		}
	default:
		return set(domain.SourceError, fmt.Sprintf("unknown extract type %q", ex.Type))
	}

	var (
		parseErr error
		noteEv   []domain.Evidence
		parsed   bool
	)
	switch {
	case ex.Type == catalog.ExtractReleaseNoteYAML:
		parsed = true
		// The parser skips files that do not parse and returns the items of
		// the others together with a joined error: keep what it produced.
		o.notes, noteEv, parseErr = p.ParseReleaseNoteYAML(inputs, src.Classify)
	case isNotesRole(src) && !isTableExtract(ex.Type):
		parsed = true
		for _, in := range inputs {
			items, evs, err := p.ParseNotes(in, src.Classify)
			if err != nil {
				parseErr = err
				o.notes, noteEv = nil, nil
				break
			}
			o.notes = append(o.notes, items...)
			noteEv = append(noteEv, evs...)
		}
	}

	o.docEvidence = docEv
	o.evidence = append(append(append(o.evidence, docEv...), partEv...), noteEv...)
	factAttrs := []string{"source", src.ID, "role", string(role), "kind", loc.Kind, "extract", ex.Type}
	if len(f.docs) == 1 {
		factAttrs = append(factAttrs, "uri", f.docs[0].URI, "digest", f.docs[0].Digest)
	} else {
		factAttrs = append(factAttrs, "uri", locatorURI(loc), "files", strconv.Itoa(len(f.docs)))
	}
	o.facts = append(o.facts, domain.NewFact(domain.FactDocumentRetrieved, r.subject, r.v.Semver,
		fmt.Sprintf("%s for %s %s retrieved from %s (%s)", role, r.def.Name, r.v.Semver, src.ID, where),
		Producer, attrs(factAttrs...), evidenceIDs(docEv)...))
	o.facts = append(o.facts, compatFacts(r.subject, r.v.Semver, o.compat)...)

	if parseErr != nil && len(o.notes) == 0 {
		return set(domain.SourcePartial, fmt.Sprintf("%s retrieved, but parsing failed: %v", extract, parseErr))
	}
	detail := extract
	if parsed {
		detail += fmt.Sprintf("; %d note items", len(o.notes))
	}
	if parseErr != nil {
		// Some files parsed, others did not: the items are kept and the
		// source counts as ok (it satisfies its fallback group); the files
		// that failed are named in the detail.
		failed := splitErrors(parseErr)
		noun := "files"
		if len(failed) == 1 {
			noun = "file"
		}
		detail += fmt.Sprintf("; %d %s failed to parse: %s", len(failed), noun, strings.Join(failed, "; "))
	}
	return set(domain.SourceOK, detail)
}

// splitErrors returns the messages of the errors joined in err (one for a
// plain error), each collapsed to a single line.
func splitErrors(err error) []string {
	var errs []error
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		errs = j.Unwrap()
	} else {
		errs = []error{err}
	}
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, strings.Join(strings.Fields(e.Error()), " "))
	}
	return out
}

func tableSelector(ex catalog.Extract, rc catalog.RenderContext) (normalize.TableSelector, string, error) {
	pattern, re, err := renderRegex(ex.KeyMatch, rc)
	if err != nil {
		return normalize.TableSelector{}, pattern, fmt.Errorf("keyMatch: %w", err)
	}
	sel := normalize.TableSelector{KeyColumns: ex.KeyColumns, KeyRe: re, Collect: ex.Collect}
	for f, w := range ex.Where {
		_, wre, err := renderRegex(w, rc)
		if err != nil {
			return normalize.TableSelector{}, pattern, fmt.Errorf("where.%s: %w", f, err)
		}
		if sel.Where == nil {
			sel.Where = map[string]*regexp.Regexp{}
		}
		sel.Where[f] = wre
	}
	for _, c := range ex.Columns {
		sel.ValueColumns = append(sel.ValueColumns, c.Headers...)
	}
	if ex.TableHeading != "" {
		_, th, err := renderRegex(ex.TableHeading, rc)
		if err != nil {
			return normalize.TableSelector{}, pattern, fmt.Errorf("tableHeading: %w", err)
		}
		sel.TableHeading = th
	}
	return sel, pattern, nil
}

// wholeEvidence points at a complete retrieved document.
func wholeEvidence(sourceID string, d sources.Document) domain.Evidence {
	locator := ""
	if n := lineCount(d.Content); n > 0 {
		locator = fmt.Sprintf("L1-L%d", n)
	}
	return domain.NewEvidence(domain.EvidenceDocument, sourceID, d.URI, locator, firstLine(d.Content), d.Digest, d.RetrievedAt)
}
