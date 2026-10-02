package drift

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// missingArtifactProposal proposes how to react to an artifact that is absent
// from every reachable channel: when every checked release misses it, the
// relationship is over — propose ending its availability at the last baseline
// pass; when only some releases miss it, propose curated exceptions.
func (a *analyzer) missingArtifactProposal(defArt *catalog.Artifact, releases []string, base baselineState) *Proposal {
	if defArt == nil {
		return nil
	}
	if len(releases) < len(a.order) {
		return &Proposal{
			Action: fmt.Sprintf("investigate why %s is absent for %s, then add exceptions or fix the channel", defArt.ID, strings.Join(releases, ", ")),
			YAML: exceptionsFragment("artifacts", defArt.ID, releases,
				fmt.Sprintf("absent from every reachable channel for these releases (reason TODO after investigating; observed until %s)", base.text)),
		}
	}
	if ceil := maxSemver(base.passes); ceil != "" {
		return &Proposal{
			Action: fmt.Sprintf("end the availability of %s at %s, the newest release where it was observed", defArt.ID, ceil),
			YAML: fragment("artifacts", defArt.ID, []string{
				fmt.Sprintf("drift: absent from every reachable channel for %s;", strings.Join(releases, ", ")),
				fmt.Sprintf("last observed for %s (%s)", ceil, base.text),
			}, []string{
				wasLine("availability", defArt.Availability, "<= "+ceil),
				"# if these releases are a one-off anomaly instead:",
				"# " + strings.ReplaceAll(exceptionsFragment("artifacts", defArt.ID, releases, "TODO reason"), "\n", "\n# "),
			}),
		}
	}
	return &Proposal{
		Action: fmt.Sprintf("investigate %s (absent for every checked release, no baseline observation), then remove it or add exceptions", defArt.ID),
		YAML: fragment("artifacts", defArt.ID, []string{
			fmt.Sprintf("drift: absent from every reachable channel for %s and never observed", strings.Join(releases, ", ")),
			"if upstream stopped publishing it, remove the artifact; otherwise fix the channel",
		}, []string{
			"# " + strings.ReplaceAll(exceptionsFragment("artifacts", defArt.ID, releases, "TODO reason"), "\n", "\n# "),
		}),
	}
}

// reorderChannelsProposal proposes moving the channel that answered to the
// front of the artifact's declared channels (a move between channels the
// definition already declares; no new host is invented).
func reorderChannelsProposal(defArt *catalog.Artifact, foundIdx int) *Proposal {
	if defArt == nil || foundIdx <= 0 || foundIdx >= len(defArt.Channels) {
		return nil
	}
	var lines []string
	for _, ch := range channelOrder(defArt.Channels, foundIdx) {
		lines = append(lines, "      - "+locatorFlow(ch))
	}
	return &Proposal{
		Action: fmt.Sprintf("reorder the channels of %s: %s answered while the leading channel no longer has it", defArt.ID, defArt.Channels[foundIdx].Kind),
		YAML: fragment("artifacts", defArt.ID, []string{
			fmt.Sprintf("source-moved: absent at the leading channel, present at %s", describeChannel(defArt.Channels[foundIdx])),
		}, append([]string{"    channels:"}, lines...)),
	}
}

// channelOrder puts channel foundIdx first and keeps the rest in order.
func channelOrder(channels []catalog.Locator, foundIdx int) []catalog.Locator {
	out := []catalog.Locator{channels[foundIdx]}
	for i, ch := range channels {
		if i != foundIdx {
			out = append(out, ch)
		}
	}
	return out
}

// sourceExceptionsProposal: a declared source location vanished; the only
// deterministic interim edit is curated exceptions for the affected releases.
func sourceExceptionsProposal(subject string, releases []string) *Proposal {
	return &Proposal{
		Action: fmt.Sprintf("find the new location of source %s (research; drift never guesses hosts), or add exceptions if the gap is temporary", subject),
		YAML:   exceptionsFragment("sources", subject, releases, "location reachable but empty; moved? (reason TODO after investigating)"),
	}
}

// availabilityProposal proposes dropping a stale availability window.
func availabilityProposal(defArt *catalog.Artifact, v domain.Version) *Proposal {
	if defArt == nil {
		return nil
	}
	return &Proposal{
		Action: fmt.Sprintf("remove or correct the availability of %s: it is declared %q but exists for %s", defArt.ID, defArt.Availability, v.Semver),
		YAML: fragment("artifacts", defArt.ID, []string{
			fmt.Sprintf("availability-violated: declared %q but the artifact exists for %s", defArt.Availability, v.Semver),
		}, []string{
			fmt.Sprintf("    availability: \"\"   %s", wasComment(defArt.Availability)),
			"    # or tighten the window to the real bound once known",
		}),
	}
}

