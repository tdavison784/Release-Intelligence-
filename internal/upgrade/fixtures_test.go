package upgrade

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Test fixtures: synthetic but realistic releases modelled on the upstream
// research in docs/research (cert-manager, Argo CD). Advisory ids marked
// "synt" are fictional.

var fixedNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func ver(tag string) domain.Version { return domain.MustVersion(tag, strings.TrimPrefix(tag, "v")) }

func versions(tags ...string) []domain.Version {
	out := make([]domain.Version, len(tags))
	for i, t := range tags {
		out[i] = ver(t)
	}
	return out
}

// series returns prefix+"major.minor.0" .. prefix+"major.minor.last".
func series(prefix string, major, minor, last int) []string {
	var out []string
	for p := 0; p <= last; p++ {
		out = append(out, fmt.Sprintf("%s%d.%d.%d", prefix, major, minor, p))
	}
	return out
}

func declared(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.notes@v1", Rule: rule, Confidence: domain.ConfidenceHigh}
}

func heuristic(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodHeuristic, Producer: "normalize.notes@v1", Rule: rule, Confidence: domain.ConfidenceMedium}
}

// rel builds a domain.Release with evidence and facts for everything added.
type rel struct {
	r    *domain.Release
	line int
}

func newRel(product, tag string) *rel {
	v := ver(tag)
	pub := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(v.Minor()*30+v.Patch()) * 24 * time.Hour)
	return &rel{r: &domain.Release{Product: domain.ProductID(product), Version: v, PublishedAt: &pub, IngestedAt: fixedNow}, line: 10}
}

func (b *rel) addEvidence(kind domain.EvidenceKind, source, uri, loc, excerpt string) domain.EvidenceID {
	e := domain.NewEvidence(kind, source, uri, loc, excerpt, domain.Digest([]byte(uri)), fixedNow)
	for _, x := range b.r.Evidence {
		if x.ID == e.ID {
			return e.ID
		}
	}
	b.r.Evidence = append(b.r.Evidence, e)
	return e.ID
}

type noteOpt func(*domain.NoteItem)

func breaking(it *domain.NoteItem) { it.Breaking = true }
func action(it *domain.NoteItem)   { it.ActionRequired = true }
func prov(p domain.Provenance) noteOpt {
	return func(it *domain.NoteItem) { it.Classification = p }
}
func refs(rs ...domain.Reference) noteOpt {
	return func(it *domain.NoteItem) { it.References = rs }
}

func (b *rel) note(source string, role domain.SourceRole, uri, section, text string, cat domain.Category, opts ...noteOpt) *rel {
	loc := fmt.Sprintf("L%d-L%d", b.line, b.line+1)
	b.line += 3
	id := b.addEvidence(domain.EvidenceDocument, source, uri, loc, text)
	it := domain.NoteItem{
		ID: "note-" + domain.ShortHash(b.r.Version.Semver, text), Release: b.r.Version.Semver, SourceID: source, Role: role,
		Section: section, Text: text, Category: cat, Classification: declared("section:" + section), Evidence: []domain.EvidenceID{id},
	}
	for _, o := range opts {
		o(&it)
	}
	b.r.Notes = append(b.r.Notes, it)
	return b
}

func (b *rel) snapshot(s domain.Snapshot, uri string) *rel {
	id := b.addEvidence(domain.EvidenceStructured, s.ArtifactID, uri, "", string(s.Kind)+" of "+s.ArtifactID)
	s.Evidence = []domain.EvidenceID{id}
	b.r.Snapshots = append(b.r.Snapshots, s)
	b.r.Facts = append(b.r.Facts, domain.NewFact(domain.FactSnapshot, s.ArtifactID, b.r.Version.Semver,
		fmt.Sprintf("%s snapshot of %s taken", s.Kind, s.ArtifactID), "ingest@v1", nil, id))
	return b
}

func (b *rel) values(artifact, chart, uri string, kv ...string) *rel {
	entries := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		entries[kv[i]] = kv[i+1]
	}
	return b.snapshot(domain.Snapshot{ArtifactID: artifact, Kind: domain.SnapshotHelmValues,
		Values: &domain.ValuesSnapshot{Chart: chart, Version: b.r.Version.Tag, Entries: entries}}, uri)
}

