package discovery

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
)

// Proposed is the proposer output.
type Proposed struct {
	Definition *catalog.ProductDefinition
	YAML       []byte
	Decisions  []Decision
	Dropped    []DroppedElement
	Issues     []catalog.Issue // remaining warnings
	Coverage   []Coverage
	Open       []string
}

// DroppedElement is a draft element left out of the definition, kept in the
// report for reviewers.
type DroppedElement struct {
	Key    string `json:"key"`
	Origin string `json:"origin"`
	Reason string `json:"reason"`
	// Element is the source or artifact as proposed (JSON-friendly).
	Element any `json:"element"`
}

// Propose applies validation results to a draft and emits a definition that
// passes catalog.Validate. AI elements enter only when validated.
func Propose(d *Draft, vr *ValidationResult, proposals []Proposal, ta *TagAnalysis, candidates []Candidate, now time.Time) (*Proposed, []Proposal, error) {
	out := &Proposed{}
	def := cloneDefinition(d.Definition)
	propByID := map[string]int{}
	for i := range proposals {
		propByID[proposals[i].ID] = i
	}
	decide := func(element, action, rule, rationale string, conf domain.Confidence, method domain.Method) {
		if method == "" {
			method = domain.MethodComputed
		}
		out.Decisions = append(out.Decisions, Decision{Element: element, Action: action, Rule: rule, Rationale: rationale,
			Provenance: domain.Provenance{Method: method, Producer: ProducerValidate, Rule: rule, Confidence: conf}})
	}
	validated := vr != nil && vr.Status == ValidationDone
	// deterministic elements
	var keepSources []catalog.Source
	for _, s := range def.Sources {
		key := "source:" + s.ID
		keep, _ := applyVerdict(key, vr, ta, func(av string) { s.Availability = av }, &s.Notes, decide)
		if keep {
			s.ValidatedAgainst = validatedReleases(vr, key)
			keepSources = append(keepSources, s)
		} else {
			out.Dropped = append(out.Dropped, DroppedElement{Key: key, Origin: OriginDeterministic, Reason: "failed validation: " + vr.detail(key), Element: s})
		}
	}
	var keepArtifacts []catalog.Artifact
	for _, a := range def.Artifacts {
		key := "artifact:" + a.ID
		// An artifact found by lookup (e.g. "chart whose appVersion is the
		// release tag") legitimately does not exist for every release. When
		// it holds for some releases, absence elsewhere is a fact about the
		// artifact, not a broken relationship: keep it as optional.
		if a.Version.Strategy == catalog.VersionLookup && vr != nil && vr.Verdict(key) == VerdictFailing {
			if pass, _ := vr.counts(key); pass >= 1 {
				a.Optional = true
				a.Notes = strings.TrimSpace(a.Notes + " Not published for every release (" + vr.detail(key) + "); marked optional by validation.")
				a.ValidatedAgainst = vr.Passed[key]
				decide(key, "annotate", "validate.optional-lookup", "Lookup artifact present for "+itoa(pass)+" sampled release(s) and absent for others; kept as optional.", domain.ConfidenceMedium, domain.MethodComputed)
				keepArtifacts = append(keepArtifacts, a)
				continue
			}
		}
		keep, _ := applyVerdict(key, vr, ta, func(av string) { a.Availability = av }, &a.Notes, decide)
		if keep {
			a.ValidatedAgainst = validatedReleases(vr, key)
			keepArtifacts = append(keepArtifacts, a)
		} else {
			out.Dropped = append(out.Dropped, DroppedElement{Key: key, Origin: OriginDeterministic, Reason: "failed validation: " + vr.detail(key), Element: a})
		}
	}
	def.Sources, def.Artifacts = keepSources, keepArtifacts
	// AI elements: only validated ones enter
	usedAI := false
	setStatus := func(e *Element, status, detail string) {
		if i, ok := propByID[e.ProposalID]; ok {
			proposals[i].Status, proposals[i].Detail = status, detail
		}
	}
	for _, s := range d.AISources {
		key := "source:" + s.ID
		e := d.Elements[key]
		verdict := vr.Verdict(key)
		if verdict != VerdictValidated {
			status, detail := aiRejection(vr, key, verdict)
			setStatus(e, status, detail)
			out.Dropped = append(out.Dropped, DroppedElement{Key: key, Origin: OriginAI, Reason: "unverified AI proposal: " + detail, Element: s})
			continue
		}
		s.ID = finalAIID(def, strings.TrimSuffix(s.ID, "--ai"))
		s.ValidatedAgainst = validatedReleases(vr, key)
		s.Notes = strings.TrimSpace(s.Notes + " " + aiNote(proposals, propByID, e))
		def.Sources = append(def.Sources, s)
		setStatus(e, ProposalValidated, "validated against "+strings.Join(s.ValidatedAgainst, ", "))
		decide("source:"+s.ID, "include", "validate.ai-proposal-validated", "AI proposal "+e.ProposalID+" validated against historical releases.", domain.ConfidenceMedium, domain.MethodAI)
		usedAI = true
	}
	for _, a := range d.AIArtifacts {
		key := "artifact:" + a.ID
		e := d.Elements[key]
		verdict := vr.Verdict(key)
		if verdict != VerdictValidated {
			status, detail := aiRejection(vr, key, verdict)
			setStatus(e, status, detail)
			out.Dropped = append(out.Dropped, DroppedElement{Key: key, Origin: OriginAI, Reason: "unverified AI proposal: " + detail, Element: a})
			continue
		}
		a.ValidatedAgainst = validatedReleases(vr, key)
		a.Notes = strings.TrimSpace(a.Notes + " " + aiNote(proposals, propByID, e))
		replaced := false
		if e.Replaces != "" {
			orig := strings.TrimPrefix(e.Replaces, "artifact:")
			for i := range def.Artifacts {
				if def.Artifacts[i].ID == orig {
					if vr.Verdict(e.Replaces) == VerdictValidated && len(def.Artifacts[i].ValidatedAgainst) >= len(a.ValidatedAgainst) {
						setStatus(e, ProposalInformational, "validated, but the deterministic element validated equally well and is kept")
						replaced = true
						break
					}
					a.ID = orig
					def.Artifacts[i] = a
					replaced = true
					setStatus(e, ProposalValidated, "validated; replaces the deterministic "+e.Replaces)
					decide(e.Replaces, "replace", "validate.ai-variant-validated", "AI variant "+e.ProposalID+" validated where the deterministic element did not.", domain.ConfidenceMedium, domain.MethodAI)
					usedAI = true
				}
			}
		}
		if !replaced {
			a.ID = finalAIID(def, strings.TrimSuffix(a.ID, "--ai"))
			def.Artifacts = append(def.Artifacts, a)
			setStatus(e, ProposalValidated, "validated against "+strings.Join(a.ValidatedAgainst, ", "))
			usedAI = true
			if e.Replaces != "" {
				decide(e.Replaces, "replace", "validate.ai-variant-validated", "AI variant "+e.ProposalID+" validated; the deterministic element failed validation.", domain.ConfidenceMedium, domain.MethodAI)
			}
		}
	}
	// references must point at kept artifacts
	kept := map[string]bool{}
	for _, a := range def.Artifacts {
		kept[a.ID] = true
	}
	for i := range def.Artifacts {
		var refs []catalog.ArtifactReference
		for _, r := range def.Artifacts[i].References {
			if kept[r.Artifact] {
				refs = append(refs, r)
			}
		}
		def.Artifacts[i].References = refs
	}
	// definition provenance
	prov := &catalog.DefinitionProvenance{Method: "discovery", Author: ProducerResolve, Updated: now.UTC().Format("2006-01-02")}
	if usedAI {
		prov.Method = "discovery+llm"
	}
	switch {
	case validated:
		prov.ValidatedReleases = vr.Releases
		prov.Notes = fmt.Sprintf("Proposed by automated discovery; relationship checks run against %s.", strings.Join(vr.Releases, ", "))
	case vr != nil && vr.Detail != "":
		prov.Notes = "Proposed by automated discovery; NOT validated against releases (" + vr.Detail + "). Review before use."
	default:
		prov.Notes = "Proposed by automated discovery; NOT validated against releases. Review before use."
	}
	def.Provenance = prov
	// static validation: drop elements with errors until clean
	for round := 0; round < 20; round++ {
		rep := catalog.Validate(def)
		errs := rep.Errors()
		if len(errs) == 0 {
			out.Issues = rep.Issues
			break
		}
		removed := false
		for _, is := range errs {
			if m := elemPathRe.FindStringSubmatch(is.Path); m != nil {
				idx := atoiDefault(m[2], -1)
				if m[1] == "sources" && idx >= 0 && idx < len(def.Sources) {
					s := def.Sources[idx]
					if s.HasRole(domain.RoleVersions) && len(def.SourcesWithRole(domain.RoleVersions)) == 1 {
						continue
					}
					out.Dropped = append(out.Dropped, DroppedElement{Key: "source:" + s.ID, Origin: originOf(d, "source:"+s.ID), Reason: "static validation: " + is.String(), Element: s})
					def.Sources = append(def.Sources[:idx], def.Sources[idx+1:]...)
					decide("source:"+s.ID, "drop", "proposer.static-validation", is.String(), domain.ConfidenceHigh, "")
					removed = true
					break
				}
				if m[1] == "artifacts" && idx >= 0 && idx < len(def.Artifacts) {
					a := def.Artifacts[idx]
					out.Dropped = append(out.Dropped, DroppedElement{Key: "artifact:" + a.ID, Origin: originOf(d, "artifact:"+a.ID), Reason: "static validation: " + is.String(), Element: a})
					def.Artifacts = append(def.Artifacts[:idx], def.Artifacts[idx+1:]...)
					decide("artifact:"+a.ID, "drop", "proposer.static-validation", is.String(), domain.ConfidenceHigh, "")
					removed = true
					break
				}
			}
		}
		if !removed {
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, e.String())
			}
			return nil, proposals, fmt.Errorf("proposed definition is invalid: %s", strings.Join(msgs, "; "))
		}
	}
	y, err := catalog.Marshal(def)
	if err != nil {
		return nil, proposals, err
	}
	out.Definition, out.YAML = def, y
	out.Coverage = coverage(def, d, candidates)
	out.Open = append(out.Open, d.Open...)
	for _, dec := range out.Decisions {
		if dec.Rule == "validate.single-failure-kept" {
			out.Open = append(out.Open, fmt.Sprintf("%s failed validation for one release and was kept: %s", dec.Element, dec.Rationale))
		}
	}
	if !validated {
		out.Open = append(out.Open, "Run the relationship checks (validation) before adopting this definition.")
	}
	for _, p := range proposals {
		if p.Status == ProposalUnverified || p.Status == ProposalFailed {
			out.Open = append(out.Open, fmt.Sprintf("Unverified AI proposal %s: %s (%s).", p.ID, p.Summary, p.Detail))
		}
	}
	return out, proposals, nil
}

