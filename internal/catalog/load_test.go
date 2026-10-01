package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func sampleYAML(t *testing.T, id string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "sample.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(string(b), "id: sample\n", "id: "+id+"\n", 1)
}

// One malformed file must not stop the others from loading.
func TestLoadDirCollectsPerFileErrors(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"a.yaml":        sampleYAML(t, "alpha"),
		"broken.yaml":   "apiVersion: ri.dev/v1alpha1\nkind: ProductDefinition\nid: broken\nname: x\nbogus: 1\n",
		"garbage.yml":   "id: [unterminated\n",
		"dup.yaml":      sampleYAML(t, "alpha"), // second definition of alpha ("a.yaml" sorts first)
		"z.yaml":        sampleYAML(t, "zulu"),
		"notes.txt":     "ignored",
		"README.md":     "ignored",
		"zz-empty.yaml": "",
	})
	cat, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir must not fail on bad files: %v", err)
	}
	var ids []string
	for _, d := range cat.List() {
		ids = append(ids, d.ID)
	}
	if got := strings.Join(ids, ","); !strings.HasPrefix(got, "alpha") || !strings.Contains(got, "zulu") || strings.Contains(got, "broken") {
		t.Fatalf("loaded ids = %v", ids)
	}
	if d, ok := cat.Get("alpha"); !ok || filepath.Base(d.Path()) != "a.yaml" {
		t.Fatalf("the first definition of a duplicated id stays loaded: %+v", d)
	}

	errs := cat.LoadErrors()
	for _, name := range []string{"broken.yaml", "garbage.yml", "dup.yaml"} {
		if errs[filepath.Join(dir, name)] == nil {
			t.Errorf("no load error for %s: %v", name, errs)
		}
	}
	if err := errs[filepath.Join(dir, "broken.yaml")]; err == nil || !strings.Contains(err.Error(), "bogus") || strings.Contains(err.Error(), dir) {
		t.Errorf("broken.yaml error should name the field and not repeat the path: %v", err)
	}
	if err := errs[filepath.Join(dir, "dup.yaml")]; err == nil || !strings.Contains(err.Error(), `duplicate product id "alpha"`) || !strings.Contains(err.Error(), "a.yaml") {
		t.Errorf("duplicate id error: %v", err)
	}
	if errs[filepath.Join(dir, "a.yaml")] != nil || errs[filepath.Join(dir, "z.yaml")] != nil || errs[filepath.Join(dir, "notes.txt")] != nil {
		t.Errorf("valid or ignored files must not be reported: %v", errs)
	}
	paths := cat.LoadErrorPaths()
	if len(paths) != len(errs) || !(paths[0] < paths[len(paths)-1]) {
		t.Errorf("LoadErrorPaths must be sorted and complete: %v", paths)
	}

	// the returned map is a copy
	delete(errs, filepath.Join(dir, "broken.yaml"))
	if len(cat.LoadErrors()) != len(paths) {
		t.Error("LoadErrors must return a copy")
	}
}

func TestLoadDirCleanAndUnreadable(t *testing.T) {
	cat, err := LoadDir(writeFiles(t, map[string]string{"a.yaml": sampleYAML(t, "alpha")}))
	if err != nil || len(cat.LoadErrors()) != 0 || len(cat.List()) != 1 {
		t.Fatalf("clean dir: %v %v", err, cat.LoadErrors())
	}
	if len(New().LoadErrors()) != 0 {
		t.Error("in-memory catalog has no load errors")
	}
	if _, err := LoadDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("an unreadable directory is still an error")
	}
}