func (b *rel) crds(artifact, uri string, crds ...domain.CRDSummary) *rel {
	return b.snapshot(domain.Snapshot{ArtifactID: artifact, Kind: domain.SnapshotCRDs, CRDs: &domain.CRDSnapshot{CRDs: crds}}, uri)
}

func (b *rel) images(artifact, uri string, refs ...string) *rel {
	var out []domain.ImageRef
	for _, s := range refs {
		repo, tag := splitCoordinate(s)
		out = append(out, domain.ImageRef{Repository: repo, Tag: tag})
	}
	return b.snapshot(domain.Snapshot{ArtifactID: artifact, Kind: domain.SnapshotImages, Images: &domain.ImageRefsSnapshot{Images: out}}, uri)
}

func (b *rel) artifact(id string, typ domain.ArtifactType, name, coord string, status domain.ArtifactStatus, detail string) *rel {
	inst := domain.ArtifactInstance{ArtifactID: id, Type: typ, Name: name, Coordinate: coord, Status: status, Detail: detail}
	if _, v := splitCoordinate(coord); v != "" {
		inst.Version = v
	}
	switch status {
	case domain.ArtifactVerified, domain.ArtifactReferenced:
		kind := domain.EvidenceRegistry
		if status == domain.ArtifactReferenced {
			kind = domain.EvidenceDocument
		}
		eid := b.addEvidence(kind, id, "https://artifacts.example/"+coord, "", string(status)+" "+coord)
		inst.Evidence = []domain.EvidenceID{eid}
		b.r.Facts = append(b.r.Facts, domain.NewFact(domain.FactArtifactPublished, coord, b.r.Version.Semver,
			fmt.Sprintf("%s %s is %s", typ, coord, status), "ingest@v1", nil, eid))
	}
	b.r.Artifacts = append(b.r.Artifacts, inst)
	return b
}

func (b *rel) compat(platform, kind, constraint, raw, uri string, vs ...string) *rel {
	id := b.addEvidence(domain.EvidenceDocument, "compat", uri, fmt.Sprintf("row %s", b.r.Version.Line()), raw)
	c := domain.CompatibilityConstraint{Platform: platform, Kind: kind, Constraint: constraint, Raw: raw, Versions: vs, SourceID: "compat",
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh}, Evidence: []domain.EvidenceID{id}}
	b.r.Compat = append(b.r.Compat, c)
	b.r.Facts = append(b.r.Facts, domain.NewFact(domain.FactCompatibility, platform, b.r.Version.Semver,
		fmt.Sprintf("%s %s: %s", platform, kind, raw), "normalize.table@v1", nil, id))
	return b
}

func (b *rel) source(id, kind string, state domain.SourceState, detail string, roles ...domain.SourceRole) *rel {
	b.r.Sources = append(b.r.Sources, domain.SourceStatus{SourceID: id, Kind: kind, Roles: roles, Version: b.r.Version.Semver, State: state, Detail: detail})
	return b
}

func crd(name, group, kind string, versions ...domain.CRDVersionInfo) domain.CRDSummary {
	return domain.CRDSummary{Name: name, Group: group, Kind: kind, Scope: "Namespaced", Versions: versions}
}

func crdVer(name string, served, storage bool, paths ...string) domain.CRDVersionInfo {
	return domain.CRDVersionInfo{Name: name, Served: served, Storage: storage, SchemaPaths: paths}
}

func advisoryEvidence(id, url, excerpt string) domain.Evidence {
	return domain.NewEvidence(domain.EvidenceAdvisory, "advisories", url, "", excerpt, "", fixedNow)
}

// ---------------------------------------------------------------- cert-manager

const (
	cmNotes   = "https://raw.githubusercontent.com/cert-manager/website/master/content/docs/releases/release-notes/release-notes-1.15.md"
	cmUpgrade = "https://raw.githubusercontent.com/cert-manager/website/master/content/docs/releases/upgrading/upgrading-1.14-1.15.md"
	cmCompat  = "https://raw.githubusercontent.com/cert-manager/website/master/content/docs/releases/README.md"
)

