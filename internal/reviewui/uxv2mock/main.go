// Command uxv2mock renders the reviewux lane's step-1 static mockup from the
// review UI demo fixtures (reviewui.NewDemoQueue): three representative items
// as "decision cards" plus the redesigned inbox. Output is self-contained
// HTML (CSS inlined) under docs/phase3/learning-loop/review-ui/ux-v2/.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/reviewui"
)

//go:embed mock.html mock.css
var files embed.FS

// --- view types ---------------------------------------------------------------------------

type chip struct{ Label, Tone string }

type evView struct {
	Kind, Link, URI, Locator, Excerpt, Digest, CitedBy string
	Fixture                                            bool
}

type valView struct {
	Validator, Aspect, Outcome, Tone, Detail, Rule string
}

type rawView struct {
	Model, ModelRaw, Provider, Confidence, CallID, ModelVersion, PromptDigest, GeneratedAt, Class, Undetermined string
}

type differCell struct{ Who, Text string }
type differRowT struct {
	AspectLabel string
	Cells       []differCell
}

type opt struct {
	Value    string
	Selected bool
}

type field struct {
	Label   string
	Value   string
	Options []opt
	Mono    bool
	Area    bool
}

type correctBlock struct {
	Label, Preview string
	Inputs         []field
}

type cardView struct {
	Product, Release, StatusLabel, StatusTone, Priority string
	Question                                            string
	QuestionTypeLabel                                   string

	UpQuote, UpLink, UpKind, UpLocator string
	UpMore                             int
	Fixture                            bool

	Suggested    string
	ClassChip    chip
	WhyConfident []confLine
	PrimaryLabel string

	WhyYou       string
	WhyTone      string
	DifferRows   []differRowT
	Undetermined string
	AgreeLine    string

	CorrectFields []correctBlock

	Evidence      []evView
	EvidenceCount int
	Validations   []valView
	RawProposals  []rawView

	ID, CandidateID, Route, Signals, Env string

	Home string
}

type rowView struct {
	Product, Release, Priority, StatusLabel, StatusTone string
	Question, WhyYou, WhyTone, Suggested, Href          string
	ClassChip                                           chip
}

func main() {
	out := flag.String("out", "docs/phase3/learning-loop/review-ui/ux-v2", "output directory")
	flag.Parse()
	cssB, err := files.ReadFile("mock.css")
	must(err)
	tpl, err := template.New("").Funcs(template.FuncMap{
		"switch": func(key string, kv ...string) string {
			for i := 0; i+1 < len(kv); i += 2 {
				if kv[i] == key {
					return kv[i+1]
				}
			}
			return ""
		},
	}).ParseFS(files, "mock.html")
	must(err)

	q := reviewui.NewDemoQueue()
	ctx := context.Background()
	ids := map[string]string{
		"item-disagreement.html":     q.ItemIDByTitle("rotationPolicy"),
		"item-thin-evidence.html":    q.ItemIDByTitle("Ambient"),
		"item-consensus-action.html": q.ItemIDByTitle("RSA"),
	}
	href := map[string]string{}
	for file, id := range ids {
		href[id] = file
	}

	must(os.MkdirAll(*out, 0o755))

	// the inbox: every fixture item as a redesigned row
	inb, err := q.Inbox(ctx, knowledge.InboxFilter{})
	must(err)
	var rows []rowView
	for _, r := range inb.Items {
		rc, err := q.Item(ctx, r.Item.ID)
		must(err)
		h := "#"
		if f, ok := href[r.Item.ID]; ok {
			h = f
		}
		rows = append(rows, rowView{
			Product: string(r.Item.Product), Release: r.Item.Release, Priority: string(r.Item.Routing.Priority),
			StatusLabel: statusWord(r.Item.Status), StatusTone: statusTone(r.Item.Status),
			Question: plainQuestion(rc), WhyYou: whyYou(rc), WhyTone: whyTone(rc),
			Suggested: truncate(suggestedAnswer(rc), 130), Href: h, ClassChip: classChip(rc.Item.Proposed.Consequence),
		})
	}
	writePage(tpl, filepath.Join(*out, "inbox.html"), "Inbox", map[string]any{
		"CSS": string(cssB), "Home": "inbox.html", "Rows": rows,
	})

	// the three representative items
	for file, id := range ids {
		rc, err := q.Item(ctx, id)
		must(err)
		writePage(tpl, filepath.Join(*out, file), rc.Candidate.Title, map[string]any{
			"CSS": string(cssB), "Home": "inbox.html", "Card": buildCard(rc),
		})
	}
	fmt.Println("wrote 4 pages to", *out)
}

