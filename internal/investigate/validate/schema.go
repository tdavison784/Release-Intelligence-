package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// SchemaCompat checks schema and API compatibility (MISSION §A5):
//
//   - compatibility items: the environment's cluster version against the
//     target release's constraint for the platform (Kubernetes) — inside →
//     pass, outside a supported/minimum/chart-kubeVersion range → fail
//     (outside a merely "tested" range is not a demonstrated
//     incompatibility: not-run);
//   - api-schema items anchored on a computed CRD change: every visible
//     object of the CRD's kind — environment files and the environment
//     renders (overlays and chart-rendered objects are only visible there) —
//     against the change: a removed kind or API version still used, a removed
//     field set, a removed enum value used, a newly required field omitted, a
//     value of the old type → fail; all visible objects compatible, with
//     complete inputs → pass for those objects. Default, storage and additive
//     changes do not make objects invalid (other validators decide them).
//
// No object visible is never a pass (absence in a repository is not proof
// of absence).
func SchemaCompat(opts ...Option) investigate.Validator {
	return &schemaCompat{base{name: NameSchemaCompat, kind: domain.ValidationSchemaCompat,
		families: []domain.ChangeItemFamily{domain.FamilyAPISchema, domain.FamilyCompatibility}, cfg: newConfig(opts)}}
}

type schemaCompat struct{ base }

func (v *schemaCompat) Validate(ctx context.Context, in investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	item, err := prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	cov, err := coverageOf(in, item)
	if err != nil {
		return nil, err
	}
	out := newOutput(v.base, in)
	edge, err := readEdge(ctx, v.cfg, in.Bundle)
	if err != nil {
		out.notRun(cov, "schema:no-edge", "The change is compatible with the environment's schema and platform",
			"the upgrade edge (change record source) cannot be read: "+err.Error())
		return out.result()
	}
	if item.Platform != "" || item.Family == domain.FamilyCompatibility {
		v.platform(in, item, cov, edge, out)
	} else {
		if err := v.crd(ctx, in, item, cov, edge, out); err != nil {
			return nil, err
		}
	}
	return out.result()
}

// --- platform version ---------------------------------------------------------

func (v *schemaCompat) platform(in investigate.ValidationInput, item domain.ChangeItem, cov coverage, edge *domain.UpgradeEdge, out *output) {
	b := in.Bundle
	// the bundle anchors "<platform>" or, when several constraints of one
	// platform moved, "<platform>/<kind>"
	platform, wantKind, _ := strings.Cut(item.Platform, "/")
	if platform == "" {
		for _, s := range item.Subjects {
			if p, _, ok := strings.Cut(s, "/"); ok {
				platform = p
				break
			}
		}
	}
	property := fmt.Sprintf("The environment's %s version is inside the target release's supported range", upgrade.PlatformName(platform))
	var cc *domain.CompatibilityChange
	for i := range edge.Compatibility {
		c := &edge.Compatibility[i]
		if !strings.EqualFold(c.Platform, platform) {
			continue
		}
		if wantKind != "" && !(c.To != nil && c.To.Kind == wantKind) && !(c.From != nil && c.From.Kind == wantKind) {
			continue
		}
		cc = c
	}
	switch {
	case platform == "":
		out.notRun(cov, "schema:platform-unknown", property, "the item names no platform")
		return
	case cc == nil:
		out.notRun(cov, "schema:no-constraint", property, fmt.Sprintf("the upgrade edge has no %s compatibility entry", platform))
		return
	case cc.To == nil:
		out.notRun(cov, "schema:no-constraint", property, fmt.Sprintf("the target release states no %s constraint (%s)", platform, cc.Summary))
		return
	case !strings.EqualFold(platform, "kubernetes") && !strings.EqualFold(platform, "k8s"):
		out.notRun(cov, "schema:platform-unsupported", property, fmt.Sprintf("the environment snapshot carries no %s version (only the Kubernetes version is collected)", platform))
		return
	}
	version := b.Snapshot.KubernetesVersion
	for _, in := range b.Snapshot.Inputs {
		if version == "" && in.Kind == domain.InputClusterVersion && in.Status == domain.CollectionOK {
			version = in.Value
		}
	}
	if version == "" {
		out.notRun(cov, "schema:no-cluster-version", property, "the cluster version was not supplied (--kubernetes)")
		return
	}
	chk := upgrade.EvaluatePlatformConstraint(cc.To, version)
	if !chk.Computable {
		out.notRun(cov, "schema:not-computable", property, fmt.Sprintf("the target constraint %q cannot be evaluated against %s", cc.To.Raw, version))
		return
	}
	d := draft{rule: "schema:platform-range", property: property, subjects: cov.subjects, paths: cov.paths,
		inputs: []string{b.Change.Edge, b.Snapshot.Facts}}
	in0 := domain.NewEvidence(domain.EvidenceInput, "", "flag:--kubernetes", "", version, domain.Digest([]byte("--kubernetes="+version)), timeZero)
	d.evidence = append(d.evidence, out.cite(in0))
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range edge.Evidence {
		pool[e.ID] = e
	}
	for _, id := range cc.To.Evidence {
		if e, ok := pool[id]; ok {
			d.evidence = append(d.evidence, out.cite(e))
		}
	}
	kind := cc.To.Kind
	if kind == "" {
		kind = "supported"
	}
	switch {
	case chk.Admits:
		d.outcome = domain.ValidationPass
		d.detail = fmt.Sprintf("cluster %s %s is inside the target's %s range %s", platform, version, kind, chk.Display)
	case kind == "tested":
		d.outcome = domain.ValidationNotRun
		d.detail = fmt.Sprintf("cluster %s %s is outside the target's tested range %s — untested is not a demonstrated incompatibility", platform, version, chk.Display)
		d.inputs, d.evidence = nil, nil
	default:
		d.outcome = domain.ValidationFail
		d.detail = fmt.Sprintf("cluster %s %s is outside the target's %s range %s", platform, version, kind, chk.Display)
		if chk.Below != "" {
			d.detail += "; the oldest admitted line is " + chk.Below
		}
		if chk.Above != "" {
			d.detail += "; the newest admitted line is " + chk.Above
		}
	}
	out.add(d)
}