func cmDefinition() *catalog.ProductDefinition {
	return &catalog.ProductDefinition{
		APIVersion: catalog.APIVersion, Kind: catalog.Kind, ID: "cert-manager", Name: "cert-manager",
		Versioning: catalog.Versioning{Scheme: domain.SchemeSemver, TagPrefix: "v", Lineage: catalog.LineageMinor},
		Artifacts: []catalog.Artifact{
			{ID: "manifest", Type: domain.ArtifactManifest}, {ID: "crds", Type: domain.ArtifactCRD}, {ID: "chart", Type: domain.ArtifactHelmChart},
			{ID: "controller", Type: domain.ArtifactContainerImage}, {ID: "webhook", Type: domain.ArtifactContainerImage},
			{ID: "cainjector", Type: domain.ArtifactContainerImage}, {ID: "acmesolver", Type: domain.ArtifactContainerImage},
			{ID: "startupapicheck", Type: domain.ArtifactContainerImage}, {ID: "ctl", Type: domain.ArtifactContainerImage},
		},
	}
}

func cmCommon(b *rel, tag string) *rel {
	gh := "https://github.com/cert-manager/cert-manager/releases/download/" + tag
	b.source("notes", "repo-file", domain.SourceOK, "", domain.RoleReleaseNotes).
		source("github-assets", "http", domain.SourceOK, "", domain.RoleVersions).
		source("quay", "oci", domain.SourceUnavailable, "quay.io blocked (HTTP 403)").
		artifact("manifest", domain.ArtifactManifest, "cert-manager.yaml", gh+"/cert-manager.yaml", domain.ArtifactVerified, "").
		artifact("crds", domain.ArtifactCRD, "cert-manager.crds.yaml", gh+"/cert-manager.crds.yaml", domain.ArtifactVerified, "").
		artifact("chart", domain.ArtifactHelmChart, "cert-manager", "quay.io/jetstack/charts/cert-manager:"+tag, domain.ArtifactExpected, "").
		artifact("controller", domain.ArtifactContainerImage, "cert-manager-controller", "quay.io/jetstack/cert-manager-controller:"+tag, domain.ArtifactReferenced, "").
		artifact("webhook", domain.ArtifactContainerImage, "cert-manager-webhook", "quay.io/jetstack/cert-manager-webhook:"+tag, domain.ArtifactReferenced, "").
		artifact("cainjector", domain.ArtifactContainerImage, "cert-manager-cainjector", "quay.io/jetstack/cert-manager-cainjector:"+tag, domain.ArtifactReferenced, "").
		artifact("acmesolver", domain.ArtifactContainerImage, "cert-manager-acmesolver", "quay.io/jetstack/cert-manager-acmesolver:"+tag, domain.ArtifactReferenced, "").
		artifact("startupapicheck", domain.ArtifactContainerImage, "cert-manager-startupapicheck", "quay.io/jetstack/cert-manager-startupapicheck:"+tag, domain.ArtifactExpected, "")
	b.images("manifest", gh+"/cert-manager.yaml",
		"quay.io/jetstack/cert-manager-cainjector:"+tag, "quay.io/jetstack/cert-manager-controller:"+tag,
		"quay.io/jetstack/cert-manager-acmesolver:"+tag, "quay.io/jetstack/cert-manager-webhook:"+tag)
	return b
}

var certPaths = []string{"spec", "spec.secretName", "spec.keystores", "spec.keystores.pkcs12", "spec.keystores.pkcs12.create", "spec.keystores.pkcs12.passwordSecretRef"}

func cmFrom() *domain.Release {
	b := newRel("cert-manager", "v1.14.4")
	cmCommon(b, "v1.14.4")
	b.source("compat", "repo-file", domain.SourceOK, "", domain.RoleCompatibility).
		artifact("ctl", domain.ArtifactContainerImage, "cert-manager-ctl", "quay.io/jetstack/cert-manager-ctl:v1.14.4", domain.ArtifactReferenced, "").
		note("notes", domain.RoleReleaseNotes, cmNotes, "Bug or Regression", "This item belongs to the source release and must not be reported.", domain.CategoryBugfix).
		values("chart", "cert-manager", "https://raw.githubusercontent.com/cert-manager/cert-manager/v1.14.4/deploy/charts/cert-manager/values.yaml",
			"installCRDs", "false",
			"global.leaderElection.namespace", `"kube-system"`,
			"global.podSecurityPolicy.enabled", "false",
			"global.podSecurityPolicy.useAppArmor", "true",
			"webhook.timeoutSeconds", "10",
			"startupapicheck.image.repository", `"quay.io/jetstack/cert-manager-ctl"`,
			"prometheus.enabled", "true").
		crds("crds", "https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.crds.yaml",
			crd("certificates.cert-manager.io", "cert-manager.io", "Certificate", crdVer("v1", true, true, certPaths...)),
			crd("issuers.cert-manager.io", "cert-manager.io", "Issuer", crdVer("v1", true, true, "spec", "spec.acme")),
		).
		compat("kubernetes", "supported", ">=1.24.0-0 <=1.29.x", "1.24 → 1.29", cmCompat).
		compat("openshift", "supported", ">=4.11.0-0 <=4.16.x", "4.11 → 4.16", cmCompat)
	b.r.Facts = append(b.r.Facts, domain.NewFact(domain.FactReleasePublished, "cert-manager@1.14.4", "1.14.4", "tag v1.14.4 exists", "ingest@v1", nil))
	return b.r
}