// appearedImageProposal proposes a new artifact entry for an image repository
// that a declared artifact references with the release tag.
func appearedImageProposal(repository, tagTemplate string) *Proposal {
	base := lastSegment(repository)
	id := base
	if !strings.HasSuffix(id, "-image") {
		id += "-image"
	}
	return &Proposal{
		Action: fmt.Sprintf("declare the new image %s (release-tagged images are referenced by a declared artifact)", repository),
		YAML: fragmentNew("artifacts", []string{
			fmt.Sprintf("artifact-appeared: %s carries the release tag and is referenced by a declared artifact", repository),
		}, []string{
			"  - id: " + id,
			"    type: container-image",
			"    name: " + base,
			fmt.Sprintf("    version: {strategy: template, template: %q}", tagTemplate),
			"    channels:",
			"      - " + locatorFlow(catalog.Locator{Kind: catalog.LocatorOCI, Repository: repository}),
		}),
	}
}

// --- fragment rendering ------------------------------------------------------

// fragment renders an annotated edit of an existing entry:
//
//	<section>:
//	  - id: <id>
//	    # <comment lines…>
//	    <changed lines…>
func fragment(section, id string, comments, changed []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s:\n  - id: %s\n", section, id)
	for _, c := range comments {
		b.WriteString("    # " + c + "\n")
	}
	for _, l := range changed {
		b.WriteString(l + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// fragmentNew renders a new entry with its comment block above the entry.
func fragmentNew(section string, comments, entry []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s:\n", section)
	for _, c := range comments {
		b.WriteString("  # " + c + "\n")
	}
	for _, l := range entry {
		b.WriteString(l + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// exceptionsFragment renders an exceptions block for releases.
func exceptionsFragment(section, id string, releases []string, reason string) string {
	return fragment(section, id, nil, []string{
		"    exceptions:",
		fmt.Sprintf("      - versions: [%s]", strings.Join(releases, ", ")),
		fmt.Sprintf("        reason: %q", reason),
	})
}

func wasLine(field, old, new string) string {
	return fmt.Sprintf("    %s: %q   %s", field, new, wasComment(old))
}

func wasComment(old string) string {
	if old == "" {
		return "# was: none"
	}
	return fmt.Sprintf("# was: %q", old)
}

func lastSegment(repository string) string {
	if i := strings.LastIndex(repository, "/"); i >= 0 {
		return repository[i+1:]
	}
	return repository
}

// locatorFlow renders a channel locator as a single-line YAML flow mapping in
// definition style, e.g. {kind: oci, repository: ghcr.io/acme/charts/acme}.
func locatorFlow(loc catalog.Locator) string {
	type field struct {
		name  string
		value string
	}
	fields := []field{
		{"kind", loc.Kind}, {"repository", loc.Repository}, {"url", loc.URL},
		{"chart", loc.Chart}, {"ref", loc.Ref}, {"baseRef", loc.BaseRef},
		{"path", loc.Path}, {"glob", loc.Glob}, {"tagPattern", loc.TagPattern},
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: f.name}
		val := &yaml.Node{Kind: yaml.ScalarNode, Value: f.value}
		if needsQuote(f.value) {
			val.Style = yaml.DoubleQuotedStyle
		}
		node.Content = append(node.Content, key, val)
	}
	b, err := yaml.Marshal(node)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(string(b), "\n")
}

// needsQuote reports whether a scalar is safer double-quoted in the proposal.
func needsQuote(s string) bool {
	if strings.ContainsAny(s, "{}[],&*!|>%@`'\"#") {
		return true
	}
	return strings.Contains(s, ": ") || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ")
}

// RenderProposal renders the combined proposal document: every event's
// fragment as its own YAML document, separated by ---, under a header that
// makes explicit that nothing is applied automatically. It returns "" when no
// event has a proposal.
func RenderProposal(rep *Report) string {
	var docs []string
	for _, ev := range rep.Events {
		if ev.Proposal == nil || ev.Proposal.YAML == "" {
			continue
		}
		docs = append(docs, fmt.Sprintf("# %s %s: %s\n%s", ev.Kind, ev.Subject, ev.Proposal.Action, ev.Proposal.YAML))
	}
	if len(docs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Proposal for %s — generated by `ri drift %s` at %s\n", definitionName(rep), rep.Product, rep.GeneratedAt.Format(time.RFC3339))
	b.WriteString("# NOT applied automatically. Review each fragment against its drift event,\n# then edit the product definition by hand. Drift never guesses hosts: every\n# location mentioned here is already declared in the definition.\n")
	for _, d := range docs {
		b.WriteString("\n---\n\n" + d + "\n")
	}
	return b.String()
}

func definitionName(rep *Report) string {
	if rep.DefinitionPath != "" {
		return rep.DefinitionPath
	}
	return "products/" + string(rep.Product) + ".yaml"
}
