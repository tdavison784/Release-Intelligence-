package render

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Limitations recorded on every Helm render.
var helmLimitations = []string{
	"no cluster access: `lookup` returns empty objects and .Capabilities come only from --kube-version/--api-versions",
	"chart tests (helm.sh/hook: test) are skipped; other hooks are rendered",
}

// Helm renders charts with `helm template` (external binary).
type Helm struct {
	Runner Runner
	Binary string // default "helm"
	Cache  *Cache // optional
	// DetectNondeterminism renders twice and suppresses fields that differ
	// between identical renders (generated certificates, random strings).
	DetectNondeterminism bool
	Now                  func() time.Time

	once    sync.Once
	version string
	verErr  error
}

// NewHelm returns a Helm renderer with the default runner.
func NewHelm(cache *Cache) *Helm {
	return &Helm{Runner: ExecRunner{}, Binary: "helm", Cache: cache, DetectNondeterminism: true, Now: time.Now}
}

// Tool implements Renderer.
func (h *Helm) Tool() Tool { return ToolHelm }

func (h *Helm) bin() string {
	if h.Binary == "" {
		return "helm"
	}
	return h.Binary
}

// Version implements Renderer.
func (h *Helm) Version(ctx context.Context) (string, error) {
	h.once.Do(func() {
		if _, err := h.Runner.LookPath(h.bin()); err != nil {
			h.verErr = &Failure{Reason: FailRendererUnavailable, Detail: fmt.Sprintf("%s is not installed (or not on PATH): %v", h.bin(), err)}
			return
		}
		out, errb, err := h.Runner.Run(ctx, "", h.bin(), "version", "--template", "{{.Version}}")
		if err != nil {
			h.verErr = &Failure{Reason: FailRendererUnavailable, Detail: fmt.Sprintf("%s version: %v: %s", h.bin(), err, firstLine(errb))}
			return
		}
		h.version = strings.TrimSpace(string(out))
	})
	return h.version, h.verErr
}

// Render implements Renderer.
func (h *Helm) Render(ctx context.Context, req Request) *Result {
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	prov := Provenance{
		Scope: req.Scope, Tool: ToolHelm, ReleaseName: req.ReleaseName, Namespace: req.Namespace,
		KubeVersion: req.KubeVersion, APIVersions: req.APIVersions, Set: req.Set,
		ValuesComplete: req.ValuesComplete, IncompleteReason: req.IncompleteReason, Variant: req.Variant,
		RenderedAt: now().UTC().Truncate(time.Second), Limitations: helmLimitations,
	}
	if req.Chart != nil {
		prov.Chart, prov.ChartVersion, prov.AppVersion = req.Chart.Name, req.Chart.Version, req.Chart.AppVersion
		prov.ChartURI, prov.ArtifactDigest, prov.Representation = req.Chart.URI, req.Chart.Digest, req.Chart.Representation
	}
	for _, l := range req.Values {
		prov.Values = append(prov.Values, InputDigest{Origin: l.Origin, Digest: l.Digest()})
	}
	prov.ValuesDigest = valuesDigest(req.Values, req.Set, req.Variant)
	fail := func(reason FailureReason, format string, args ...any) *Result {
		return &Result{Status: StatusFailed, Failure: &Failure{Reason: reason, Detail: fmt.Sprintf(format, args...)}, Provenance: prov}
	}
	ver, err := h.Version(ctx)
	if err != nil {
		if f, ok := err.(*Failure); ok {
			return &Result{Status: StatusFailed, Failure: f, Provenance: prov}
		}
		return fail(FailRendererUnavailable, "%v", err)
	}
	prov.ToolVersion = ver
	if req.Chart == nil || (len(req.Chart.Archive) == 0 && req.Chart.Path == "") {
		return fail(FailChartUnavailable, "no chart package was resolved")
	}
	if req.ReleaseName == "" || req.Namespace == "" {
		return fail(FailUnsupportedFeature, "release name and namespace are required for a reproducible render")
	}

	args := []string{"template", req.ReleaseName, "chart.tgz", "--namespace", req.Namespace, "--include-crds", "--skip-tests"}
	if req.Chart.Path != "" && len(req.Chart.Archive) == 0 {
		args[2] = "chart"
	}
	if req.KubeVersion != "" {
		args = append(args, "--kube-version", req.KubeVersion)
	}
	for _, av := range req.APIVersions {
		args = append(args, "--api-versions", av)
	}
	for i := range req.Values {
		args = append(args, "-f", fmt.Sprintf("values-%d.yaml", i))
	}
	sets := append([]SetValue(nil), req.Set...)
	if req.Variant != nil {
		sets = append(sets, req.Variant.Set...)
	}
	for _, s := range sets {
		args = append(args, "--set-json", s.Path+"="+s.Value)
	}
	prov.Command = append([]string{"helm"}, args...)
	prov.CacheKey = cacheKey(prov)
	if h.Cache != nil {
		if res, ok := h.Cache.Get(prov.CacheKey); ok {
			res.Provenance.RenderedAt = prov.RenderedAt
			res.Provenance.Values = prov.Values // keep this run's origins
			res.Provenance.Set = prov.Set
			res.Provenance.IncompleteReason = prov.IncompleteReason
			res.Cached = true
			return res
		}
	}

	dir, err := os.MkdirTemp("", "ri-helm-")
	if err != nil {
		return fail(FailUnsupportedFeature, "temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	if len(req.Chart.Archive) > 0 {
		if err := os.WriteFile(filepath.Join(dir, "chart.tgz"), req.Chart.Archive, 0o600); err != nil {
			return fail(FailChartUnavailable, "write chart: %v", err)
		}
	} else {
		abs, err := filepath.Abs(req.Chart.Path)
		if err != nil {
			return fail(FailChartUnavailable, "chart path: %v", err)
		}
		if err := os.Symlink(abs, filepath.Join(dir, "chart")); err != nil {
			return fail(FailChartUnavailable, "chart path: %v", err)
		}
	}
	for i, l := range req.Values {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("values-%d.yaml", i)), l.Content, 0o600); err != nil {
			return fail(FailInvalidValues, "write values: %v", err)
		}
	}
	out, errb, err := h.Runner.Run(ctx, dir, h.bin(), args...)
	if err != nil {
		reason, detail := classifyHelm(string(errb), err)
		return fail(reason, "%s", detail)
	}
	res, f := finish(prov, out)
	if f != nil {
		return &Result{Status: StatusFailed, Failure: f, Provenance: prov}
	}
	if h.DetectNondeterminism && !req.NoProbe {
		out2, _, err := h.Runner.Run(ctx, dir, h.bin(), args...)
		if err == nil && string(out2) != string(out) {
			if objs2, err := ParseObjects(out2); err == nil {
				res.Nondeterministic = nondeterministicPaths(res.Objects, objs2)
			}
		}
	}
	if h.Cache != nil {
		h.Cache.Put(res)
	}
	return res
}