// --- CRD objects --------------------------------------------------------------

type crdAspect string

const (
	aspectKind     crdAspect = "kind"     // crd:removed
	aspectVersion  crdAspect = "version"  // crd:version-removed / -unserved
	aspectFields   crdAspect = "fields"   // crd:fields-removed
	aspectEnum     crdAspect = "enum"     // crd:enum-changed
	aspectRequired crdAspect = "required" // crd:field-required
	aspectType     crdAspect = "type"     // crd:field-type-changed
)

var (
	enumDetail = regexp.MustCompile(`(?m)^(\S+): removed \[([^\]]*)\]`)
	typeDetail = regexp.MustCompile(`(?m)^(\S+): (\S+) → (\S+)$`)
)

func (v *schemaCompat) crd(ctx context.Context, in investigate.ValidationInput, item domain.ChangeItem, cov coverage, edge *domain.UpgradeEdge, out *output) error {
	property := fmt.Sprintf("The environment's objects stay valid under the target's API schema (%s)", item.Title)
	var change *domain.Change
	for i := range edge.Changes {
		if edge.Changes[i].ID == item.ChangeID {
			change = &edge.Changes[i]
		}
	}
	if change == nil {
		out.notRun(cov, "schema:no-change", property, "the item is not anchored on a change of the upgrade edge")
		return nil
	}
	var aspect crdAspect
	switch change.Provenance.Rule {
	case upgrade.RuleCRDRemoved:
		aspect = aspectKind
	case upgrade.RuleCRDVersionRemoved, upgrade.RuleCRDVersionUnserved:
		aspect = aspectVersion
	case upgrade.RuleCRDFieldsRemoved:
		aspect = aspectFields
	case upgrade.RuleCRDEnumChanged:
		aspect = aspectEnum
	case upgrade.RuleCRDFieldRequired:
		aspect = aspectRequired
	case upgrade.RuleCRDFieldTypeChange:
		aspect = aspectType
	default:
		out.notRun(cov, "schema:no-object-rule", property, fmt.Sprintf(
			"the change (%s) does not make existing objects invalid by itself — a default change is decided by override inspection, a storage move or an addition by other evidence; no schema rule applies",
			orAny(change.Provenance.Rule)))
		return nil
	}
	name, group, version, kind, ok := impact.CRDIdentity(*change)
	if !ok || (kind == "" && aspect != aspectKind) {
		out.notRun(cov, "schema:no-identity", property, "the change does not identify the CRD kind its schema paths belong to")
		return nil
	}
	if kind == "" {
		kind = kindOfCRD(ctx, v.cfg, in.Bundle, name)
	}
	if kind == "" {
		out.notRun(cov, "schema:no-identity", property, fmt.Sprintf("the kind of CRD %s is not stated by the change and no render defines it", name))
		return nil
	}
	view, err := loadView(ctx, v.cfg, in.Bundle)
	if err != nil {
		return err
	}
	var enums map[string][]string
	var types map[string]string
	switch aspect {
	case aspectEnum:
		enums = map[string][]string{}
		for _, m := range enumDetail.FindAllStringSubmatch(change.Detail, -1) {
			for _, x := range strings.Split(m[2], ",") {
				if x = strings.TrimSpace(x); x != "" {
					enums[m[1]] = append(enums[m[1]], x)
				}
			}
		}
	case aspectType:
		types = map[string]string{}
		for _, m := range typeDetail.FindAllStringSubmatch(change.Detail, -1) {
			types[m[1]] = m[3]
		}
	}

	type objRef struct {
		src      objSource
		obj      render.Object
		rendered bool
	}
	// the objects as deployed (the effective view): a file a render
	// consumed is judged only through the render's objects, never on its
	// own; one whose render is unavailable is not judged at all
	var objs []objRef
	var unrendered []string
	for _, e := range view.objects() {
		if e.obj.ID.Kind != kind || e.obj.ID.Group != group {
			continue
		}
		if e.unrendered != "" {
			unrendered = append(unrendered, fmt.Sprintf("%s (%s): %s", e.obj.ID, e.src.artifact.Path, e.unrendered))
			continue
		}
		objs = append(objs, objRef{e.src, e.obj, e.rendered})
	}
	var violations, checked, withheld []string
	var vEvidence, cEvidence []domain.Evidence
	var vObjects, cObjects []string
	for _, o := range objs {
		ev := func(path, excerpt string) domain.Evidence {
			if o.rendered {
				return artifactEvidence(domain.EvidenceRendered, o.src.artifact, o.obj.ID.String()+" "+path, excerpt, envRender(o.obj.ID.String(), path, ""))
			}
			return artifactEvidence(domain.EvidenceLocalFile, o.src.artifact, o.obj.ID.String()+" "+path, excerpt, nil)
		}
		var bad, hidden []string
		switch aspect {
		case aspectKind:
			bad = append(bad, fmt.Sprintf("is a %s, whose CRD %s the target no longer ships", kind, name))
		case aspectVersion:
			if o.obj.ID.Version == version {
				bad = append(bad, fmt.Sprintf("uses %s/%s, which the target no longer serves", group, version))
			}
		case aspectFields:
			for _, p := range change.Subjects {
				if vals, _ := lookup(o.obj.Body, p); len(vals) > 0 {
					bad = append(bad, fmt.Sprintf("sets %s, which the target schema prunes", p))
				}
			}
		case aspectEnum:
			for p, removed := range enums {
				vals, _ := lookup(o.obj.Body, p)
				for _, val := range vals {
					if redactedValue(val) {
						hidden = append(hidden, p)
						continue
					}
					for _, r := range removed {
						if sameValue(val, r) {
							bad = append(bad, fmt.Sprintf("sets %s to %s, a value the target no longer allows", p, val))
						}
					}
				}
			}
		case aspectRequired:
			for _, p := range change.Subjects {
				parent := parentOf(p)
				if parent != "" {
					if pv, pm := lookup(o.obj.Body, parent); pm > 0 || len(pv) == 0 {
						continue // the parent is absent: the requirement does not apply
					}
				}
				if _, missing := lookup(o.obj.Body, p); missing > 0 {
					bad = append(bad, fmt.Sprintf("omits %s, which the target makes required", p))
				}
			}
		case aspectType:
			for p, want := range types {
				vals, _ := lookup(o.obj.Body, p)
				for _, val := range vals {
					if redactedValue(val) {
						hidden = append(hidden, p)
						continue
					}
					if got := jsonKind(val); got != "" && !typeFits(got, want) {
						bad = append(bad, fmt.Sprintf("sets %s to a %s, but the target schema wants %s", p, got, want))
					}
				}
			}
		}
		where := fmt.Sprintf("%s (%s)", o.obj.ID, o.src.artifact.Path)
		switch {
		case len(bad) > 0:
			violations = append(violations, where+" "+strings.Join(bad, "; "))
			vEvidence = append(vEvidence, ev("", strings.Join(bad, "; ")))
			vObjects = append(vObjects, o.obj.ID.String())
		case len(hidden) > 0:
			// a withheld value may be the very value the target rejects
			withheld = append(withheld, where+" "+strings.Join(hidden, ", "))
		default:
			checked = append(checked, where)
			cEvidence = append(cEvidence, ev("", "compatible with "+string(change.Provenance.Rule)))
			cObjects = append(cObjects, o.obj.ID.String())
		}
	}
	inputs := append([]string{in.Bundle.Change.Edge}, view.inputs(true)...)
	if len(violations) > 0 {
		d := draft{rule: "schema:crd-" + string(aspect), outcome: domain.ValidationFail, property: property, inputs: inputs}
		d.subjects, d.paths = exposedBy(in, vObjects)
		for _, e := range vEvidence {
			d.evidence = append(d.evidence, out.cite(e))
		}
		d.evidence = append(d.evidence, change.Evidence...)
		d.detail = fmt.Sprintf("objects the target's %s schema of %s rejects or prunes (%s):\n- %s", group, kind, change.Provenance.Rule, strings.Join(violations, "\n- "))
		d.evidence = keepKnown(out, edge, d.evidence)
		out.add(d)
		return nil
	}
	switch {
	case len(unrendered) > 0:
		out.notRun(cov, "schema:unrendered", property, fmt.Sprintf("the deployed form of %d %s object(s) is unknown, so their compatibility is unknown: %s",
			len(unrendered), kind, strings.Join(unrendered, "; ")))
	case len(withheld) > 0:
		out.notRun(cov, "schema:withheld", property, fmt.Sprintf("the value the change is about is withheld (redacted) in %d %s object(s), so their compatibility is unknown: %s",
			len(withheld), kind, strings.Join(withheld, "; ")))
	case len(checked) == 0:
		out.notRun(cov, "schema:no-objects", property, fmt.Sprintf(
			"no %s/%s %s object is visible in the environment files or renders; absence in the repository is not proof of absence (objects can be created by controllers or out of band)",
			group, orAny(version), kind))
	case !view.complete || len(view.gaps) > 0 || len(view.mentionsKind(kind)) > 0:
		var why []string
		if !view.complete {
			why = append(why, incompleteDetail(view.pairs))
		}
		why = append(why, view.gaps...)
		if files := view.mentionsKind(kind); len(files) > 0 {
			why = append(why, "unparsed environment files may declare "+kind+" objects: "+strings.Join(files, ", "))
		}
		out.notRun(cov, "schema:incomplete", property, fmt.Sprintf("the %d visible %s objects are compatible, but not every object is visible: %s", len(checked), kind, strings.Join(why, "; ")))
	default:
		d := draft{rule: "schema:crd-" + string(aspect), outcome: domain.ValidationPass, property: property, inputs: inputs}
		d.subjects, d.paths = exposedBy(in, cObjects)
		for _, e := range cEvidence {
			d.evidence = append(d.evidence, out.cite(e))
		}
		d.evidence = keepKnown(out, edge, append(d.evidence, change.Evidence...))
		d.detail = fmt.Sprintf("every visible %s object is compatible with the target schema change (%s):\n- %s", kind, change.Provenance.Rule, strings.Join(checked, "\n- "))
		out.add(d)
	}
	return nil
}

