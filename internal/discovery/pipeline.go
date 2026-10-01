package discovery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// Discoverer runs the discovery pipeline. Only Checkout and Tags are
// required; LLM and Checker are optional.
type Discoverer struct {
	Checkout Checkout
	Tags     TagLister
	Scanner  *Scanner
	// LLM enables the AI ambiguity resolver.
	LLM llm.Client
	// Model overrides the LLM client's default model.
	Model string
	// Checker validates the draft against historical releases (wire
	// *ingest.Ingester in production).
	Checker RelationshipChecker
	// ValidationReleases is the number of releases to check (default 4).
	ValidationReleases int
	// MaxDocsRepos bounds how many referenced documentation repositories
	// are scanned (default 2).
	MaxDocsRepos int
	Clock        func() time.Time
}

// Request describes one discovery run.
type Request struct {
	// Repository: owner/name, host/owner/name or URL.
	Repository string
	// Ref to scan; default the latest stable tag (the default branch when
	// the repository has no release tags).
	Ref       string
	ProductID string
	Name      string
	// LocalDir scans an existing directory instead of cloning.
	LocalDir string
	// NoFollow disables scanning referenced documentation repositories.
	NoFollow bool
	// NoLLM disables the AI resolver even when a client is configured.
	NoLLM bool
}

// Result is the outcome of a discovery run.
type Result struct {
	Definition *catalog.ProductDefinition
	YAML       []byte
	Report     *Report
}

func (d *Discoverer) now() time.Time {
	if d.Clock != nil {
		return d.Clock().UTC()
	}
	return time.Now().UTC()
}

// Run executes checkout → scan → resolve → (LLM) → validate → propose.
func (d *Discoverer) Run(ctx context.Context, req Request) (*Result, error) {
	if d.Checkout == nil || d.Tags == nil {
		return nil, errors.New("discovery: Checkout and Tags are required")
	}
	repo, err := ParseRepo(req.Repository)
	if err != nil {
		return nil, err
	}
	rep := &Report{Repository: repo.String(), GeneratedAt: d.now()}
	// 1. tags
	tags, err := d.Tags.ListTags(ctx, repo)
	if err != nil {
		if req.LocalDir == "" {
			return nil, fmt.Errorf("list tags of %s: %w", repo, err)
		}
		rep.Errors = append(rep.Errors, "list tags: "+err.Error())
	}
	ta := AnalyzeTags(repo, tags)
	ta.ListedAt = d.now()
	rep.Tags = ta
	ref := req.Ref
	if ref == "" {
		ref = ta.Latest
	}
	refVersion, _ := ta.Find(ref)
	// 2. checkout
	var tree Tree
	if req.LocalDir != "" {
		info := TreeInfo{Repo: repo, Ref: ref, RefIsTag: !refVersion.IsZero(), RetrievedAt: d.now()}
		info.PinRef = pinRef(info)
		tree, err = NewDirTree(req.LocalDir, info)
	} else {
		tree, err = d.Checkout.Checkout(ctx, repo, ref, CheckoutOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("checkout %s@%s: %w", repo, ref, err)
	}
	info := tree.Info()
	rep.Ref, rep.Commit = info.Ref, info.Commit
	productID := req.ProductID
	if productID == "" {
		productID = repo.Name
	}
	hints := ProductHints{Owner: repo.Owner, Name: repo.Name, ID: productID}
	scanner := d.Scanner
	if scanner == nil {
		scanner = &Scanner{}
	}
	// 3. scan
	main, err := scanner.Scan(ctx, tree, ScanInput{Profile: ProfileSource, Product: hints, Main: repo, Tags: ta, RefVersion: refVersion})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", repo, err)
	}
	// 3a. tag-train disambiguation: several version trains live in this
	// repository and the scan evidence (release triggers, chart appVersion)
	// names another one than the most numerous. Re-checkout at the new
	// train's latest release and re-scan, so every downstream inference
	// (path templates, image tag classes, relations) sees the right ref.
	if req.Ref == "" {
		if f, why := selectTagTrain(ta, main.Candidates); f != nil {
			nt := ta.SwitchFamily(*f, ta.raw)
			nt.TrainSwitch = &TrainSwitch{From: ta.Families[0], To: *f, Rationale: why}
			if t2, cerr := d.Checkout.Checkout(ctx, repo, nt.Latest, CheckoutOptions{}); cerr == nil {
				rv, _ := nt.Find(nt.Latest)
				if m2, serr := scanner.Scan(ctx, t2, ScanInput{Profile: ProfileSource, Product: hints, Main: repo, Tags: nt, RefVersion: rv}); serr == nil {
					ta, tree, main = nt, t2, m2
					ref, refVersion = nt.Latest, rv
					rep.Tags, rep.Ref = ta, ref
					info := tree.Info()
					rep.Commit = info.Commit
				}
			}
		}
	}
	cands := append([]Candidate{tagCandidate(ta, d.now())}, main.Candidates...)
	rep.Scans = append(rep.Scans, *main)
	// 3b. referenced repositories
	docsListings := map[string][]string{}
	chartTags := map[string][]RemoteTag{}
	if !req.NoFollow {
		maxDocs := d.MaxDocsRepos
		if maxDocs <= 0 {
			maxDocs = 2
		}
		for i, dc := range rankByOccurrence(filterKind(cands, KindDocsRepo)) {
			if i >= maxDocs || dc.Confidence == domain.ConfidenceLow {
				break
			}
			dr, err := ParseRepo(dc.Value)
			if err != nil {
				continue
			}
			branch, err := d.Tags.DefaultBranch(ctx, dr)
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("default branch of %s: %v", dr, err))
				continue
			}
			dt, err := d.Checkout.Checkout(ctx, dr, branch, CheckoutOptions{Partial: true})
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("checkout %s: %v", dr, err))
				continue
			}
			res, err := scanner.Scan(ctx, dt, ScanInput{Profile: ProfileDocs, Product: hints, Main: repo, Tags: ta})
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("scan %s: %v", dr, err))
				continue
			}
			rep.Scans = append(rep.Scans, *res)
			cands = mergeCandidates(cands, res.Candidates)
			for _, f := range dt.Files() {
				if docsWorthReading(f.Path) && docRole(f.Path) != "" {
					docsListings[dr.String()] = append(docsListings[dr.String()], f.Path)
				}
			}
		}
		for _, cc := range filterKind(cands, KindChartRepo) {
			cr, err := ParseRepo(cc.Value)
			if err != nil {
				continue
			}
			if t, err := d.Tags.ListTags(ctx, cr); err == nil {
				chartTags[cc.Value] = t
			} else {
				rep.Errors = append(rep.Errors, fmt.Sprintf("list tags of %s: %v", cr, err))
			}
		}
	}
	// 4. resolve
	in := ResolveInput{Repo: repo, ProductID: productID, Name: req.Name, Tags: ta, ScanRef: refVersion, Candidates: cands,
		ChartRepoTags: chartTags, DocsListings: docsListings, Clock: d.Clock}
	draft, err := Resolve(in)
	if err != nil {
		return nil, err
	}
	// 5. LLM (optional)
	var proposals []Proposal
	if d.LLM != nil && !req.NoLLM && len(draft.Questions) > 0 {
		lr := &LLMResolver{Client: d.LLM, Model: d.Model, Clock: d.Clock}
		props, errs := lr.Resolve(ctx, in, draft)
		proposals = props
		for _, e := range errs {
			rep.Errors = append(rep.Errors, "llm: "+e.Error())
		}
		for _, p := range props {
			if p.Provenance.Model != "" {
				rep.LLM = p.Provenance.Model
				break
			}
		}
		if rep.LLM == "" {
			rep.LLM = "configured (no answer)"
		}
	}
	// 6. validate
	vr := (&Validator{Checker: d.Checker, Releases: d.ValidationReleases}).Validate(ctx, draft, ta)
	// 7. propose
	out, proposals, err := Propose(draft, vr, proposals, ta, cands, d.now())
	if err != nil {
		return nil, err
	}
	rep.Product, rep.Name = out.Definition.ID, out.Definition.Name
	rep.Candidates = cands
	for _, k := range sortedKeys(draft.Elements) {
		rep.Elements = append(rep.Elements, draft.Elements[k])
	}
	rep.Decisions = append(append([]Decision{}, draft.Decisions...), out.Decisions...)
	rep.Questions = draft.Questions
	rep.Proposals = proposals
	rep.Validation = vr
	rep.Dropped = out.Dropped
	rep.Coverage = out.Coverage
	rep.Open = out.Open
	if rep.LLM == "" {
		for _, q := range draft.Questions {
			rep.Open = append(rep.Open, "Ambiguity ("+q.Kind+"): "+q.Summary)
		}
	}
	rep.StaticIssues = out.Issues
	rep.Definition = string(out.YAML)
	return &Result{Definition: out.Definition, YAML: out.YAML, Report: rep}, nil
}

