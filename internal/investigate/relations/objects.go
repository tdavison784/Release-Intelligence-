package relations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// object is one Kubernetes object read from a rendered manifest stream or an
// environment file.
type object struct {
	id   render.ObjectID
	body map[string]any
	// artifact is the artifact the object was read from; file is its logical
	// path for environment files ("" for rendered objects).
	artifact string
	file     string
	rendered bool
	// effNS is the namespace used to resolve references (the declared one,
	// else the kustomize/Application default); id.Namespace stays the key.
	effNS string
}

func (o *object) key() string { return o.id.String() }

// --- generic accessors over decoded YAML -------------------------------------------------

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSeq(v any) []any {
	s, _ := v.([]any)
	return s
}

func get(v any, keys ...string) any {
	for _, k := range keys {
		m := asMap(v)
		if m == nil {
			return nil
		}
		v = m[k]
	}
	return v
}

func str(v any, keys ...string) string {
	s, _ := get(v, keys...).(string)
	return s
}

// --- loading ----------------------------------------------------------------------------

func readArtifact(ctx context.Context, st investigate.ArtifactStore, a domain.ArtifactRef) ([]byte, error) {
	rc, err := st.Open(ctx, a.Digest)
	if err != nil {
		return nil, fmt.Errorf("relations: open %s (%s): %w", a.Path, a.ID, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// parseDocs decodes a multi-document YAML stream into objects. Documents
// that are not Kubernetes-shaped (no kind) are returned with ok=false so the
// caller can still look at them (kustomization files omit apiVersion/kind).
func parseDocs(data []byte) []map[string]any {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var out []map[string]any
	for {
		var doc map[string]any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A broken document loses its objects; references to them then
			// resolve as missing (fail closed), so skipping is safe.
			break
		}
		if doc != nil {
			out = append(out, doc)
		}
	}
	return out
}

func newObject(doc map[string]any, artifact, file string, rendered bool) *object {
	kind := str(doc, "kind")
	name := str(doc, "metadata", "name")
	if kind == "" || name == "" {
		return nil
	}
	group, version := "", str(doc, "apiVersion")
	if i := strings.Index(version, "/"); i >= 0 {
		group, version = version[:i], version[i+1:]
	}
	ns := str(doc, "metadata", "namespace")
	return &object{
		id:       render.ObjectID{Group: group, Version: version, Kind: kind, Namespace: ns, Name: name},
		body:     doc,
		artifact: artifact,
		file:     file,
		rendered: rendered,
		effNS:    ns,
	}
}

func isKustomizationFile(p string) bool {
	switch path.Base(p) {
	case "kustomization.yaml", "kustomization.yml", "Kustomization":
		return true
	}
	return false
}

func isYAMLFile(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".yaml", ".yml":
		return true
	}
	return false
}

// kustomization is one kustomization file of the environment.
type kustomization struct {
	dir      string // logical directory, "env/..."
	file     string // logical path of the file
	artifact string
	body     map[string]any
}

func (k *kustomization) key() string {
	return render.ObjectID{Group: "kustomize.config.k8s.io", Version: "v1beta1", Kind: "Kustomization", Name: k.dir}.String()
}

// environment is everything read from the bundle.
type environment struct {
	objects        []*object // rendered first, then files; sorted for determinism
	kustomizations map[string]*kustomization
	files          map[string]domain.ArtifactRef // env file artifacts by logical path
	// patchFiles are files used as kustomize patches: their documents are
	// partial objects, not objects.
	patchFiles map[string]bool
	// destNS is the Application destination namespace by the dir/file it
	// deploys, for default namespaces.
	pairObjects map[string][]*object // pair target → its rendered objects
	// dirNS / dirDest are the effective namespace and Application destination
	// of each kustomization directory (applyNamespaces).
	dirNS, dirDest map[string]string
	// cluster is the set of cluster-scoped kinds (clusterKinds).
	cluster map[string]bool
}

func load(ctx context.Context, b *domain.EvidenceBundle, st investigate.ArtifactStore) (*environment, error) {
	env := &environment{
		kustomizations: map[string]*kustomization{},
		files:          map[string]domain.ArtifactRef{},
		patchFiles:     map[string]bool{},
		pairObjects:    map[string][]*object{},
	}
	// Rendered objects: the TO render of every complete environment pair.
	for _, p := range b.Renders {
		if p.Status != domain.CollectionOK || p.To == "" {
			continue
		}
		ref, ok := b.Manifest.Lookup(p.To)
		if !ok {
			continue
		}
		data, err := readArtifact(ctx, st, ref)
		if err != nil {
			return nil, err
		}
		objs, err := render.ParseObjects(data)
		if err != nil {
			continue // an unparsable render contributes no objects; references stay unresolved
		}
		for _, ro := range objs {
			o := &object{id: ro.ID, body: ro.Body, artifact: ref.ID, rendered: true, effNS: ro.ID.Namespace}
			env.objects = append(env.objects, o)
			env.pairObjects[p.Target] = append(env.pairObjects[p.Target], o)
		}
	}
	// Environment files.
	var paths []string
	for _, a := range b.Manifest.Artifacts {
		if a.Kind == domain.ArtifactEnvironmentFile && isYAMLFile(a.Path) {
			env.files[a.Path] = a
			paths = append(paths, a.Path)
		}
	}
	sort.Strings(paths)
	parsed := map[string][]map[string]any{}
	for _, p := range paths {
		data, err := readArtifact(ctx, st, env.files[p])
		if err != nil {
			return nil, err
		}
		parsed[p] = parseDocs(data)
	}
	for _, p := range paths {
		if !isKustomizationFile(p) || len(parsed[p]) == 0 {
			continue
		}
		env.kustomizations[path.Dir(p)] = &kustomization{dir: path.Dir(p), file: p, artifact: env.files[p].ID, body: parsed[p][0]}
	}
	for _, k := range env.kustomizations {
		for _, patch := range asSeq(k.body["patches"]) {
			if p := str(patch, "path"); p != "" {
				env.patchFiles[path.Join(k.dir, p)] = true
			}
		}
		for _, key := range []string{"patchesStrategicMerge", "patchesJson6902"} {
			for _, patch := range asSeq(k.body[key]) {
				if s, ok := patch.(string); ok {
					env.patchFiles[path.Join(k.dir, s)] = true
				} else if p := str(patch, "path"); p != "" {
					env.patchFiles[path.Join(k.dir, p)] = true
				}
			}
		}
	}
	for _, p := range paths {
		if isKustomizationFile(p) || env.patchFiles[p] {
			continue
		}
		for _, doc := range parsed[p] {
			if o := newObject(doc, env.files[p].ID, p, false); o != nil {
				env.objects = append(env.objects, o)
			}
		}
	}
	return env, nil
}