func writePage(tpl *template.Template, path, title string, data map[string]any) {
	var b strings.Builder
	must(tpl.ExecuteTemplate(&b, "top", struct {
		Title string
		CSS   string
		Home  string
	}{title, data["CSS"].(string), data["Home"].(string)}))
	switch {
	case data["Rows"] != nil:
		must(tpl.ExecuteTemplate(&b, "inbox", data))
	case data["Card"] != nil:
		must(tpl.ExecuteTemplate(&b, "card", data["Card"]))
	}
	must(tpl.ExecuteTemplate(&b, "bottom", nil))
	must(os.WriteFile(path, []byte(b.String()), 0o644))
}

// --- card assembly -------------------------------------------------------------------------

func buildCard(rc *knowledge.ReviewContext) cardView {
	it := rc.Item
	v := cardView{
		Product: string(it.Product), Release: it.Release,
		StatusLabel: statusWord(it.Status), StatusTone: statusTone(it.Status),
		Priority: string(it.Routing.Priority),
		Question: plainQuestion(rc), QuestionTypeLabel: questionWord(it.QuestionType),
		Suggested: suggestedAnswer(rc), PrimaryLabel: "Accept suggested answer",
		WhyYou: whyYou(rc), WhyTone: whyTone(rc),
		Home: "inbox.html",
	}
	if it.QuestionType == domain.QuestionEvidenceSufficiency {
		v.PrimaryLabel = "Send back — need more evidence"
	}
	if c := it.Proposed.Consequence; c != nil {
		v.ClassChip = classChip(c)
	}

	// §2: the best evidence quote = the record the models cited most
	best, bestN := -1, -1
	cited := map[domain.EvidenceID]int{}
	for _, p := range rc.Proposals {
		for _, id := range p.Citations {
			cited[id]++
		}
	}
	for i, e := range rc.Candidate.Evidence {
		if cited[e.ID] > bestN {
			best, bestN = i, cited[e.ID]
		}
	}
	if best < 0 && len(rc.Candidate.Evidence) > 0 {
		best = 0
	}
	if best >= 0 {
		e := rc.Candidate.Evidence[best]
		v.UpQuote = e.Excerpt
		if v.UpQuote == "" {
			v.UpQuote = rc.Candidate.Text
		}
		v.UpLink, v.UpKind, v.UpLocator = e.URI, string(e.SourceID)+" · "+string(e.Kind), e.Locator
		v.UpMore = len(rc.Candidate.Evidence) - 1
	}
	v.Fixture = true // demo fixtures, shown with the banner

	// §3: why confident
	v.WhyConfident = whyConfident(rc)

	// §5: differing aspects as plain rows; agreeing aspects collapsed
	v.DifferRows, v.AgreeLine = differ(rc)
	for _, ag := range rc.Agreement {
		if len(ag.Groups) == 0 && len(ag.Undetermined) > 0 {
			for _, p := range rc.Proposals {
				if p.UndeterminedReason != "" && slices.Contains(ag.Undetermined, p.ID) {
					v.Undetermined = p.UndeterminedReason
					break
				}
			}
		}
	}

	// §6: the correction form, pre-filled, with plain-English previews
	v.CorrectFields = correctBlocks(rc.Item.Proposed)

	// §7: details
	v.EvidenceCount = len(rc.Candidate.Evidence)
	modelOf := map[domain.EvidenceID]string{}
	for _, p := range rc.Proposals {
		for _, id := range p.Citations {
			if modelOf[id] == "" {
				modelOf[id] = modelShort(p.Provenance.Model)
			} else {
				modelOf[id] += ", " + modelShort(p.Provenance.Model)
			}
		}
	}
	for _, e := range rc.Candidate.Evidence {
		link := ""
		if strings.HasPrefix(e.URI, "http") {
			link = e.URI
		}
		v.Evidence = append(v.Evidence, evView{
			Kind: string(e.Kind), Link: link, URI: e.URI, Locator: e.Locator, Excerpt: e.Excerpt,
			Digest: short(e.ContentDigest), CitedBy: modelOf[e.ID], Fixture: true,
		})
	}
	for _, val := range rc.Validations {
		for _, c := range val.Checks {
			tone := "warn"
			switch c.Outcome {
			case domain.OutcomeConfirmed:
				tone = "ok"
			case domain.OutcomeRefuted:
				tone = "danger"
			}
			v.Validations = append(v.Validations, valView{
				Validator: val.Validator, Aspect: aspectWords[c.Aspect], Outcome: string(c.Outcome),
				Tone: tone, Detail: c.Detail, Rule: c.Rule,
			})
		}
	}
	props := append([]domain.SemanticProposal(nil), rc.Proposals...)
	sort.SliceStable(props, func(i, j int) bool { return props[i].Provenance.Model < props[j].Provenance.Model })
	for _, p := range props {
		und := strings.Join(aspectsWords(p.Undetermined), ", ")
		reason := p.UndeterminedReason
		if reason != "" {
			und += " — " + reason
		}
		v.RawProposals = append(v.RawProposals, rawView{
			Model: modelShort(p.Provenance.Model), ModelRaw: p.Provenance.Model, Provider: p.Provider,
			Confidence: string(p.Provenance.Confidence), CallID: p.Provenance.CallID,
			ModelVersion: p.Provenance.ModelVersion, PromptDigest: short(p.Provenance.PromptDigest),
			GeneratedAt: p.Provenance.GeneratedAt.UTC().Format("2006-01-02 15:04 UTC"),
			Class:       classLabel(p.SuggestedClass), Undetermined: und,
		})
	}
	var sigs []string
	for _, s := range it.Routing.Signals {
		sigs = append(sigs, string(s))
	}
	v.ID, v.CandidateID = it.ID, it.CandidateID
	v.Route = string(it.Routing.Route) + "/" + string(it.Routing.Priority)
	v.Signals = strings.Join(sigs, ", ")
	if rc.Environment != nil {
		v.Env = rc.Environment.Label
	}
	return v
}

