package semvalidate

import (
	"encoding/json"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func certCRDs(r *rel, v1 domain.CRDVersionInfo, others ...domain.CRDVersionInfo) *rel {
	return r.crds("crds",
		crd("certificates.cert-manager.io", "cert-manager.io", "Certificate", append([]domain.CRDVersionInfo{v1}, others...)...),
		crd("issuers.cert-manager.io", "cert-manager.io", "Issuer", crdVer("v1", true, true, fld("spec.ca", "object", ""), fld("spec.rotationPolicy", "string", `"Never"`))))
}

func TestCRDFieldValidator(t *testing.T) {
	req := func(path string) domain.CRDFieldSchema {
		f := fld(path, "string", "")
		f.Required = true
		return f
	}
	enum := func(path string, vals ...string) domain.CRDFieldSchema {
		f := fld(path, "string", "")
		f.Enum = vals
		return f
	}
	from := certCRDs(newRel("v1.16.0"), crdVer("v1", true, true,
		fld("spec.privateKey.rotationPolicy", "string", `"Never"`),
		fld("spec.old", "string", ""),
		fld("spec.nodefault", "string", ""),
		fld("spec.dnsNames[]", "array", ""),
		fld("spec.optional", "string", ""),
		req("spec.alreadyRequired"),
		enum("spec.mode", `"A"`, `"B"`),
		enum("spec.kind", `"x"`, `"y"`)))
	to := certCRDs(newRel("v1.17.0"), crdVer("v1", true, true,
		fld("spec.privateKey.rotationPolicy", "string", `"Always"`),
		fld("spec.new", "string", ""),
		fld("spec.nodefault", "string", ""),
		fld("spec.dnsNames[]", "array", ""),
		req("spec.optional"),
		req("spec.alreadyRequired"),
		enum("spec.mode", `"A"`),
		enum("spec.kind", `"x"`, `"z"`)))
	O, R, I := domain.OutcomeConfirmed, domain.OutcomeRefuted, domain.OutcomeInconclusive
	cases := []struct {
		name           string
		group, kind    string
		path           string
		chg            *domain.ChangeSpec
		subject, chOut domain.ValidationOutcome
	}{
		{"default changed", "cert-manager.io", "Certificate", "spec.privateKey.rotationPolicy", change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`), O, O},
		{"wrong default", "cert-manager.io", "Certificate", "spec.privateKey.rotationPolicy", change(domain.ChangeKindDefaultChanged, `"Always"`, `"Never"`), O, R},
		{"wrong after default", "cert-manager.io", "Certificate", "spec.privateKey.rotationPolicy", change(domain.ChangeKindDefaultChanged, `"Never"`, `"Rotate"`), O, R},
		{"schema states no default", "cert-manager.io", "Certificate", "spec.nodefault", change(domain.ChangeKindDefaultChanged, `"a"`, `"b"`), O, I},
		{"field removed", "cert-manager.io", "Certificate", "spec.old", change(domain.ChangeKindRemoved, "", ""), O, O},
		{"field added", "cert-manager.io", "Certificate", "spec.new", change(domain.ChangeKindAdded, "", ""), O, O},
		{"removed field still there", "cert-manager.io", "Certificate", "spec.nodefault", change(domain.ChangeKindRemoved, "", ""), O, R},
		{"list marker is optional", "cert-manager.io", "Certificate", "spec.dnsNames", change(domain.ChangeKindDefaultChanged, `[]`, `["a"]`), O, I},
		{"now required", "cert-manager.io", "Certificate", "spec.optional", change(domain.ChangeKindNowRequired, "", ""), O, O},
		{"was already required", "cert-manager.io", "Certificate", "spec.alreadyRequired", change(domain.ChangeKindNowRequired, "", ""), O, R},
		{"not required at all", "cert-manager.io", "Certificate", "spec.nodefault", change(domain.ChangeKindNowRequired, "", ""), O, R},
		{"enum value lost = tightened", "cert-manager.io", "Certificate", "spec.mode", change(domain.ChangeKindValidationTightened, "", ""), O, O},
		{"nothing tightened", "cert-manager.io", "Certificate", "spec.nodefault", change(domain.ChangeKindValidationTightened, "", ""), O, I},
		{"enum value renamed", "cert-manager.io", "Certificate", "spec.kind", change(domain.ChangeKindValueChanged, `"y"`, `"z"`), O, O},
		{"enum rename of a value that was never allowed", "cert-manager.io", "Certificate", "spec.kind", change(domain.ChangeKindValueChanged, `"q"`, `"z"`), O, R},
		// the adversarial cases
		{"plausible but nonexistent path", "cert-manager.io", "Certificate", "spec.privateKey.rotationPolicyMode", change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`), R, R},
		{"same path, different kind", "cert-manager.io", "Issuer", "spec.privateKey.rotationPolicy", change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`), R, R},
		{"path of another kind's schema", "cert-manager.io", "Certificate", "spec.ca", change(domain.ChangeKindAdded, "", ""), R, R},
		// a CRD snapshot is not known to hold every kind of a group (cilium generates
		// some CRDs at runtime): a kind it lacks is a gap, never a refutation
		{"kind missing from the snapshot", "cert-manager.io", "Widget", "spec.a", change(domain.ChangeKindAdded, "", ""), I, I},
		{"group outside the snapshots", "other.io", "Certificate", "spec.a", change(domain.ChangeKindAdded, "", ""), I, I},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, crdValidator{}, input(from, to, assertion(crdSubject(tc.group, tc.kind, tc.path), tc.chg)))
			if got[domain.AspectSubject] != tc.subject || got[domain.AspectChange] != tc.chOut {
				t.Errorf("subject/change = %s/%s, want %s/%s", got[domain.AspectSubject], got[domain.AspectChange], tc.subject, tc.chOut)
			}
		})
	}
}

func TestCRDFieldSnapshotsWithoutFieldFactsAreInconclusive(t *testing.T) {
	// snapshots captured before the capture lane carry paths but no Fields
	legacy := func(tag string, paths ...string) *rel {
		v := domain.CRDVersionInfo{Name: "v1", Served: true, Storage: true, SchemaPaths: paths}
		return newRel(tag).crds("crds", crd("a.x.io", "x.io", "A", v))
	}
	got := run(t, crdValidator{}, input(legacy("v1.0.0", "spec.a"), legacy("v1.1.0", "spec.a"),
		assertion(crdSubject("x.io", "A", "spec.a"), change(domain.ChangeKindDefaultChanged, `"a"`, `"b"`))))
	if got[domain.AspectSubject] != domain.OutcomeConfirmed || got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("legacy snapshot: %v", got)
	}
	// but removal needs only the paths
	got = run(t, crdValidator{}, input(legacy("v1.0.0", "spec.a", "spec.b"), legacy("v1.1.0", "spec.a"),
		assertion(crdSubject("x.io", "A", "spec.b"), change(domain.ChangeKindRemoved, "", ""))))
	if got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("removal from path lists: %v", got)
	}
}

func TestCRDFieldVersionSelection(t *testing.T) {
	// the field is removed from v1beta1 only; v1 keeps it
	from := newRel("v1.0.0").crds("crds", crd("a.x.io", "x.io", "A",
		crdVer("v1", true, true, fld("spec.a", "string", "")), crdVer("v1beta1", true, false, fld("spec.a", "string", ""))))
	to := newRel("v1.1.0").crds("crds", crd("a.x.io", "x.io", "A",
		crdVer("v1", true, true, fld("spec.a", "string", "")), crdVer("v1beta1", true, false)))
	s := crdSubject("x.io", "A", "spec.a")
	s.Version = "v1"
	if got := run(t, crdValidator{}, input(from, to, assertion(s, change(domain.ChangeKindRemoved, "", "")))); got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("v1 keeps the field: %v", got)
	}
	s.Version = "v1beta1"
	if got := run(t, crdValidator{}, input(from, to, assertion(s, change(domain.ChangeKindRemoved, "", "")))); got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("v1beta1 loses the field: %v", got)
	}
}

func TestGVKValidator(t *testing.T) {
	from := newRel("v0.37.0").crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool",
		crdVer("v1beta1", true, true), crdVer("v1alpha5", true, false)))
	to := newRel("v1.0.0").crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool",
		crdVer("v1", true, true), crdVer("v1beta1", false, false)))
	gvk := func(v string) *domain.Subject {
		return &domain.Subject{Family: domain.SubjectGVK, Product: "karpenter", Group: "karpenter.sh", Version: v, Kind: "NodePool"}
	}
	O, R, I := domain.OutcomeConfirmed, domain.OutcomeRefuted, domain.OutcomeInconclusive
	cases := []struct {
		name            string
		subj            *domain.Subject
		chg             *domain.ChangeSpec
		subject, change domain.ValidationOutcome
	}{
		{"version removed", gvk("v1alpha5"), change(domain.ChangeKindRemoved, "", ""), O, O},
		{"unserved counts as removed", gvk("v1beta1"), change(domain.ChangeKindRemoved, "", ""), O, O},
		{"added but it already existed", gvk("v1beta1"), change(domain.ChangeKindAdded, "", ""), O, R},
		{"removed but new", gvk("v1"), change(domain.ChangeKindRemoved, "", ""), O, R},
		{"version added", gvk("v1"), change(domain.ChangeKindAdded, "", ""), O, O},
		{"storage moved", gvk("v1"), change(domain.ChangeKindValueChanged, `"v1beta1"`, `"v1"`), O, O},
		{"storage not moved like that", gvk("v1"), change(domain.ChangeKindValueChanged, `"v1alpha5"`, `"v1"`), O, R},
		{"version that never existed", gvk("v2"), change(domain.ChangeKindRemoved, "", ""), R, R},
		{"behavior", gvk("v1"), change(domain.ChangeKindBehaviorChanged, "", ""), O, I},
		{"unknown group is not the product's", &domain.Subject{Family: domain.SubjectGVK, Product: "karpenter", Group: "elsewhere.io", Version: "v1", Kind: "NodePool"}, change(domain.ChangeKindRemoved, "", ""), I, I},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, crdValidator{}, input(from, to, assertion(tc.subj, tc.chg)))
			if got[domain.AspectSubject] != tc.subject || got[domain.AspectChange] != tc.change {
				t.Errorf("subject/change = %s/%s, want %s/%s", got[domain.AspectSubject], got[domain.AspectChange], tc.subject, tc.change)
			}
		})
	}
	dep := newRel("v1.1.0").crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool", func() domain.CRDVersionInfo {
		v := crdVer("v1", true, true)
		v.Deprecated = true
		return v
	}()))
	plain := newRel("v1.0.0").crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool", crdVer("v1", true, true)))
	if got := run(t, crdValidator{}, input(plain, dep, assertion(gvk("v1"), change(domain.ChangeKindDeprecated, "", "")))); got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("deprecation: %v", got)
	}
}

// Audit regressions (docs/phase3/learning-loop/VALIDATOR-AUDIT.md).

func TestCRDDefaultEncodedAsStringAndRemovedDefault(t *testing.T) {
	obj := func(hop string) string { return `{"httpEndpoint":"enabled","httpPutResponseHopLimit":` + hop + `}` }
	from := newRel("v0.37.8").crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool",
		crdVer("v1", true, true, fld("spec.metadataOptions", "object", obj("2")), fld("spec.disruption", "object", `{"consolidateAfter":"0s"}`))))
	to := newRel("v1.0.0").crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool",
		crdVer("v1", true, true, fld("spec.metadataOptions", "object", obj("1")), fld("spec.disruption", "object", ""))))
	enc := func(s string) string { b, _ := json.Marshal(s); return string(b) } // the object wrapped as a JSON string
	sub := func(p string) *domain.Subject { return crdSubject("karpenter.sh", "NodePool", p) }
	// a model that sends an object default as a JSON string is understood
	got := run(t, crdValidator{}, input(from, to, assertion(sub("spec.metadataOptions"), change(domain.ChangeKindDefaultChanged, enc(obj("2")), enc(obj("1"))))))
	if got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("string-encoded object default: %v", got)
	}
	// ...but a wrong value inside is still refuted
	got = run(t, crdValidator{}, input(from, to, assertion(sub("spec.metadataOptions"), change(domain.ChangeKindDefaultChanged, enc(obj("3")), enc(obj("1"))))))
	if got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("wrong encoded default: %v", got)
	}
	// default → null: the target schema no longer states one
	got = run(t, crdValidator{}, input(from, to, assertion(sub("spec.disruption"), change(domain.ChangeKindDefaultChanged, enc(`{"consolidateAfter":"0s"}`), "null"))))
	if got[domain.AspectChange] != domain.OutcomeConfirmed {
		t.Errorf("removed schema default: %v", got)
	}
	// null → default is not provable (the source may have had a code default)
	got = run(t, crdValidator{}, input(to, from, assertion(sub("spec.disruption"), change(domain.ChangeKindDefaultChanged, "null", enc(`{"consolidateAfter":"0s"}`)))))
	if got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("default introduced: %v", got)
	}
	// a child property whose own default did not change
	both := newRel("v1.0.0").crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool",
		crdVer("v1", true, true, fld("spec.metadataOptions.httpPutResponseHopLimit", "integer", "2"))))
	got = run(t, crdValidator{}, input(both, both, assertion(sub("spec.metadataOptions.httpPutResponseHopLimit"), change(domain.ChangeKindDefaultChanged, "2", "1"))))
	if got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("child default unchanged: %v", got)
	}
}

// v1beta1 → v1 moves are real but happen BETWEEN versions: neither schema's
// enum or field set changes, so a per-schema refutation would be wrong and a
// confirmation impossible.
func TestCRDCrossVersionMovesAreInconclusive(t *testing.T) {
	enum := func(path string, vals ...string) domain.CRDFieldSchema {
		f := fld(path, "string", "")
		f.Enum = vals
		return f
	}
	mk := func(tag string) *rel {
		return newRel(tag).crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool",
			crdVer("v1", true, true, enum("spec.disruption.consolidationPolicy", `"WhenEmpty"`, `"WhenEmptyOrUnderutilized"`), fld("spec.template.spec.expireAfter", "string", "")),
			crdVer("v1beta1", true, false, enum("spec.disruption.consolidationPolicy", `"WhenEmpty"`, `"WhenUnderutilized"`), fld("spec.disruption.expireAfter", "string", ""))))
	}
	from, to := mk("v0.37.8"), mk("v1.0.0")
	enumRename := assertion(crdSubject("karpenter.sh", "NodePool", "spec.disruption.consolidationPolicy"),
		change(domain.ChangeKindValueChanged, `"WhenUnderutilized"`, `"WhenEmptyOrUnderutilized"`))
	if got := run(t, crdValidator{}, input(from, to, enumRename)); got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("enum renamed between versions: %v", got)
	}
	c := change(domain.ChangeKindRenamed, "", "")
	c.ReplacedBy = crdSubject("karpenter.sh", "NodePool", "spec.template.spec.expireAfter")
	if got := run(t, crdValidator{}, input(from, to, assertion(crdSubject("karpenter.sh", "NodePool", "spec.disruption.expireAfter"), c))); got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("field moved between versions: %v", got)
	}
	// a value that was never allowed anywhere is still refuted
	bogus := assertion(crdSubject("karpenter.sh", "NodePool", "spec.disruption.consolidationPolicy"), change(domain.ChangeKindValueChanged, `"Nope"`, `"WhenEmptyOrUnderutilized"`))
	if got := run(t, crdValidator{}, input(from, to, bogus)); got[domain.AspectChange] != domain.OutcomeRefuted {
		t.Errorf("never-allowed value: %v", got)
	}
}

func TestCRDUnversionedClaimsNeedEveryVersionToAgree(t *testing.T) {
	from := newRel("v1.0.0").crds("crds", crd("a.x.io", "x.io", "A",
		crdVer("v1", true, true, fld("spec.a", "string", "")), crdVer("v1beta1", true, false, fld("spec.a", "string", ""))))
	to := newRel("v1.1.0").crds("crds", crd("a.x.io", "x.io", "A",
		crdVer("v1", true, true, fld("spec.a", "string", "")), crdVer("v1beta1", true, false)))
	// dropped from v1beta1 only: "removed" without a version would mint a wrong fact
	got := run(t, crdValidator{}, input(from, to, assertion(crdSubject("x.io", "A", "spec.a"), change(domain.ChangeKindRemoved, "", ""))))
	if got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("removed from one version of two: %v", got)
	}
	// a brand-new API version brings every field with it: that is not "field added"
	withV2 := newRel("v1.1.0").crds("crds", crd("a.x.io", "x.io", "A",
		crdVer("v1", true, true, fld("spec.a", "string", "")), crdVer("v2", true, false, fld("spec.a", "string", ""))))
	s := crdSubject("x.io", "A", "spec.a")
	s.Version = "v2"
	got = run(t, crdValidator{}, input(from, withV2, assertion(s, change(domain.ChangeKindAdded, "", ""))))
	if got[domain.AspectChange] != domain.OutcomeInconclusive {
		t.Errorf("field of a new version: %v", got)
	}
}