var elemPathRe = regexp.MustCompile(`^(sources|artifacts)\[(\d+)\]`)

func originOf(d *Draft, key string) string {
	if e, ok := d.Elements[key]; ok {
		return e.Origin
	}
	return OriginDeterministic
}

func finalAIID(def *catalog.ProductDefinition, id string) string {
	used := map[string]bool{}
	for _, s := range def.Sources {
		used[s.ID] = true
	}
	for _, a := range def.Artifacts {
		used[a.ID] = true
	}
	if !used[id] {
		return id
	}
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s-ai-%d", id, i)
		if !used[c] {
			return c
		}
	}
}

func aiNote(props []Proposal, idx map[string]int, e *Element) string {
	if e == nil {
		return ""
	}
	i, ok := idx[e.ProposalID]
	if !ok {
		return ""
	}
	p := props[i]
	return fmt.Sprintf("AI-proposed (%s, model %s, prompt %s) and validated.", p.ID, p.Provenance.Model, shortDigest(p.Provenance.PromptDigest))
}

func shortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		return "sha256:" + d[:12]
	}
	return d
}

func aiRejection(vr *ValidationResult, key, verdict string) (string, string) {
	switch verdict {
	case VerdictFailing:
		return ProposalFailed, "failed validation: " + vr.detail(key)
	case VerdictNotChecked:
		if vr == nil || vr.Status != ValidationDone {
			return ProposalUnverified, "validation did not run"
		}
		return ProposalUnverified, "not checked by the relationship checker"
	}
	return ProposalUnverified, verdict + ": " + vr.detail(key)
}