// differ builds §5: one row per aspect with ≥2 answer groups, and the
// "all calls agree on" collapse line for the single-group aspects.
func differ(rc *knowledge.ReviewContext) ([]differRowT, string) {
	var rows []differRowT
	var agreed []string
	for _, ag := range rc.Agreement {
		w, ok := aspectWords[ag.Aspect]
		if !ok {
			continue
		}
		if len(ag.Groups) < 2 {
			if len(ag.Groups) == 1 && len(rc.Proposals) >= 2 {
				n := 0
				for _, ids := range ag.Groups {
					n = len(ids)
				}
				if n == len(rc.Proposals) {
					agreed = append(agreed, w)
				}
			}
			continue
		}
		digests := make([]string, 0, len(ag.Groups))
		for d := range ag.Groups {
			digests = append(digests, d)
		}
		// deterministic, reader-friendly order: biggest group first, then model name
		sort.Slice(digests, func(i, j int) bool {
			if len(ag.Groups[digests[i]]) != len(ag.Groups[digests[j]]) {
				return len(ag.Groups[digests[i]]) > len(ag.Groups[digests[j]])
			}
			return firstModel(rc, ag.Groups[digests[i]]) < firstModel(rc, ag.Groups[digests[j]])
		})
		row := differRowT{AspectLabel: w}
		for _, d := range digests {
			var models []string
			var first *domain.SemanticProposal
			for _, pid := range ag.Groups[d] {
				for i := range rc.Proposals {
					if rc.Proposals[i].ID == pid {
						models = append(models, modelShort(rc.Proposals[i].Provenance.Model))
						if first == nil {
							p := rc.Proposals[i]
							first = &p
						}
					}
				}
			}
			if first == nil {
				continue
			}
			row.Cells = append(row.Cells, differCell{Who: strings.Join(models, " + "), Text: aspectSentence(first.Assertion, ag.Aspect)})
		}
		if len(row.Cells) >= 2 {
			rows = append(rows, row)
		}
	}
	var line string
	if len(agreed) > 0 {
		n := len(rc.Proposals)
		who := fmt.Sprintf("All %d calls", n)
		if n == 2 {
			who = "Both calls"
		}
		line = who + " agree on: " + strings.Join(agreed, ", ") + "."
	}
	return rows, line
}

