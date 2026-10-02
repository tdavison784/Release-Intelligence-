package semvalidate

import (
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
		{"kind that does not exist in the group", "cert-manager.io", "Widget", "spec.a", change(domain.ChangeKindAdded, "", ""), R, R},
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
		{"unknown group is not the product's", &domain.Subject{Family: domain.SubjectGVK, Product: "karpenter", Group: "elsewhere.io", Version: "v1", Kind: "NodePool"}, change(domain.ChangeKindRemoved, "", ""), I, R},
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