// applyVerdict applies a validation verdict to a deterministic element.
func applyVerdict(key string, vr *ValidationResult, ta *TagAnalysis, setAvail func(string), notes *string,
	decide func(element, action, rule, rationale string, conf domain.Confidence, method domain.Method)) (bool, string) {
	verdict := vr.Verdict(key)
	add := func(s string) {
		*notes = strings.TrimSpace(*notes + " " + s)
	}
	switch verdict {
	case VerdictValidated, VerdictNotChecked:
		return true, verdict
	case VerdictFailing:
		if first, ok := vr.failuresBeforePasses(key, ta); ok {
			av := fmt.Sprintf(">= %d.%d.0", first.Major(), first.Minor())
			setAvail(av)
			add("Availability inferred from validation (" + vr.detail(key) + ").")
			decide(key, "annotate", "validate.availability-inferred", "Fails only for releases older than every passing one; availability "+av+".", domain.ConfidenceMedium, domain.MethodComputed)
			return true, verdict
		}
		if pass, fail := vr.counts(key); fail == 1 && pass >= ingest.MinValidations-1 {
			add("Failed validation for one release (" + vr.detail(key) + "); possibly published late or elsewhere.")
			decide(key, "annotate", "validate.single-failure-kept", "Holds for "+itoa(pass)+" releases but failed for one ("+vr.detail(key)+"); kept with a comment for review.", domain.ConfidenceLow, domain.MethodComputed)
			return true, verdict
		}
		decide(key, "drop", "validate.failed", "Relationship check failed: "+vr.detail(key)+".", domain.ConfidenceHigh, domain.MethodComputed)
		return false, verdict
	default:
		add("Unverified by discovery (" + verdict + ": " + vr.detail(key) + ").")
		decide(key, "annotate", "validate."+verdict, "Kept unverified: "+vr.detail(key)+".", domain.ConfidenceLow, domain.MethodComputed)
		return true, verdict
	}
}

