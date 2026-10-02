package semantic

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The vocabulary the prompts teach. It is data with one validated example
// per entry (vocabulary_test.go proves every example against the domain's own
// Validate, and that every family / change kind / op / consequence kind of
// the domain has an entry), so the prompt cannot teach a shape the contract
// rejects.

type familyDoc struct {
	Family   domain.SubjectFamily
	Fields   string // identity fields, as the prompt states them
	Meaning  string
	Example  domain.Subject
	Example2 string // optional extra guidance
}

var familyDocs = []familyDoc{
	{domain.SubjectCRDField, "group, kind, path (+version)", "a field of a custom resource's schema",
		domain.Subject{Family: domain.SubjectCRDField, Group: "example.io", Kind: "Widget", Path: "spec.rotation.policy"},
		"path is the schema path below the object root (spec./status.), spelled as the schema spells it, [] marks list elements: spec.rules[].host; set version ONLY when the change is specific to one API version"},
	{domain.SubjectHelmValue, "path (+name = chart, only for a subchart)", "a key of the Helm chart's values.yaml",
		domain.Subject{Family: domain.SubjectHelmValue, Path: "tls.secretsBackend"}, "path is the dotted values key path"},
	{domain.SubjectGVK, "version, kind (+group; \"\" = core)", "a served API group/version/kind",
		domain.Subject{Family: domain.SubjectGVK, Group: "example.io", Version: "v1beta1", Kind: "Widget"}, ""},
	{domain.SubjectFeatureGate, "name (+component)", "a feature gate",
		domain.Subject{Family: domain.SubjectFeatureGate, Name: "ExampleGate", Component: "controller"}, ""},
	{domain.SubjectImage, "name (= image repository)", "a container image repository",
		domain.Subject{Family: domain.SubjectImage, Name: "registry.example.io/project/controller"}, ""},
	{domain.SubjectConfigKey, "path (+component = file or binary)", "a key of a configuration file or ConfigMap (not Helm values)",
		domain.Subject{Family: domain.SubjectConfigKey, Path: "server.rbac.enforce", Component: "example-cm"}, ""},
	{domain.SubjectRBACPermission, "name = resource[/verb] (+group, component)", "a permission a component or user role grants or needs",
		domain.Subject{Family: domain.SubjectRBACPermission, Name: "widgets/update", Group: "example.io"}, ""},
	{domain.SubjectCLIFlag, "name (with leading --) (+component)", "a command-line flag",
		domain.Subject{Family: domain.SubjectCLIFlag, Name: "--enable-widgets", Component: "controller"}, ""},
	{domain.SubjectEnvVar, "name (+component)", "an environment variable",
		domain.Subject{Family: domain.SubjectEnvVar, Name: "EXAMPLE_MODE", Component: "controller"}, ""},
	{domain.SubjectCompatibilityBoundary, "name (= platform or operand)", "a supported-version boundary of a platform (kubernetes) or managed operand",
		domain.Subject{Family: domain.SubjectCompatibilityBoundary, Name: "kubernetes"}, ""},
	{domain.SubjectAPIEndpoint, "name = METHOD /path (+component)", "an HTTP/gRPC endpoint the product serves",
		domain.Subject{Family: domain.SubjectAPIEndpoint, Name: "GET /api/v1/widgets", Component: "server"}, ""},
	{domain.SubjectProtocolBehavior, "name (+component)", "wire/protocol behaviour (TLS, ALPN, path matching, signing)",
		domain.Subject{Family: domain.SubjectProtocolBehavior, Name: "grpc-alpn"}, ""},
	{domain.SubjectTerraformAttribute, "kind (= resource type), path", "an attribute of a Terraform resource",
		domain.Subject{Family: domain.SubjectTerraformAttribute, Kind: "example_bucket", Path: "acl"}, ""},
	{domain.SubjectProductRelationship, "name (= the OTHER product's id)", "a dependency on, or interaction with, another product",
		domain.Subject{Family: domain.SubjectProductRelationship, Name: "ingress-nginx"}, ""},
	{domain.SubjectMigration, "name (a short step identifier) (+component)", "a mandatory operator step",
		domain.Subject{Family: domain.SubjectMigration, Name: "storage-version-migration"}, ""},
}

type changeDoc struct {
	Kind    domain.ChangeKind
	Meaning string
	Fields  string
}