func cmV1150() *domain.Release {
	b := newRel("cert-manager", "v1.15.0")
	cmCommon(b, "v1.15.0")
	b.source("upgrade", "repo-file", domain.SourceOK, "", domain.RoleUpgradeGuide).
		artifact("ctl", domain.ArtifactContainerImage, "cert-manager-ctl", "", domain.ArtifactNotApplicable, "availability < 1.15.0").
		note("notes", domain.RoleReleaseNotes, cmNotes, "Major Themes",
			"The `cmctl` binary and the `quay.io/jetstack/cert-manager-ctl` image are no longer published from this repository. cmctl now lives in its own repository at github.com/cert-manager/cmctl and is released independently.",
			domain.CategoryRemoval, breaking).
		note("notes", domain.RoleReleaseNotes, cmNotes, "Major Themes",
			"Helm: the `installCRDs` value is deprecated in favour of `crds.enabled`.", domain.CategoryDeprecation).
		note("notes", domain.RoleReleaseNotes, cmNotes, "Feature",
			"Add the `crds.keep` Helm value to retain CRDs when the chart is uninstalled ([`#6760`](https://github.com/cert-manager/cert-manager/pull/6760), [`@inteon`](https://github.com/inteon))",
			domain.CategoryFeature, refs(domain.Reference{Type: "pull-request", ID: "#6760", URL: "https://github.com/cert-manager/cert-manager/pull/6760"})).
		note("notes", domain.RoleReleaseNotes, cmNotes, "Feature",
			"Add the PKCS#12 `profile` field to Certificate keystores so that legacy Java keystores can be generated, e.g. for older JREs ([`#6919`](https://github.com/cert-manager/cert-manager/pull/6919), [`@ThatsMrTalbot`](https://github.com/ThatsMrTalbot))",
			domain.CategoryFeature).
		note("notes", domain.RoleReleaseNotes, cmNotes, "Bug or Regression",
			"Fix a panic in the ACME issuer when the account key was rotated ([`#7001`](https://github.com/cert-manager/cert-manager/pull/7001), [`@wallrj`](https://github.com/wallrj))",
			domain.CategoryBugfix).
		note("notes", domain.RoleReleaseNotes, cmNotes, "Other (Cleanup or Flake)",
			"Upgrade Go to 1.22.4 ([`#7050`](https://github.com/cert-manager/cert-manager/pull/7050), [`@cert-manager-bot`](https://github.com/cert-manager-bot))",
			domain.CategoryOther).
		note("upgrade", domain.RoleUpgradeGuide, cmUpgrade, "Upgrading from v1.14 to v1.15",
			"If you use `cmctl` or the `cert-manager-ctl` image in automation, switch to the standalone cmctl release before upgrading.",
			domain.CategoryMigration, action)
	return b.r
}

