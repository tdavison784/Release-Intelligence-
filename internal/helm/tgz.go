package helm

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// Limits of the archive reader. Archives are bounded by the fetch client's
// max body size on download; these bounds cover the decompressed stream.
const (
	// maxMembers bounds how many members an archive may have.
	maxMembers = 10000
	// maxMemberSize bounds one decompressed member (16 MB; the largest chart
	// members in practice are CRD bundles).
	maxMemberSize = 16 << 20
)

// Archive is the interesting content of a packaged chart tarball
// (what `helm package` produces): the chart metadata, the default values,
// the CRDs and the raw templates.
type Archive struct {
	// ChartDir is the archive's top-level directory, the chart name as
	// packaged ("acme" of "acme/Chart.yaml"). Member paths are relative to it.
	ChartDir  string
	ChartMeta *ChartMeta
	// ChartYAML and Values are the Chart.yaml / values.yaml members; Values is
	// nil when the archive has no values.yaml.
	ChartYAML []byte
	Values    []byte
	// CRDs and Templates are the crds/** / templates/** members, sorted by
	// path, with paths relative to ChartDir.
	CRDs      []ArchiveFile
	Templates []ArchiveFile
}

// ArchiveFile is one member of a chart archive.
type ArchiveFile struct {
	Path    string // relative to the chart directory, e.g. "crds/acme.yaml"
	Content []byte
}

// ReadArchive reads a packaged chart tarball (a gzip-compressed tar stream)
// and returns its interesting members: Chart.yaml, values.yaml, the CRD
// manifests and the raw templates — of the chart itself and of packaged
// subcharts (a chart's crds/ directory anywhere in the archive, e.g.
// charts/crds/crds/*.yaml, how kube-prometheus-stack ships its CRDs).
// Everything else (README, LICENSE, values.schema.json, subchart
// values/Chart.yaml, ...) is skipped. The reader is position-independent (an
// io.Reader is enough) and never touches the network; determinism follows
// from the archive bytes.
//
// The archive must have the shape helm produces: a single top-level directory
// (the chart name) containing Chart.yaml. Members outside that directory,
// absolute paths and ".." traversal are rejected.
func ReadArchive(r io.Reader) (*Archive, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("chart archive: not a gzip stream: %w", err)
	}
	defer gz.Close()

	out := &Archive{}
	tr := tar.NewReader(gz)
	members := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("chart archive: read tar: %w", err)
		}
		members++
		if members > maxMembers {
			return nil, fmt.Errorf("chart archive: more than %d members", maxMembers)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue // directories, links, anything else: skip
		}
		name := path.Clean(hdr.Name)
		if !isSafeMember(name) {
			return nil, fmt.Errorf("chart archive: unsafe member name %q", hdr.Name)
		}
		dir, rest := splitMember(name)
		if dir == "" {
			// helm writes pax global headers and occasional top-level files
			// (e.g. "README.md" provenance companions); only accept the shape
			// we parse, ignore other top-level entries.
			continue
		}
		if out.ChartDir == "" {
			out.ChartDir = dir
		} else if out.ChartDir != dir {
			return nil, fmt.Errorf("chart archive: second top-level directory %q (want only %q)", dir, out.ChartDir)
		}
		switch {
		case rest == "Chart.yaml" || rest == "Chart.yml":
			if out.ChartYAML != nil {
				return nil, fmt.Errorf("chart archive: two Chart.yaml members")
			}
			if out.ChartYAML, err = readMember(tr); err != nil {
				return nil, fmt.Errorf("chart archive: Chart.yaml: %w", err)
			}
		case rest == "values.yaml" || rest == "values.yml":
			if out.Values != nil {
				return nil, fmt.Errorf("chart archive: two values.yaml members")
			}
			if out.Values, err = readMember(tr); err != nil {
				return nil, fmt.Errorf("chart archive: values.yaml: %w", err)
			}
		case isYAML(rest) && underDir(rest, "crds") && !isChartMetaName(rest):
			// CRD manifests live in a crds/ directory — the chart's own
			// (crds/*.yaml) or a packaged subchart's (charts/crds/crds/*.yaml,
			// how kube-prometheus-stack ships its operator CRDs since 48.x).
			// A subchart NAMED "crds" contributes its own Chart.yaml and
			// values.yaml, which are chart metadata, not CRDs.
			b, err := readMember(tr)
			if err != nil {
				return nil, fmt.Errorf("chart archive: %s: %w", rest, err)
			}
			out.CRDs = append(out.CRDs, ArchiveFile{Path: rest, Content: b})
		case underDir(rest, "templates"):
			// Raw templates of the chart and of packaged subcharts; scanned
			// for static image references (nothing is rendered).
			b, err := readMember(tr)
			if err != nil {
				return nil, fmt.Errorf("chart archive: %s: %w", rest, err)
			}
			out.Templates = append(out.Templates, ArchiveFile{Path: rest, Content: b})
		}
	}
	if out.ChartYAML == nil {
		return nil, fmt.Errorf("chart archive: no Chart.yaml member")
	}
	meta, err := ParseChartYAML(out.ChartYAML)
	if err != nil {
		return nil, fmt.Errorf("chart archive: %w", err)
	}
	out.ChartMeta = meta
	sort.SliceStable(out.CRDs, func(i, j int) bool { return out.CRDs[i].Path < out.CRDs[j].Path })
	sort.SliceStable(out.Templates, func(i, j int) bool { return out.Templates[i].Path < out.Templates[j].Path })
	return out, nil
}

// isSafeMember rejects absolute paths and traversal outside the archive.
func isSafeMember(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

// splitMember splits "acme/values.yaml" into ("acme", "values.yaml"); a
// top-level file yields ("", name).
func splitMember(name string) (dir, rest string) {
	i := strings.IndexByte(name, '/')
	if i < 0 {
		return "", name
	}
	return name[:i], name[i+1:]
}

// isYAML reports whether a member path has a YAML extension.
func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

// isChartMetaName reports whether a member is a chart metadata file
// (Chart.yaml, values.yaml), whatever directory it sits in.
func isChartMetaName(name string) bool {
	base := name[strings.LastIndexByte(name, '/')+1:]
	return base == "Chart.yaml" || base == "Chart.yml" || base == "values.yaml" || base == "values.yml"
}

// underDir reports whether one of the directory elements of a member path
// (relative to the chart directory) is dir, e.g. "crds" for "crds/a.yaml" and
// "charts/crds/crds/a.yaml".
func underDir(name, dir string) bool {
	parts := strings.Split(name, "/")
	for _, p := range parts[:len(parts)-1] {
		if p == dir {
			return true
		}
	}
	return false
}

// readMember reads one member's bytes, bounded by maxMemberSize.
func readMember(tr *tar.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(tr, maxMemberSize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxMemberSize {
		return nil, fmt.Errorf("member larger than %d bytes", maxMemberSize)
	}
	return b, nil
}