// correctBlocks is the per-aspect edit form with plain-English previews.
func correctBlocks(a domain.SemanticAssertion) []correctBlock {
	var out []correctBlock
	if s := a.Subject; s != nil {
		var fams []opt
		for _, f := range domain.SubjectFamilies {
			fams = append(fams, opt{Value: familyWord(f), Selected: f == s.Family})
		}
		out = append(out, correctBlock{Label: "What the thing is", Preview: subjectPhrase(s), Inputs: []field{
			{Label: "Kind of thing", Options: fams},
			{Label: "Product", Value: string(s.Product)},
			{Label: "API group", Value: s.Group, Mono: true},
			{Label: "Kind", Value: s.Kind, Mono: true},
			{Label: "Field path", Value: s.Path, Mono: true},
			{Label: "Name", Value: s.Name, Mono: true},
			{Label: "Component", Value: s.Component},
		}})
	}
	if ch := a.Change; ch != nil {
		var kinds []opt
		for _, k := range domain.ChangeKinds {
			kinds = append(kinds, opt{Value: changeWord(k), Selected: k == ch.Type})
		}
		before, after := "", ""
		if ch.Before != nil {
			before = *ch.Before
		}
		if ch.After != nil {
			after = *ch.After
		}
		repl := ""
		if ch.ReplacedBy != nil {
			repl = subjectPhrase(ch.ReplacedBy)
		}
		out = append(out, correctBlock{Label: "What changed", Preview: changeSentence(a), Inputs: []field{
			{Label: "Type of change", Options: kinds},
			{Label: "Before", Value: before, Mono: true},
			{Label: "After", Value: after, Mono: true},
			{Label: "Replaced by", Value: repl, Mono: true},
		}})
	}
	if ap := a.Applicability; ap != nil {
		ov := ""
		if ap.Overlap != nil {
			ov = prettyJSON(*ap.Overlap)
		}
		preview := "Exposed: " + exposurePhrase(ap)
		if ov != "" {
			preview += ". Safe when " + overlapPhrase(ap)
		}
		out = append(out, correctBlock{Label: "Who is exposed", Preview: preview, Inputs: []field{
			{Label: "Exposed when (condition tree, DESIGN §1.3)", Value: prettyJSON(ap.Exposure), Area: true},
			{Label: "Safe when (optional overlap)", Value: ov, Area: true},
		}})
	}
	if c := a.Consequence; c != nil {
		var kinds []opt
		for _, k := range domain.ConsequenceKinds {
			kinds = append(kinds, opt{Value: kindWord(k), Selected: k == c.Kind})
		}
		var sevs []opt
		for _, s := range []domain.ImpactSeverity{domain.SeverityCritical, domain.SeverityHigh, domain.SeverityMedium, domain.SeverityLow} {
			sevs = append(sevs, opt{Value: severityWord(s), Selected: s == c.Severity})
		}
		out = append(out, correctBlock{Label: "What happens", Preview: consequenceSentence(c), Inputs: []field{
			{Label: "Type of consequence", Options: kinds},
			{Label: "Severity", Options: sevs},
			{Label: "What exactly fails", Value: c.Statement},
			{Label: "How to fix it", Value: c.Remediation},
		}})
	}
	if a.Statement != "" {
		out = append(out, correctBlock{Label: "One-line summary", Preview: a.Statement, Inputs: []field{
			{Label: "Summary (prose, never compared)", Value: a.Statement},
		}})
	}
	return out
}