func cmTo() *domain.Release {
	b := newRel("cert-manager", "v1.15.1")
	cmCommon(b, "v1.15.1")
	b.source("compat", "repo-file", domain.SourceOK, "", domain.RoleCompatibility).
		source("upgrade", "repo-file", domain.SourceSkipped, "release kind patch", domain.RoleUpgradeGuide).
		artifact("ctl", domain.ArtifactContainerImage, "cert-manager-ctl", "", domain.ArtifactNotApplicable, "availability < 1.15.0").
		note("notes", domain.RoleReleaseNotes, cmNotes, "Bug or Regression",
			"Fix a panic in the ACME issuer when the account key was rotated ([`#7123`](https://github.com/cert-manager/cert-manager/pull/7123), [`@cert-manager-bot`](https://github.com/cert-manager-bot))",
			domain.CategoryBugfix).
		note("notes", domain.RoleReleaseNotes, cmNotes, "Bug or Regression",
			"Bump `golang.org/x/net` to address CVE-2024-45338 ([`#7140`](https://github.com/cert-manager/cert-manager/pull/7140), [`@cert-manager-bot`](https://github.com/cert-manager-bot))",
			domain.CategorySecurity, prov(heuristic("text:/CVE-/")), refs(domain.Reference{Type: "cve", ID: "CVE-2024-45338"})).
		values("chart", "cert-manager", "https://raw.githubusercontent.com/cert-manager/cert-manager/v1.15.1/deploy/charts/cert-manager/values.yaml",
			"installCRDs", "false",
			"crds.enabled", "false",
			"crds.keep", "true",
			"global.leaderElection.namespace", `"kube-system"`,
			"webhook.timeoutSeconds", "30",
			"startupapicheck.image.repository", `"quay.io/jetstack/cert-manager-startupapicheck"`,
			"prometheus.enabled", "true").
		crds("crds", "https://github.com/cert-manager/cert-manager/releases/download/v1.15.1/cert-manager.crds.yaml",
			crd("certificates.cert-manager.io", "cert-manager.io", "Certificate", crdVer("v1", true, true, append(append([]string{}, certPaths...), "spec.keystores.pkcs12.profile")...)),
			crd("issuers.cert-manager.io", "cert-manager.io", "Issuer", crdVer("v1", true, true, "spec", "spec.acme")),
		).
		compat("kubernetes", "supported", ">=1.25.0-0 <=1.30.x", "1.25 → 1.30", cmCompat).
		compat("openshift", "supported", ">=4.12.0-0 <=4.17.x", "4.12 → 4.17", cmCompat)
	return b.r
}

func cmInput(t testing.TB) Input {
	t.Helper()
	all := append(append([]string{}, series("v", 1, 14, 7)...), series("v", 1, 15, 5)...)
	sel, err := SelectPath(versions(all...), ver("v1.14.4"), ver("v1.15.1"), catalog.LineageMinor)
	if err != nil {
		t.Fatal(err)
	}
	fixed := advisoryEvidence("GHSA-synt-heti-c001", "https://github.com/cert-manager/cert-manager/security/advisories/GHSA-synt-heti-c001", "Webhook may log Secret data at verbosity >= 5")
	pem := advisoryEvidence("GHSA-r4pg-vg54-wxx4", "https://github.com/cert-manager/cert-manager/security/advisories/GHSA-r4pg-vg54-wxx4", "Potential DoS when parsing specially crafted PEM inputs")
	unparsed := advisoryEvidence("GHSA-synt-heti-c002", "https://github.com/cert-manager/cert-manager/security/advisories/GHSA-synt-heti-c002", "Advisory without affected range")
	to := cmTo()
	return Input{
		Definition: cmDefinition(),
		From:       cmFrom(),
		To:         to,
		Path:       []*domain.Release{cmV1150(), to},
		Selection:  sel,
		Advisories: []domain.Advisory{
			{ID: "GHSA-r4pg-vg54-wxx4", Aliases: []string{"CVE-2024-12401"}, Summary: "Potential DoS when parsing specially crafted PEM inputs", Severity: "moderate",
				URL: "https://github.com/cert-manager/cert-manager/security/advisories/GHSA-r4pg-vg54-wxx4", Vulnerable: "< 1.12.14 || >= 1.13.0, < 1.15.4 || >= 1.16.0, < 1.16.2",
				Patched: []string{"1.12.14", "1.15.4", "1.16.2"}, SourceID: "advisories", Evidence: []domain.EvidenceID{pem.ID}},
			{ID: "GHSA-synt-heti-c001", Aliases: []string{"CVE-2099-0001"}, Summary: "Webhook may log Secret data at verbosity >= 5", Severity: "high",
				URL: "https://github.com/cert-manager/cert-manager/security/advisories/GHSA-synt-heti-c001", Vulnerable: ">= 1.14.0, < 1.14.5",
				Patched: []string{"1.14.5"}, SourceID: "advisories", Evidence: []domain.EvidenceID{fixed.ID}},
			{ID: "GHSA-synt-heti-c002", Summary: "Advisory without affected range", URL: "https://github.com/cert-manager/cert-manager/security/advisories/GHSA-synt-heti-c002",
				SourceID: "advisories", Evidence: []domain.EvidenceID{unparsed.ID}},
			{ID: "GHSA-synt-heti-c003", Summary: "Only affects 1.12", Vulnerable: ">= 1.12.0, < 1.12.10", SourceID: "advisories"},
		},
		AdvisoryEvidence: []domain.Evidence{pem, fixed, unparsed},
		ExtraSources: []domain.SourceStatus{
			{SourceID: "tags", Kind: "git-tags", Roles: []domain.SourceRole{domain.RoleVersions}, State: domain.SourceOK},
			{SourceID: "advisories", Kind: "github-advisories", Roles: []domain.SourceRole{domain.RoleSecurity}, State: domain.SourceOK},
		},
		Now: fixedNow,
	}
}

