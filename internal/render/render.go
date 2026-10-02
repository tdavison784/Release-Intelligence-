package render

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Producer is the producer string of render-derived records.
const Producer = "render@v1"

// Tool names a renderer.
type Tool string

const (
	ToolHelm      Tool = "helm"
	ToolKustomize Tool = "kustomize"
)

// Status is the outcome of one render.
type Status string

const (
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// FailureReason classifies an explicit render failure (RENDER-MISSION R13).
// A failure is an evidence gap (UNKNOWN / render-failed), never "no change".
type FailureReason string

const (
	FailChartUnavailable    FailureReason = "chart-unavailable"    // the chart package could not be resolved or fetched
	FailRendererUnavailable FailureReason = "renderer-unavailable" // the helm/kustomize binary is not installed
	FailMissingDependency   FailureReason = "missing-dependency"   // a chart dependency (subchart) is not packaged
	FailInvalidValues       FailureReason = "invalid-values"       // the values do not parse or violate values.schema.json
	FailMissingCapability   FailureReason = "missing-capability"   // kubeVersion / API capability the chart requires is absent
	FailTemplateError       FailureReason = "template-error"       // a template failed to execute (required/fail/parse)
	FailKustomizeDependency FailureReason = "kustomize-dependency" // a kustomize base/component/resource could not be loaded
	FailUnsupportedFeature  FailureReason = "unsupported-feature"  // an input feature the renderer path does not support
	FailOutputUnparsable    FailureReason = "output-unparsable"    // the renderer's output is not a YAML stream of objects
)

// FailureReasons lists every reason.
var FailureReasons = []FailureReason{
	FailChartUnavailable, FailRendererUnavailable, FailMissingDependency, FailInvalidValues,
	FailMissingCapability, FailTemplateError, FailKustomizeDependency, FailUnsupportedFeature, FailOutputUnparsable,
}

// Failure is an explicit render failure.
type Failure struct {
	Reason FailureReason `json:"reason"`
	Detail string        `json:"detail"`
}

func (f *Failure) Error() string { return "render failed (" + string(f.Reason) + "): " + f.Detail }

// Chart is a resolved Helm chart artifact: either archive bytes (a published
// package) or a local chart directory/archive path.
type Chart struct {
	Name           string                `json:"name"`
	Version        string                `json:"version"` // chart (artifact) version
	AppVersion     string                `json:"appVersion,omitempty"`
	URI            string                `json:"uri,omitempty"` // where the package came from
	Digest         string                `json:"digest"`        // sha256 of the archive (or of the directory content)
	Representation domain.Representation `json:"representation,omitempty"`
	// Archive holds the packaged chart bytes (.tgz); Path is a local chart
	// directory or archive. Exactly one is used (Archive wins).
	Archive []byte `json:"-"`
	Path    string `json:"-"`
	// Evidence records how the package was retrieved (index entry, archive).
	Evidence []domain.Evidence `json:"-"`
}

// ValuesLayer is one values input, applied in order (later layers win).
type ValuesLayer struct {
	// Origin names where the layer came from: a file path, or a locator such
	// as "apps/cert-manager.yaml#$.spec.source.helm.valuesObject".
	Origin  string `json:"origin"`
	Content []byte `json:"-"`
}

// Digest is the sha256 of the layer content.
func (v ValuesLayer) Digest() string { return domain.Digest(v.Content) }

// Request is one render.
type Request struct {
	Tool  Tool               `json:"tool"`
	Scope domain.RenderScope `json:"scope"`

	// Helm
	Chart       *Chart        `json:"chart,omitempty"`
	ReleaseName string        `json:"releaseName,omitempty"`
	Namespace   string        `json:"namespace,omitempty"`
	Values      []ValuesLayer `json:"values,omitempty"`
	// Set holds --set style overrides (Argo helm.parameters, Helmfile set,
	// counterfactual variants), applied after Values, in order.
	Set []SetValue `json:"set,omitempty"`

	// Platform inputs (helm): --kube-version and --api-versions.
	KubeVersion string   `json:"kubeVersion,omitempty"`
	APIVersions []string `json:"apiVersions,omitempty"`

	// Kustomize
	Kustomize *KustomizeInput `json:"kustomize,omitempty"`

	// ValuesComplete is false when the customer's configuration references
	// inputs no file can answer for (Flux valuesFrom, Argo valueFiles outside
	// the repository, templated Helmfile values): a render still runs, but its
	// absence of a change decides nothing.
	ValuesComplete   bool   `json:"valuesComplete"`
	IncompleteReason string `json:"incompleteReason,omitempty"`

	// Variant is set on bounded counterfactual renders (R8).
	Variant *Variant `json:"variant,omitempty"`
}

// SetValue is one --set override.
type SetValue struct {
	Path   string `json:"path"`
	Value  string `json:"value"` // JSON-encoded scalar
	Origin string `json:"origin"`
}

// KustomizeInput is a kustomize build of one overlay.
type KustomizeInput struct {
	// Dir is the overlay directory to build (inside Root).
	Dir string `json:"dir"`
	// Root bounds the input tree (the repository root); digests and the
	// version substitution operate on files under it.
	Root string `json:"root"`
	// Substitutions rewrite the product version inside the overlay's
	// kustomization files for the target render (images newTag, remote
	// resource refs): see kustomize.go.
	Substitutions []Substitution `json:"substitutions,omitempty"`
	// SourceRevision is the repository revision when known (git HEAD).
	SourceRevision string `json:"sourceRevision,omitempty"`
	// Why records why this overlay was chosen.
	Why string `json:"why,omitempty"`
}

// Substitution is one rewrite applied to a kustomization file.
type Substitution struct {
	File  string `json:"file"`  // path relative to Root
	Field string `json:"field"` // e.g. "images[quay.io/x].newTag", "resources[2]"
	From  string `json:"from"`
	To    string `json:"to"`
}

// Variant describes a bounded counterfactual render (R8): which candidate
// change asked for it and the question it answers.
type Variant struct {
	Name     string     `json:"name"`     // e.g. "foo.enabled=false"
	Question string     `json:"question"` // e.g. "is the difference due to the default of foo.enabled?"
	Set      []SetValue `json:"set"`
}

// InputDigest pins one input file.
type InputDigest struct {
	Origin string `json:"origin"`
	Digest string `json:"digest"`
}

// Provenance is everything needed to reproduce a render (R1, R2, R14).
type Provenance struct {
	Scope       domain.RenderScope `json:"scope"`
	Tool        Tool               `json:"tool"`
	ToolVersion string             `json:"toolVersion"`
	// Command is the renderer invocation with temporary paths replaced by
	// stable names (chart.tgz, values-0.yaml, …).
	Command []string `json:"command"`

	Chart          string                `json:"chart,omitempty"`
	ChartVersion   string                `json:"chartVersion,omitempty"`
	AppVersion     string                `json:"appVersion,omitempty"`
	ChartURI       string                `json:"chartUri,omitempty"`
	ArtifactDigest string                `json:"artifactDigest"` // chart archive digest, or kustomize input-tree digest
	Representation domain.Representation `json:"representation,omitempty"`
	ReleaseName    string                `json:"releaseName,omitempty"`
	Namespace      string                `json:"namespace,omitempty"`
	KubeVersion    string                `json:"kubeVersion,omitempty"`
	APIVersions    []string              `json:"apiVersions,omitempty"`

	Values           []InputDigest `json:"values,omitempty"`
	Set              []SetValue    `json:"set,omitempty"`
	ValuesDigest     string        `json:"valuesDigest,omitempty"` // over every values layer and override, in order
	ValuesComplete   bool          `json:"valuesComplete"`
	IncompleteReason string        `json:"incompleteReason,omitempty"`

	KustomizeRoot  string         `json:"kustomizeRoot,omitempty"`
	KustomizeDir   string         `json:"kustomizeDir,omitempty"`
	Inputs         []InputDigest  `json:"inputs,omitempty"` // kustomize input files
	Substitutions  []Substitution `json:"substitutions,omitempty"`
	SourceRevision string         `json:"sourceRevision,omitempty"`

	Variant *Variant `json:"variant,omitempty"`

	OutputDigest string    `json:"outputDigest,omitempty"`
	RenderedAt   time.Time `json:"renderedAt"`
	// Limitations are always-true caveats of the render (no cluster access:
	// `lookup` returns empty, …).
	Limitations []string `json:"limitations,omitempty"`
	// CacheKey is the content address the render is cached under.
	CacheKey string `json:"cacheKey"`
}

// Domain returns the contract-level provenance (domain.RenderProvenance).
func (p Provenance) Domain() domain.RenderProvenance {
	flags := append([]string(nil), p.Command...)
	if len(flags) > 0 {
		flags = flags[1:] // drop the binary
	}
	return domain.RenderProvenance{
		Scope:        p.Scope,
		Tool:         string(p.Tool),
		ToolVersion:  p.ToolVersion,
		ChartDigest:  p.ArtifactDigest,
		ValuesDigest: p.ValuesDigest,
		Flags:        flags,
	}
}

// Result is one render's outcome.
type Result struct {
	Status     Status     `json:"status"`
	Failure    *Failure   `json:"failure,omitempty"`
	Provenance Provenance `json:"provenance"`
	// Output is the rendered YAML stream (nil on failure).
	Output []byte `json:"-"`
	// Objects are the normalized objects parsed from Output.
	Objects []Object `json:"-"`
	// Nondeterministic lists paths ("<object>|<path>") that differed between
	// two renders of identical inputs (random/generated content); the diff
	// suppresses them (noise rule "nondeterministic").
	Nondeterministic []string `json:"nondeterministic,omitempty"`
	Cached           bool     `json:"cached,omitempty"`
}

// OK reports whether the render succeeded.
func (r *Result) OK() bool { return r != nil && r.Status == StatusSucceeded }

// Renderer is the rendering port: one adapter per external tool.
type Renderer interface {
	Tool() Tool
	// Version returns the tool version, or a Failure (renderer-unavailable).
	Version(ctx context.Context) (string, error)
	// Render runs one render. It never returns an error for render failures:
	// those are Result{Status: failed, Failure}.
	Render(ctx context.Context, req Request) *Result
}

// valuesDigest digests the ordered values layers and overrides.
func valuesDigest(layers []ValuesLayer, set []SetValue, variant *Variant) string {
	if len(layers) == 0 && len(set) == 0 && variant == nil {
		return ""
	}
	h := sha256.New()
	for _, l := range layers {
		h.Write([]byte("layer\x00"))
		h.Write([]byte(l.Digest()))
		h.Write([]byte{0})
	}
	for _, s := range set {
		h.Write([]byte("set\x00" + s.Path + "\x00" + s.Value + "\x00"))
	}
	if variant != nil {
		for _, s := range variant.Set {
			h.Write([]byte("variant\x00" + s.Path + "\x00" + s.Value + "\x00"))
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// cacheKey is the content address of a render: every provenance field that
// determines the output (not the timestamp, not the output digest).
func cacheKey(p Provenance) string {
	q := p
	q.RenderedAt = time.Time{}
	q.OutputDigest = ""
	q.CacheKey = ""
	q.Values = append([]InputDigest(nil), p.Values...)
	// origins are labels; only content decides the output
	for i := range q.Values {
		q.Values[i].Origin = ""
	}
	q.Set = nil
	for _, s := range p.Set {
		q.Set = append(q.Set, SetValue{Path: s.Path, Value: s.Value})
	}
	q.Inputs = append([]InputDigest(nil), p.Inputs...)
	sort.Slice(q.Inputs, func(i, j int) bool { return q.Inputs[i].Origin < q.Inputs[j].Origin })
	q.KustomizeRoot = "" // absolute location does not change the output
	q.IncompleteReason = ""
	q.SourceRevision = ""
	b, _ := json.Marshal(q)
	s := sha256.Sum256(b)
	return "rk-" + hex.EncodeToString(s[:])[:24]
}

// shortDigest abbreviates a sha256 digest for display.
func shortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}
	return d
}
