// Package store is a small, dependency-free JSON file store for ingested
// releases, version lists and upgrade edges. Layout under the root directory:
//
//	<dir>/<product>/versions.json
//	<dir>/<product>/releases/<semver>.json
//	<dir>/<product>/edges/<from-semver>_<to-semver>.json
//
// Files are written atomically (temp file + rename) with stable two-space
// indentation, so re-running an ingestion over unchanged sources rewrites
// byte-identical files. A stored release is reusable as long as it was
// produced from the same product definition revision (DefinitionDigest).
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
)

// DefaultDir is the default store root, relative to the working directory.
const DefaultDir = ".ri/store"

// Store is a JSON file store rooted at a directory. It is safe for
// concurrent use within one process (writes are atomic renames).
type Store struct {
	dir string
}

// New returns a store rooted at dir ("" means DefaultDir). The directory is
// created on first write.
func New(dir string) *Store {
	if dir == "" {
		dir = DefaultDir
	}
	return &Store{dir: dir}
}

// Dir returns the root directory.
func (s *Store) Dir() string { return s.dir }

// componentRe restricts path components to product ids and semvers.
var componentRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

func component(kind, v string) (string, error) {
	if !componentRe.MatchString(v) || strings.Contains(v, "..") {
		return "", fmt.Errorf("store: invalid %s %q", kind, v)
	}
	return v, nil
}

// ReleasePath returns the file holding a release (version is the semver,
// e.g. "1.18.0").
func (s *Store) ReleasePath(product, version string) (string, error) {
	p, err := component("product", product)
	if err != nil {
		return "", err
	}
	v, err := component("version", version)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.dir, p, "releases", v+".json"), nil
}

// EdgePath returns the file holding the edge from → to (semvers).
func (s *Store) EdgePath(product, from, to string) (string, error) {
	p, err := component("product", product)
	if err != nil {
		return "", err
	}
	f, err := component("version", from)
	if err != nil {
		return "", err
	}
	t, err := component("version", to)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.dir, p, "edges", f+"_"+t+".json"), nil
}

// VersionListPath returns the file holding a product's version list.
func (s *Store) VersionListPath(product string) (string, error) {
	p, err := component("product", product)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.dir, p, "versions.json"), nil
}

// SaveRelease stores a release under its product and semver.
func (s *Store) SaveRelease(r *domain.Release) error {
	if r == nil {
		return errors.New("store: nil release")
	}
	path, err := s.ReleasePath(string(r.Product), r.Version.Semver)
	if err != nil {
		return err
	}
	return writeJSON(path, r)
}

// LoadRelease returns the stored release, or nil, nil when absent.
func (s *Store) LoadRelease(product, version string) (*domain.Release, error) {
	path, err := s.ReleasePath(product, version)
	if err != nil {
		return nil, err
	}
	var r domain.Release
	ok, err := readJSON(path, &r)
	if err != nil || !ok {
		return nil, err
	}
	if string(r.Product) != product || r.Version.Semver != version {
		return nil, fmt.Errorf("store: %s holds %s@%s", path, r.Product, r.Version.Semver)
	}
	return &r, nil
}

// LoadCurrentRelease returns the stored release only when it was ingested
// with the given definition revision (see ingest.DefinitionDigest); a
// missing or stale release yields nil, nil. This is what makes re-runs
// idempotent: unchanged definitions reuse earlier ingestions.
func (s *Store) LoadCurrentRelease(product, version, definitionDigest string) (*domain.Release, error) {
	r, err := s.LoadRelease(product, version)
	if err != nil || r == nil {
		return nil, err
	}
	if definitionDigest == "" || r.DefinitionDigest != definitionDigest {
		return nil, nil
	}
	return r, nil
}

// IngestOrLoad returns the stored release of v when it is current for def,
// otherwise ingests it and stores the result. reused reports which happened.
func (s *Store) IngestOrLoad(ctx context.Context, ing *ingest.Ingester, def *catalog.ProductDefinition, v domain.Version, known *ingest.VersionList) (rel *domain.Release, reused bool, err error) {
	digest := ingest.DefinitionDigest(def)
	if v.Semver != "" {
		r, err := s.LoadCurrentRelease(def.ID, v.Semver, digest)
		if err != nil {
			return nil, false, err
		}
		if r != nil {
			return r, true, nil
		}
	}
	r, err := ing.IngestRelease(ctx, def, v, known)
	if err != nil {
		return nil, false, err
	}
	if err := s.SaveRelease(r); err != nil {
		return nil, false, err
	}
	return r, false, nil
}

// ListReleases returns the semvers stored for a product, in semver order.
func (s *Store) ListReleases(product string) ([]string, error) {
	p, err := component("product", product)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, p, "releases"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var vs []domain.Version
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		vs = append(vs, domain.Version{Semver: strings.TrimSuffix(name, ".json")})
	}
	domain.SortVersions(vs)
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Semver
	}
	return out, nil
}

// SaveEdge stores an upgrade edge under its product and endpoints.
func (s *Store) SaveEdge(e *domain.UpgradeEdge) error {
	if e == nil {
		return errors.New("store: nil edge")
	}
	path, err := s.EdgePath(string(e.Product.ID), e.From.Semver, e.To.Semver)
	if err != nil {
		return err
	}
	return writeJSON(path, e)
}

// LoadEdge returns the stored edge, or nil, nil when absent.
func (s *Store) LoadEdge(product, from, to string) (*domain.UpgradeEdge, error) {
	path, err := s.EdgePath(product, from, to)
	if err != nil {
		return nil, err
	}
	var e domain.UpgradeEdge
	ok, err := readJSON(path, &e)
	if err != nil || !ok {
		return nil, err
	}
	return &e, nil
}

// LoadCurrentEdge returns the stored edge only when it was built with the
// given definition revision; otherwise nil, nil.
func (s *Store) LoadCurrentEdge(product, from, to, definitionDigest string) (*domain.UpgradeEdge, error) {
	e, err := s.LoadEdge(product, from, to)
	if err != nil || e == nil {
		return nil, err
	}
	if definitionDigest == "" || e.DefinitionDigest != definitionDigest {
		return nil, nil
	}
	return e, nil
}

// SaveVersionList stores a product's version list.
func (s *Store) SaveVersionList(vl *ingest.VersionList) error {
	if vl == nil {
		return errors.New("store: nil version list")
	}
	path, err := s.VersionListPath(string(vl.Product))
	if err != nil {
		return err
	}
	return writeJSON(path, vl)
}

// LoadVersionList returns the stored version list, or nil, nil when absent.
func (s *Store) LoadVersionList(product string) (*ingest.VersionList, error) {
	path, err := s.VersionListPath(product)
	if err != nil {
		return nil, err
	}
	var vl ingest.VersionList
	ok, err := readJSON(path, &vl)
	if err != nil || !ok {
		return nil, err
	}
	return &vl, nil
}

// encode renders v as indented JSON without HTML escaping (so "→" and "<"
// stay readable), with a trailing newline.
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeJSON(path string, v any) error {
	data, err := encode(v)
	if err != nil {
		return fmt.Errorf("store: encode %s: %w", path, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	cleanup := func(err error) error {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return cleanup(err)
	}
	if err := tmp.Sync(); err != nil {
		return cleanup(err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// readJSON decodes path into v; ok is false when the file does not exist.
func readJSON(path string, v any) (ok bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, fmt.Errorf("store: decode %s: %w", path, err)
	}
	return true, nil
}