// ---------------------------------------------------------------- Argo CD

const (
	argoUpgrade = "https://raw.githubusercontent.com/argoproj/argo-cd/v3.1.0/docs/operator-manual/upgrading/3.0-3.1.md"
	argoTested  = "https://raw.githubusercontent.com/argoproj/argo-cd/v3.1.0/docs/operator-manual/tested-kubernetes-versions.md"
	argoGitLog  = "https://github.com/argoproj/argo-cd/compare/v3.0.0...v3.1.0"
)

func argoDefinition() *catalog.ProductDefinition {
	return &catalog.ProductDefinition{
		APIVersion: catalog.APIVersion, Kind: catalog.Kind, ID: "argo-cd", Name: "Argo CD",
		Versioning: catalog.Versioning{Scheme: domain.SchemeSemver, TagPrefix: "v", Lineage: catalog.LineageMinor},
		Artifacts: []catalog.Artifact{
			{ID: "image", Type: domain.ArtifactContainerImage}, {ID: "install", Type: domain.ArtifactManifest},
			{ID: "install-ha", Type: domain.ArtifactManifest}, {ID: "chart", Type: domain.ArtifactHelmChart}, {ID: "cli", Type: domain.ArtifactBinary},
		},
	}
}

func argoRelease(tag, chart, dex, redis string, k8s []string) *rel {
	raw := "https://raw.githubusercontent.com/argoproj/argo-cd/" + tag + "/manifests/"
	b := newRel("argo-cd", tag)
	b.source("gh-releases", "github-releases", domain.SourceUnavailable, "api.github.com blocked", domain.RoleReleaseNotes).
		source("tested-k8s", "repo-file", domain.SourceOK, "", domain.RoleCompatibility).
		artifact("image", domain.ArtifactContainerImage, "argocd", "quay.io/argoproj/argocd:"+tag, domain.ArtifactVerified, "").
		artifact("install", domain.ArtifactManifest, "install.yaml", raw+"install.yaml", domain.ArtifactVerified, "").
		artifact("install-ha", domain.ArtifactManifest, "ha/install.yaml", raw+"ha/install.yaml", domain.ArtifactVerified, "").
		artifact("chart", domain.ArtifactHelmChart, "argo-cd", "ghcr.io/argoproj/argo-helm/argo-cd:"+chart, domain.ArtifactVerified, "").
		artifact("cli", domain.ArtifactBinary, "argocd-linux-amd64", "https://github.com/argoproj/argo-cd/releases/download/"+tag+"/argocd-linux-amd64", domain.ArtifactVerified, "").
		images("install", raw+"install.yaml", "quay.io/argoproj/argocd:"+tag, dex, redis).
		images("install-ha", raw+"ha/install.yaml", "quay.io/argoproj/argocd:"+tag, dex, redis, "public.ecr.aws/docker/library/haproxy:3.0.8-alpine").
		compat("kubernetes", "tested", "", strings.Join(k8s, ", "), argoTested, k8s...)
	return b
}