func validatedReleases(vr *ValidationResult, key string) []string {
	if vr == nil || vr.Status != ValidationDone || vr.Verdict(key) != VerdictValidated {
		return nil
	}
	return append([]string(nil), vr.Passed[key]...)
}

func cloneDefinition(d *catalog.ProductDefinition) *catalog.ProductDefinition {
	c := *d
	c.Sources = append([]catalog.Source(nil), d.Sources...)
	c.Artifacts = append([]catalog.Artifact(nil), d.Artifacts...)
	return &c
}

// coverage summarises the ten discovery targets.
func coverage(def *catalog.ProductDefinition, d *Draft, cands []Candidate) []Coverage {
	kinds := map[string][]CandidateKind{
		TargetReleaseSource:    {KindTagScheme, KindReleaseTrigger, KindReleasePublisher, KindReleaseAsset},
		TargetReleaseNotes:     {KindReleaseNotes, KindNotesDir},
		TargetChangelog:        {KindChangelog},
		TargetHelmCharts:       {KindHelmChart, KindHelmRepo, KindHelmOCI, KindChartRepo},
		TargetRegistries:       {KindRegistry},
		TargetImages:           {KindImage, KindImageName},
		TargetUpgradeDocs:      {KindUpgradeGuide},
		TargetCompatibility:    {KindCompatibility},
		TargetSecurity:         {KindSecurityPolicy, KindAdvisories, KindSecurityDocs},
		TargetVersionRelations: {KindVersionRelation},
	}
	inDef := map[string]bool{}
	for _, s := range def.Sources {
		inDef["source:"+s.ID] = true
	}
	for _, a := range def.Artifacts {
		inDef["artifact:"+a.ID] = true
	}
	var out []Coverage
	for _, t := range AllTargets {
		cv := Coverage{Target: t}
		for _, c := range cands {
			for _, k := range kinds[t] {
				if c.Kind == k {
					cv.Candidates++
				}
			}
		}
		var keys []string
		for k, e := range d.Elements {
			if containsStr(e.Targets, t) && (inDef[k] || (e.Origin == OriginAI && inDef[strings.Replace(k, "--ai", "", 1)])) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		cv.InDraft = keys
		cv.Findings = findings(t, def)
		switch {
		case len(keys) > 0:
			cv.Status = "found"
		case cv.Candidates > 0:
			cv.Status = "candidates-only"
		default:
			cv.Status = "not-found"
		}
		out = append(out, cv)
	}
	return out
}

func findings(t string, def *catalog.ProductDefinition) []string {
	var out []string
	switch t {
	case TargetReleaseSource:
		for _, s := range def.SourcesWithRole(domain.RoleVersions) {
			out = append(out, s.Locator.Kind+" "+s.Locator.Repository)
		}
	case TargetReleaseNotes, TargetChangelog, TargetUpgradeDocs, TargetCompatibility, TargetSecurity:
		role := map[string]domain.SourceRole{TargetReleaseNotes: domain.RoleReleaseNotes, TargetChangelog: domain.RoleChangelog,
			TargetUpgradeDocs: domain.RoleUpgradeGuide, TargetCompatibility: domain.RoleCompatibility, TargetSecurity: domain.RoleSecurity}[t]
		for _, s := range def.SourcesWithRole(role) {
			out = append(out, strings.TrimSpace(s.Locator.Kind+" "+s.Locator.Repository+" "+s.Locator.Path+refSuffix(s.Locator.Ref)))
		}
	case TargetHelmCharts:
		for _, a := range def.Artifacts {
			if a.Type == domain.ArtifactHelmChart {
				out = append(out, a.Name+" via "+locatorList(a.Channels))
			}
		}
	case TargetImages, TargetRegistries:
		regs := map[string]bool{}
		for _, a := range def.Artifacts {
			if a.Type != domain.ArtifactContainerImage {
				continue
			}
			if t == TargetImages {
				out = append(out, a.Channels[0].Repository+":"+a.Version.Template)
			}
			for _, ch := range a.Channels {
				if i := strings.LastIndex(ch.Repository, "/"); i > 0 {
					regs[ch.Repository[:i]] = true
				}
			}
		}
		if t == TargetRegistries {
			for r := range regs {
				out = append(out, r)
			}
			sort.Strings(out)
		}
	case TargetVersionRelations:
		for _, a := range def.Artifacts {
			v := a.Version
			switch v.Strategy {
			case catalog.VersionTemplate:
				out = append(out, a.ID+": version = "+v.Template)
			case catalog.VersionLookup:
				out = append(out, a.ID+": lookup "+v.Field+" == "+v.Match)
			default:
				out = append(out, a.ID+": "+v.Strategy)
			}
		}
	}
	return out
}

func refSuffix(ref string) string {
	if ref == "" {
		return ""
	}
	return " @" + ref
}