var changeDocs = []changeDoc{
	{domain.ChangeKindAdded, "the subject now exists", "after optional"},
	{domain.ChangeKindRemoved, "the subject no longer exists or is no longer honoured", "before optional; replacedBy optional"},
	{domain.ChangeKindRenamed, "the subject moved to another identity", "replacedBy REQUIRED (same family)"},
	{domain.ChangeKindDefaultChanged, "the default value changed", "before AND after REQUIRED, different"},
	{domain.ChangeKindValueChanged, "a fixed value changed (an image tag, a pinned version)", "before AND after REQUIRED, different"},
	{domain.ChangeKindBehaviorChanged, "same configuration, different semantics", "before/after optional"},
	{domain.ChangeKindDeprecated, "still works, scheduled for removal", "replacedBy optional"},
	{domain.ChangeKindNowRequired, "previously optional, now mandatory", "after optional"},
	{domain.ChangeKindValidationTightened, "values accepted before are now rejected", "before/after optional"},
	{domain.ChangeKindRequirementChanged, "a version requirement moved (compatibility-boundary / product-relationship only)", "after REQUIRED: a semver constraint string such as \">=1.12.0\""},
	{domain.ChangeKindMigrationRequired, "an operator step must run (migration family only, and the migration family only uses this)", "-"},
}

type consequenceDoc struct {
	Kind    domain.ConsequenceKind
	Meaning string
}

var consequenceDocs = []consequenceDoc{
	{domain.ConsequenceUpgradeBlocked, "the upgrade/install itself fails (version gate, failing hook, unmet precondition)"},
	{domain.ConsequenceResourceRejected, "existing or newly applied resources are rejected (validation, removed API)"},
	{domain.ConsequenceSettingIgnored, "configured values stop being honoured (removed/renamed key, unserved field)"},
	{domain.ConsequencePermissionLost, "a component or user loses access it relies on"},
	{domain.ConsequenceWorkloadFailure, "a running workload or integration breaks (incompatible peer, protocol enforcement)"},
	{domain.ConsequenceMigrationRequired, "a mandatory operator step must run before/after the upgrade"},
	{domain.ConsequenceBehaviorChange, "it still works, but differently (e.g. a new default applies); worth verifying"},
	{domain.ConsequenceDeprecation, "works today, scheduled to break in a later release"},
	{domain.ConsequenceNone, "no operational consequence for an operator"},
}

type opDoc struct {
	Op      domain.ConditionOp
	Fields  string
	Meaning string
	Example domain.Condition
}

func cond(op domain.ConditionOp, f func(*domain.Condition)) domain.Condition {
	c := domain.Condition{Op: op}
	if f != nil {
		f(&c)
	}
	return c
}

var fieldUnset = cond(domain.OpField, func(c *domain.Condition) { c.Path = "spec.rotation.policy"; c.State = domain.StateUnset })

