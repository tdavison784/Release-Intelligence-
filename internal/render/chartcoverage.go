package render

// Chart coverage for the render-backed values evaluators (PO-7a): a render
// can only decide a values key the rendered chart itself defines. Products
// ship several charts (istio: istiod, istio-cni, gateway), and the
// environment pairs render the product's primary chart; a key that belongs
// to another chart never reaches it, so the counterfactual renders
// identically with and without the key — a vacuous "nothing attributable"
// that would read as no-effect while no deployment consuming the key was
// ever rendered. Uncovered keys make the no-effect conclusion undecided
// (an evidence gap is UNKNOWN, never "no change"); attributable matches
// stand either way, and a target render's refusal of the values stays
// decisive on its own.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/helm"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// chartDefaults flattens a chart version's values.yaml exactly like the
// values diff that produced the change (flattened path → JSON).
func chartDefaults(ctx context.Context, e *Engine, product, version string) (map[string]string, string) {
	if e == nil || e.Charts == nil {
		return nil, "no chart resolver configured"
	}
	c, err := e.Charts.ResolveChart(ctx, product, version)
	if err != nil {
		return nil, "chart " + version + ": " + err.Error()
	}
	var raw []byte
	switch {
	case len(c.Archive) > 0:
		arc, err := helm.ReadArchive(bytes.NewReader(c.Archive))
		if err != nil {
			return nil, "chart " + version + ": " + err.Error()
		}
		raw = arc.Values
	case c.Path != "":
		raw, err = os.ReadFile(filepath.Join(c.Path, "values.yaml"))
		if err != nil && !os.IsNotExist(err) {
			return nil, "chart " + version + " values: " + err.Error()
		}
	}
	flat, err := normalize.FlattenValues(raw)
	if err != nil {
		return nil, "chart " + version + " values: " + err.Error()
	}
	out := make(map[string]string, len(flat))
	for _, f := range flat {
		out[f.Path] = f.Value
	}
	return out, ""
}

// uncoveredKeys returns the keys no default of the chart defines — neither
// the exact path nor a path under the key. A default ABOVE the key does not
// cover it: the chart's parent map exists, but the counterfactual deletes
// only the customer's leaf, which the chart's templates never read.
func uncoveredKeys(defaults map[string]string, keys []string) []string {
	var out []string
	for _, k := range keys {
		covered := false
		for d := range defaults {
			if pathAtOrUnder(d, k) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, k)
		}
	}
	return out
}

// pathAtOrUnder reports whether path d names key k itself or a path under
// it ('.' or '[' right after the shared prefix is a segment boundary).
func pathAtOrUnder(d, k string) bool {
	if !strings.HasPrefix(d, k) {
		return false
	}
	rest := d[len(k):]
	return rest == "" || rest[0] == '.' || rest[0] == '['
}