func argoInput(t testing.TB) Input {
	t.Helper()
	all := append(series("v", 3, 0, 6), "v3.1.0")
	sel, err := SelectPath(versions(all...), ver("v3.0.4"), ver("v3.1.0"), catalog.LineageMinor)
	if err != nil {
		t.Fatal(err)
	}
	from := argoRelease("v3.0.4", "8.0.10", "ghcr.io/dexidp/dex:v2.41.1", "redis:7.2.7-alpine", []string{"v1.32", "v1.31", "v1.30", "v1.29"})
	from.crds("install", "https://raw.githubusercontent.com/argoproj/argo-cd/v3.0.4/manifests/crds/application-crd.yaml",
		crd("applications.argoproj.io", "argoproj.io", "Application", crdVer("v1alpha1", true, true, "spec", "spec.source", "spec.source.repoURL", "spec.syncPolicy", "spec.syncPolicy.retry.refresh")))
	to := argoRelease("v3.1.0", "8.2.0", "ghcr.io/dexidp/dex:v2.43.0", "public.ecr.aws/docker/library/redis:7.2.7-alpine", []string{"v1.33", "v1.32", "v1.31", "v1.30"})
	to.source("git-log", "git-log", domain.SourceOK, "", domain.RoleReleaseNotes).
		source("upgrade", "repo-file", domain.SourceOK, "", domain.RoleUpgradeGuide).
		crds("install", "https://raw.githubusercontent.com/argoproj/argo-cd/v3.1.0/manifests/crds/application-crd.yaml",
			crd("applications.argoproj.io", "argoproj.io", "Application", crdVer("v1alpha1", true, true, "spec", "spec.source", "spec.source.repoURL", "spec.sourceHydrator", "spec.sourceHydrator.drySource", "spec.sourceHydrator.syncSource", "spec.syncPolicy"))).
		note("upgrade", domain.RoleUpgradeGuide, argoUpgrade, "Breaking Changes",
			"The `server.rbac.log.enforce.enable` setting was removed; logs RBAC is now always enforced. Grant the `logs, get` permission to roles that need to read pod logs before upgrading.",
			domain.CategoryConfiguration, breaking, action).
		note("upgrade", domain.RoleUpgradeGuide, argoUpgrade, "Helm Upgraded", "Helm was upgraded to 3.18.4.", domain.CategoryDependency).
		note("upgrade", domain.RoleUpgradeGuide, argoUpgrade, "Dex Upgraded", "Dex was upgraded to v2.43.0; review the Dex release notes if you rely on custom connectors.", domain.CategoryDependency).
		note("git-log", domain.RoleReleaseNotes, argoGitLog, "Commits", "feat: source hydrator can push to a separate hydrated branch (#22000)", domain.CategoryFeature, prov(heuristic("text:/^feat/"))).
		note("git-log", domain.RoleReleaseNotes, argoGitLog, "Commits", "feat(appset): add `ignoreApplicationDifferences` to the matrix generator (#22011)", domain.CategoryFeature, prov(heuristic("text:/^feat/"))).
		note("git-log", domain.RoleReleaseNotes, argoGitLog, "Commits", "fix: application controller leaks goroutines on cluster removal (#22100)", domain.CategoryBugfix, prov(heuristic("text:/^fix/"))).
		note("git-log", domain.RoleReleaseNotes, argoGitLog, "Commits", "fix(ui): resource tree does not refresh after sync (#22111)", domain.CategoryBugfix, prov(heuristic("text:/^fix/"))).
		note("git-log", domain.RoleReleaseNotes, argoGitLog, "Commits", "chore: deprecate the `--redis-compress` flag in favour of `redis.compression` (#22200)", domain.CategoryDeprecation, prov(heuristic("text:/deprecat/"))).
		note("git-log", domain.RoleReleaseNotes, argoGitLog, "Commits", "chore(deps): bump github.com/go-git/go-git/v5 from 5.13.2 to 5.16.0 (#22300)", domain.CategoryOther, prov(heuristic("default")))
	return Input{
		Definition: argoDefinition(),
		From:       from.r,
		To:         to.r,
		Path:       []*domain.Release{to.r},
		Selection:  sel,
		ExtraSources: []domain.SourceStatus{
			{SourceID: "tags", Kind: "git-tags", Roles: []domain.SourceRole{domain.RoleVersions}, State: domain.SourceOK},
			{SourceID: "advisories", Kind: "github-advisories", Roles: []domain.SourceRole{domain.RoleSecurity}, State: domain.SourceUnavailable, Detail: "api.github.com blocked"},
		},
		Now: fixedNow,
	}
}

func mustBuild(t testing.TB, in Input) *domain.UpgradeEdge {
	t.Helper()
	e, err := Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return e
}

func findChanges(e *domain.UpgradeEdge, rule string) []domain.Change {
	return e.ChangesWhere(func(c domain.Change) bool { return c.Provenance.Rule == rule })
}

func findTitle(e *domain.UpgradeEdge, substr string) *domain.Change {
	for i := range e.Changes {
		if strings.Contains(e.Changes[i].Title, substr) {
			return &e.Changes[i]
		}
	}
	return nil
}

func hasWarning(e *domain.UpgradeEdge, substr string) bool {
	for _, w := range e.Warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