// --- small helpers ---------------------------------------------------------------------------

func questionWord(q domain.QuestionType) string {
	switch q {
	case domain.QuestionConsequence:
		return "Consequence question"
	case domain.QuestionEvidenceSufficiency:
		return "Evidence check"
	case domain.QuestionSemanticMapping:
		return "Mapping question"
	case domain.QuestionApplicability:
		return "Applicability question"
	case domain.QuestionClassification:
		return "Classification question"
	case domain.QuestionRelationship:
		return "Compatibility question"
	case domain.QuestionDuplicate:
		return "Duplicate check"
	}
	return strings.ReplaceAll(string(q), "-", " ")
}

func familyWord(f domain.SubjectFamily) string {
	switch f {
	case domain.SubjectCRDField:
		return "a field on a custom resource"
	case domain.SubjectCLIFlag:
		return "a command-line flag"
	case domain.SubjectHelmValue:
		return "a Helm value"
	case domain.SubjectConfigKey:
		return "a config file key"
	case domain.SubjectProductRelationship:
		return "a relationship to another product"
	}
	return strings.ReplaceAll(string(f), "-", " ")
}

func statusWord(s domain.ReviewStatus) string {
	return strings.ReplaceAll(string(s), "-", " ")
}

func statusTone(s domain.ReviewStatus) string {
	switch s {
	case domain.ReviewPending:
		return "accent"
	case domain.ReviewNeedsEvidence:
		return "warn"
	case domain.ReviewDecided:
		return "ok"
	}
	return "soft"
}

func whyTone(rc *knowledge.ReviewContext) string {
	if consensusAction(rc) {
		return "danger"
	}
	for _, s := range rc.Item.Routing.Signals {
		if s == domain.SignalModelsDisagree || s == domain.SignalAllUndetermined || s == domain.SignalValidationRefuted {
			return "warn"
		}
	}
	return ""
}

func classChip(c *domain.Consequence) chip {
	if c == nil {
		return chip{}
	}
	tone := "soft"
	switch c.ExposedClass {
	case domain.ImpactActionRequired:
		tone = "danger"
	case domain.ImpactReviewRequired:
		tone = "warn"
	}
	return chip{Label: classLabel(c.ExposedClass), Tone: tone}
}

func aspectsWords(as []domain.Aspect) []string {
	var out []string
	for _, a := range as {
		if w, ok := aspectWords[a]; ok {
			out = append(out, w)
		}
	}
	return out
}

// firstModel is the alphabetically-first model name behind a group of proposals
// (a deterministic tie-break for ordering groups).
func firstModel(rc *knowledge.ReviewContext, ids []string) string {
	best := ""
	for _, pid := range ids {
		for _, p := range rc.Proposals {
			if p.ID == pid && (best == "" || p.Provenance.Model < best) {
				best = p.Provenance.Model
			}
		}
	}
	return best
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func short(id string) string {
	if len(id) > 14 {
		return id[:14] + "…"
	}
	return id
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "uxv2mock:", err)
		os.Exit(1)
	}
}
