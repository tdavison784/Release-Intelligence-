package helm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
	"time"
)

// buildArchive packs files (path -> content) into a deterministic gzipped tar
// stream in the order given, like helm package would.
func buildArchive(t *testing.T, files [][2]string) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: f[0], Mode: 0o644, Size: int64(len(f[1])), ModTime: time.Unix(0, 0).UTC(),
			Typeflag: tar.TypeReg, Format: tar.FormatPAX,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var gzBuf bytes.Buffer
	zw := gzip.NewWriter(&gzBuf)
	zw.ModTime = time.Unix(0, 0).UTC()
	if _, err := zw.Write(tarBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return gzBuf.Bytes()
}

const (
	testChartYAML = "apiVersion: v2\nname: acme\nversion: 1.2.3\nappVersion: 3.4.5\nkubeVersion: \">= 1.29\"\n"
	testValues    = "replicas: 1\nimage:\n  repository: quay.io/acme/app\n  tag: 3.4.5\n"
)

func TestReadArchiveMembers(t *testing.T) {
	b := buildArchive(t, [][2]string{
		{"acme/Chart.yaml", testChartYAML},
		{"acme/values.yaml", testValues},
		{"acme/README.md", "# acme"},
		{"acme/crds/crd-b.yaml", "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\n"},
		{"acme/crds/crd-a.yaml", "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\n"},
		{"acme/crds/notes.txt", "not yaml"},
		{"acme/templates/deployment.yaml", "image: quay.io/acme/app:3.4.5\n"},
		{"acme/templates/_helpers.tpl", "{{ define name }}acme{{ end }}\n"},
		{"acme/charts/sub/values.yaml", "skipped: true\n"},
		{"acme/charts/crds/Chart.yaml", "apiVersion: v2\nname: crds\n"}, // subchart NAMED crds: metadata, not a CRD
		{"acme/charts/sub/crds/sub-crd.yaml", "kind: CustomResourceDefinition\n"},
		{"acme/charts/sub/templates/svc.yaml", "kind: Service\n"},
	})
	arc, err := ReadArchive(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if arc.ChartDir != "acme" {
		t.Errorf("ChartDir = %q, want acme", arc.ChartDir)
	}
	if arc.ChartMeta == nil || arc.ChartMeta.Name != "acme" || arc.ChartMeta.Version != "1.2.3" ||
		arc.ChartMeta.AppVersion != "3.4.5" || arc.ChartMeta.KubeVersion != ">= 1.29" {
		t.Errorf("ChartMeta = %+v", arc.ChartMeta)
	}
	if string(arc.Values) != testValues {
		t.Errorf("Values = %q", arc.Values)
	}
	// CRDs sorted by path, including the subchart's; the .txt is not yaml.
	var crds []string
	for _, f := range arc.CRDs {
		crds = append(crds, f.Path)
	}
	want := []string{"charts/sub/crds/sub-crd.yaml", "crds/crd-a.yaml", "crds/crd-b.yaml"}
	if strings.Join(crds, ",") != strings.Join(want, ",") {
		t.Errorf("CRDs = %v, want %v", crds, want)
	}
	var tpls []string
	for _, f := range arc.Templates {
		tpls = append(tpls, f.Path)
	}
	wantT := []string{"charts/sub/templates/svc.yaml", "templates/_helpers.tpl", "templates/deployment.yaml"}
	if strings.Join(tpls, ",") != strings.Join(wantT, ",") {
		t.Errorf("Templates = %v, want %v", tpls, wantT)
	}
}

func TestReadArchiveNoValues(t *testing.T) {
	b := buildArchive(t, [][2]string{{"acme/Chart.yaml", testChartYAML}})
	arc, err := ReadArchive(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if arc.Values != nil {
		t.Errorf("Values = %q, want nil", arc.Values)
	}
	if len(arc.CRDs) != 0 || len(arc.Templates) != 0 {
		t.Errorf("unexpected members: %d crds, %d templates", len(arc.CRDs), len(arc.Templates))
	}
}

func TestReadArchiveRejects(t *testing.T) {
	cases := []struct {
		name  string
		files [][2]string
		want  string
	}{
		{"no Chart.yaml", [][2]string{{"acme/values.yaml", testValues}}, "no Chart.yaml"},
		{"not gzip", [][2]string{}, "not a gzip stream"},
		{
			"two top-level dirs", [][2]string{
				{"acme/Chart.yaml", testChartYAML}, {"other/Chart.yaml", testChartYAML},
			}, "second top-level directory",
		},
		{
			"broken Chart.yaml", [][2]string{{"acme/Chart.yaml", ":\n:-\n"}},
			"decode Chart.yaml",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b []byte
			if tc.name == "not gzip" {
				b = []byte("plain bytes")
			} else {
				b = buildArchive(t, tc.files)
			}
			_, err := ReadArchive(bytes.NewReader(b))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestReadArchiveUnsafeMember(t *testing.T) {
	// Hand-built tar with a traversal member outside the top-level dir.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, f := range [][2]string{
		{"acme/Chart.yaml", testChartYAML},
		{"../escape.yaml", "evil"},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: f[0], Mode: 0o644, Size: int64(len(f[1])), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(buf.Bytes())
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := ReadArchive(bytes.NewReader(gz.Bytes()))
	if err == nil || !strings.Contains(err.Error(), "unsafe member") {
		t.Fatalf("error = %v, want unsafe member rejection", err)
	}
}

func TestSplitMemberAndUnderDir(t *testing.T) {
	if d, r := splitMember("acme/values.yaml"); d != "acme" || r != "values.yaml" {
		t.Errorf("splitMember = %q, %q", d, r)
	}
	if d, r := splitMember("README.md"); d != "" || r != "README.md" {
		t.Errorf("splitMember = %q, %q", d, r)
	}
	if !underDir("crds/a.yaml", "crds") || !underDir("charts/crds/crds/a.yaml", "crds") {
		t.Error("underDir should match crds directories at any depth")
	}
	if underDir("templates/crds-like.yaml", "crds") {
		t.Error("a file name must not count as a directory element")
	}
	if !isYAML("a.yml") || isYAML("a.txt") {
		t.Error("isYAML")
	}
}