var opDocs = []opDoc{
	{domain.OpAll, "of: [conditions]", "every operand holds (conjunction)",
		cond(domain.OpAll, func(c *domain.Condition) {
			c.Of = []domain.Condition{
				cond(domain.OpValuesKey, func(c *domain.Condition) { c.Path = "old.key"; c.State = domain.StateSet }),
				cond(domain.OpValuesKey, func(c *domain.Condition) { c.Path = "new.key"; c.State = domain.StateUnset }),
			}
		})},
	{domain.OpAny, "of: [conditions]", "some operand holds", cond(domain.OpAny, func(c *domain.Condition) {
		c.Of = []domain.Condition{
			cond(domain.OpGVKInUse, func(c *domain.Condition) { c.Group = "example.io"; c.Version = "v1beta1"; c.Kind = "Widget" }),
			cond(domain.OpGVKInUse, func(c *domain.Condition) { c.Group = "example.io"; c.Version = "v1beta1"; c.Kind = "Gadget" }),
		}
	})},
	{domain.OpNot, "of: [exactly one condition]", "negation", cond(domain.OpNot, func(c *domain.Condition) {
		c.Of = []domain.Condition{cond(domain.OpValuesKey, func(c *domain.Condition) { c.Path = "feature.enabled"; c.State = domain.StateSet })}
	})},
	{domain.OpResource, "kind (+group, version, name), of: [scoped conditions]", "SOME single resource of that kind satisfies all operands",
		cond(domain.OpResource, func(c *domain.Condition) {
			c.Group = "example.io"
			c.Kind = "Widget"
			c.Of = []domain.Condition{fieldUnset}
		})},
	{domain.OpField, "path, state (+values | pattern | separator) — only inside resource/ref", "a field of the scoped resource", fieldUnset},
	{domain.OpTextLine, "path, pattern, state exists|none — only inside resource/ref", "lines of embedded text (e.g. ConfigMap data[\"policy.csv\"])",
		cond(domain.OpTextLine, func(c *domain.Condition) {
			c.Path = `data["policy.csv"]`
			c.Pattern = `^p, .*, widgets, update`
			c.State = domain.StateExists
		})},
	{domain.OpRef, "path (the *Ref object) (+kind, group), of: [scoped conditions] — only inside resource/ref", "the reference resolves to a resource satisfying the operands",
		cond(domain.OpRef, func(c *domain.Condition) {
			c.Path = "spec.issuerRef"
			c.Kind = "Issuer"
			c.Of = []domain.Condition{cond(domain.OpField, func(c *domain.Condition) { c.Path = "spec.ca"; c.State = domain.StateSet })}
		})},
	{domain.OpValuesKey, "path, state (+values | pattern | separator)", "the environment's Helm values",
		cond(domain.OpValuesKey, func(c *domain.Condition) { c.Path = "tls.secretsBackend"; c.State = domain.StateSet })},
	{domain.OpGVKInUse, "kind (+group, version)", "manifests use this API version/kind",
		cond(domain.OpGVKInUse, func(c *domain.Condition) { c.Group = "example.io"; c.Version = "v1beta1"; c.Kind = "Widget" })},
	{domain.OpImageInUse, "name (repository) (+state in-range|out-of-range with range on the tag)", "the environment references the image",
		cond(domain.OpImageInUse, func(c *domain.Condition) { c.Name = "registry.example.io/project/controller" })},
	{domain.OpCLIFlag, "name, state (+values | pattern, component)", "a container argument of the product's workloads",
		cond(domain.OpCLIFlag, func(c *domain.Condition) { c.Name = "--enable-widgets"; c.State = domain.StateSet })},
	{domain.OpEnvVar, "name, state (+values | pattern, component)", "a container environment variable",
		cond(domain.OpEnvVar, func(c *domain.Condition) { c.Name = "EXAMPLE_MODE"; c.State = domain.StateSet })},
	{domain.OpFeatureGate, "name, state enabled|disabled|unset (+path = values key carrying the gate list, component)", "a feature gate setting",
		cond(domain.OpFeatureGate, func(c *domain.Condition) { c.Name = "ExampleGate"; c.State = domain.StateEnabled })},
	{domain.OpProductVersion, "name (product id), range, state in-range|out-of-range", "another product's version in the environment",
		cond(domain.OpProductVersion, func(c *domain.Condition) {
			c.Name = "ingress-nginx"
			c.Range = ">=1.12.0"
			c.State = domain.StateInRange
		})},
	{domain.OpClusterVersion, "name (platform), range, state in-range|out-of-range", "the platform version",
		cond(domain.OpClusterVersion, func(c *domain.Condition) {
			c.Name = "kubernetes"
			c.Range = ">=1.29.0"
			c.State = domain.StateOutOfRange
		})},
	{domain.OpEdgeFromVersion, "range, state in-range|out-of-range", "the version the environment runs today (upgrade from-version)",
		cond(domain.OpEdgeFromVersion, func(c *domain.Condition) { c.Range = "<1.15.6"; c.State = domain.StateInRange })},
	{domain.OpRenderedChange, "kind, path (+group, name, values), state changed|unchanged|added|removed", "a field of the chart's rendered objects differs between the from and to release with the environment's values",
		cond(domain.OpRenderedChange, func(c *domain.Condition) {
			c.Kind = "Deployment"
			c.Path = "spec.template.spec.containers[].args"
			c.State = domain.StateChanged
		})},
	{domain.OpUndecidable, "reason (unknown reason), needed", "exposure depends on something no static input carries (runtime state, live traffic)",
		cond(domain.OpUndecidable, func(c *domain.Condition) {
			c.Reason = domain.UnknownRuntimeBehaviorGap
			c.Needed = "which clients connect over the legacy protocol at runtime"
		})},
}

// renderVocabulary writes the parts of the vocabulary a task needs.
func renderVocabulary(b *strings.Builder, aspects []domain.Aspect) {
	has := func(a domain.Aspect) bool { return containsAspect(aspects, a) }
	if has(domain.AspectSubject) || has(domain.AspectChange) {
		b.WriteString(subjectGuide)
		for _, d := range familyDocs {
			fmt.Fprintf(b, "- %s: %s. Fields: %s. Example: %s", d.Family, d.Meaning, d.Fields, subjectJSON(d.Example))
			if d.Example2 != "" {
				fmt.Fprintf(b, " (%s)", d.Example2)
			}
			b.WriteString("\n")
		}
	}
	if has(domain.AspectChange) {
		b.WriteString("\nCHANGE TYPES (before/after: the literal value as the text states it — string, number, boolean, or null for nil; never paraphrased):\n")
		for _, d := range changeDocs {
			fmt.Fprintf(b, "- %s: %s. %s\n", d.Kind, d.Meaning, d.Fields)
		}
	}
	if has(domain.AspectApplicability) {
		b.WriteString(applicabilityGuide)
		for _, d := range opDocs {
			ex, _ := jsonCompact(d.Example)
			fmt.Fprintf(b, "- %s: %s. %s. Example: %s\n", d.Op, d.Meaning, d.Fields, ex)
		}
		b.WriteString(statesGuide)
		b.WriteString(canonicalGuide)
	}
	if has(domain.AspectConsequence) {
		b.WriteString("\nCONSEQUENCE KINDS (what happens to an exposed environment whose operator does nothing):\n")
		for _, d := range consequenceDocs {
			fmt.Fprintf(b, "- %s: %s\n", d.Kind, d.Meaning)
		}
		b.WriteString(consequenceGuide)
	}
}