// keepKnown keeps evidence ids that resolve, citing edge records the bundle
// pool lacks.
func keepKnown(out *output, edge *domain.UpgradeEdge, ids []domain.EvidenceID) []domain.EvidenceID {
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range edge.Evidence {
		pool[e.ID] = e
	}
	var keep []domain.EvidenceID
	for _, id := range ids {
		switch {
		case out.known(id):
			keep = append(keep, id)
		case pool[id].ID != "":
			keep = append(keep, out.cite(pool[id]))
		}
	}
	return keep
}

// kindOfCRD finds the kind a CRD defines in the environment or release
// renders.
func kindOfCRD(ctx context.Context, c config, b *domain.EvidenceBundle, name string) string {
	for _, ps := range [][]domain.ManifestPair{b.Renders, b.Change.Renders} {
		for _, p := range ps {
			for _, id := range []string{p.From, p.To} {
				if id == "" {
					continue
				}
				data, _, err := c.read(ctx, b, id)
				if err != nil {
					continue
				}
				objs, err := render.ParseObjects(data)
				if err != nil {
					continue
				}
				for _, o := range objs {
					if o.ID.Kind == "CustomResourceDefinition" && o.ID.Name == name {
						if spec, ok := o.Body["spec"].(map[string]any); ok {
							if names, ok := spec["names"].(map[string]any); ok {
								if k, ok := names["kind"].(string); ok {
									return k
								}
							}
						}
					}
				}
			}
		}
	}
	return ""
}

func parentOf(p string) string {
	if i := strings.LastIndexByte(p, '.'); i > 0 {
		return p[:i]
	}
	return ""
}

// jsonKind is the JSON type of an encoded value.
func jsonKind(v string) string {
	var x any
	if json.Unmarshal([]byte(v), &x) != nil {
		return ""
	}
	switch t := x.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if t == float64(int64(t)) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return ""
}

func typeFits(got, want string) bool {
	return got == want || (want == "number" && got == "integer")
}