func sortedKeys(m map[string]*Element) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// tagCandidate turns the tag analysis into a candidate with git-ref evidence.
func tagCandidate(ta *TagAnalysis, now time.Time) Candidate {
	c := Candidate{Kind: KindTagScheme, Value: ta.TagRegex(), Confidence: domain.ConfidenceHigh, Rules: []string{"tags.ls-remote"},
		Attributes: map[string]string{"prefix": ta.Prefix, "latest": ta.Latest, "stable": itoa(ta.StableCount), "prereleases": itoa(ta.PrereleaseCount),
			"junk": strings.Join(firstN(ta.Junk, 8), ","), "lineage": ta.Lineage}}
	if ta.Scheme == SchemeComponentGroups {
		c.Attributes["scheme"] = SchemeComponentGroups
		c.Attributes["matches"] = ta.PatternCoverage
		c.Confidence = domain.ConfidenceMedium
	}
	if ta.Latest != "" {
		c.Evidence = append(c.Evidence, ta.Evidence(ta.Latest, now))
	}
	for _, j := range firstN(ta.Junk, 2) {
		c.Evidence = append(c.Evidence, ta.Evidence(j, now))
	}
	if ta.StableCount == 0 {
		c.Confidence = domain.ConfidenceLow
	}
	c.ID = candidateID(c.Kind, c.Value)
	return c
}

func filterKind(cs []Candidate, k CandidateKind) []Candidate {
	var out []Candidate
	for _, c := range cs {
		if c.Kind == k {
			out = append(out, c)
		}
	}
	return out
}

func rankByOccurrence(cs []Candidate) []Candidate {
	out := append([]Candidate(nil), cs...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := confRank(out[i].Confidence), confRank(out[j].Confidence)
		if ri != rj {
			return ri > rj
		}
		return atoiDefault(out[i].Attr("occurrences"), 1) > atoiDefault(out[j].Attr("occurrences"), 1)
	})
	return out
}

// mergeCandidates folds b into a (same kind and value merge).
func mergeCandidates(a, b []Candidate) []Candidate {
	idx := map[string]int{}
	for i, c := range a {
		idx[string(c.Kind)+"\x00"+c.Value] = i
	}
	for _, c := range b {
		k := string(c.Kind) + "\x00" + c.Value
		if i, ok := idx[k]; ok {
			a[i].merge(c)
			continue
		}
		idx[k] = len(a)
		a = append(a, c)
	}
	return a
}