func subjectJSON(s domain.Subject) string {
	m := map[string]string{"family": string(s.Family)}
	for _, kv := range [][2]string{{"group", s.Group}, {"version", s.Version}, {"kind", s.Kind}, {"path", s.Path}, {"name", s.Name}, {"component", s.Component}} {
		if kv[1] != "" {
			m[kv[0]] = kv[1]
		}
	}
	out, _ := jsonCompact(m)
	return out
}

const subjectGuide = `
SUBJECT FAMILIES. Two answers naming the same thing must spell it identically, so:
- set exactly the identity fields a family lists; the optional ones in (+…) ONLY when the text makes them
  part of the identity: component only when the text names a specific component/binary/chart of the product
  that owns the subject (never the product's own name); a crd-field's version only when the change is
  specific to one API version;
- copy names from the text or the ARTIFACT CONTEXT exactly (case, punctuation, list markers);
- free-form names (protocol-behavior, migration, api-endpoint): lowercase kebab-case built only from the
  evidence's own key words, as short as stays unambiguous ("http01-ingress-pathtype");
- the product is implied (the candidate's product); never put it in a field.
`

const applicabilityGuide = `
APPLICABILITY: a condition tree over a customer's environment, evaluated later by a deterministic engine
with three-valued logic. "exposure" = the environment state in which the consequence happens.
"overlap" (optional) = the environment touches the subject but is shielded (e.g. it pins the old value
explicitly); such environments are told "applies to you, you appear safe".
Scoped leaves (field, text-line, ref) exist ONLY inside a resource (or ref) node; every other leaf is
environment-wide and never inside a resource. At most ` + "4" + ` levels deep. Ops:
`

const statesGuide = `States: unset | set | equals (values = literal values; any of) | not-equals | matches (pattern = RE2) |
has-token / has-token-key (values = tokens of a separator-delimited list, default ","; has-token-key matches
the key of k=v tokens such as --feature-gates=Gate=true) | enabled | disabled | unset (feature-gate) |
in-range | out-of-range (range = semver constraint; "requires >= X" is out-of-range of ">=X") |
exists | none (text-line) | changed | unchanged | added | removed (rendered-change).
`

const canonicalGuide = `Canonical shapes (use them when they fit; they are what an engineer would write):
- helm-value default-changed: exposure values-key{path, unset}; overlap values-key{path, set}
- helm-value removed/renamed: exposure values-key{path, set}
- helm-value deprecated with replacedBy: exposure all[values-key{path, set}, values-key{replacedBy.path, unset}]; overlap values-key{replacedBy.path, set}
- crd-field default-changed: exposure resource{group, kind, of:[field{path, unset}]}; overlap resource{group, kind, of:[field{path, set}]}
- crd-field removed/validation-tightened: exposure resource{group, kind, of:[field{path, set}]}
- gvk removed: exposure gvk-in-use{group, version, kind}
- image removed/value-changed: exposure image-in-use{name}
- cli-flag/env-var removed/renamed: exposure cli-flag/env-var{name, set}
- product-relationship requirement-changed: exposure product-version{name, out-of-range, range: the new requirement}
- compatibility-boundary requirement-changed: exposure cluster-version{name, out-of-range, range}
If exposure depends on runtime behaviour no configuration shows, use undecidable{reason, needed} (alone or as
an operand) instead of guessing a predicate.
`

const consequenceGuide = `The class an exposed environment receives follows from the kind (you do not choose it):
upgrade-blocked, resource-rejected, setting-ignored, permission-lost, workload-failure, migration-required →
mandatory work; behavior-change, deprecation → review; none → informational.
Pick a failure kind ONLY when the evidence says something fails, is rejected, stops being honoured, loses
access or must be migrated; "statement" then says exactly what fails if the operator does nothing, in the
evidence's terms. A changed default that keeps working is behavior-change. A deprecation that still works is
deprecation. When the evidence does not say what happens, the consequence is undetermined.
`