// finish parses a successful render's output into objects.
func finish(prov Provenance, out []byte) (*Result, *Failure) {
	objs, err := ParseObjects(out)
	if err != nil {
		return nil, &Failure{Reason: FailOutputUnparsable, Detail: err.Error()}
	}
	prov.OutputDigest = domain.Digest(out)
	return &Result{Status: StatusSucceeded, Provenance: prov, Output: out, Objects: objs}, nil
}

var (
	reMissingDep   = regexp.MustCompile(`(?i)(found in Chart\.yaml, but missing in charts/|missing in charts/ directory|dependencies? .*(not|missing))`)
	reValues       = regexp.MustCompile(`(?i)(values don't meet the specifications of the schema|failed to parse .*values|error converting YAML to JSON|cannot unmarshal .* into Go value of type map|invalid --set|parsing --set|unable to parse key)`)
	reCapability   = regexp.MustCompile(`(?i)(chart requires kubeVersion|is incompatible with Kubernetes|no matches for kind|ensure CRDs are installed first|resource mapping not found|invalid kube version)`)
	reChartRejects = regexp.MustCompile(`execution error at \(`)
	reChartAccess  = regexp.MustCompile(`(?i)(failed to download|not a valid chart|chart not found|no such file or directory|Chart\.yaml file is missing)`)
)

// classifyHelm maps helm's stderr onto the R13 failure classes. The first
// line of stderr is the detail; nothing is inferred beyond the patterns.
func classifyHelm(stderr string, err error) (FailureReason, string) {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = err.Error()
	}
	detail = boundDetail(detail)
	switch {
	case reChartRejects.MatchString(stderr):
		// helm reports chart-authored `fail` / `required` checks as
		// "execution error at (<template>:<line>:<col>): <message>": the
		// chart itself rejects this configuration
		return FailInvalidValues, "the chart rejects these values: " + detail
	case reMissingDep.MatchString(stderr):
		return FailMissingDependency, detail
	case reValues.MatchString(stderr):
		return FailInvalidValues, detail
	case reCapability.MatchString(stderr):
		return FailMissingCapability, detail
	case reChartAccess.MatchString(stderr):
		return FailChartUnavailable, detail
	case strings.Contains(err.Error(), "timed out"):
		return FailTemplateError, "render timed out: " + detail
	}
	return FailTemplateError, detail
}

func boundDetail(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return s
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
