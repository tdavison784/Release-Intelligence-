package reportui

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
)

// ErrReadOnly is returned by a source that cannot record review decisions
// (the stored evaluation runs).
var ErrReadOnly = errors.New("read-only source: review decisions are recorded in an investigation store")

// Record is one investigation as the report shows it: the run record, the
// evidence bundle it was built from (nil when the source does not hold it)
// and where it came from.
type Record struct {
	Key           string
	Investigation *domain.Investigation
	Bundle        *domain.EvidenceBundle
	// Origin says where the record is stored ("investigation store <dir>",
	// "evaluation run <id> (arm …)"); Run is set for stored evaluation runs.
	Origin string
	Run    *RunInfo
}

// RunInfo describes a stored evaluation run (eval/investigations/results/<id>/run.json).
type RunInfo struct {
	ID        string    `json:"runId"`
	Arm       string    `json:"arm"`
	ArmDetail string    `json:"armDetail"`
	Commit    string    `json:"commit"`
	StartedAt time.Time `json:"startedAt"`
}

// Label is a short human name for the run.
func (r *RunInfo) Label() string {
	if r == nil {
		return ""
	}
	s := r.ID
	if r.ArmDetail != "" {
		s += " · arm " + r.Arm + " " + r.ArmDetail
	} else if r.Arm != "" {
		s += " · arm " + r.Arm
	}
	return s
}

// Source holds investigations. Keys are URL-safe path segments (they may
// contain "/").
type Source interface {
	// Keys lists every investigation key.
	Keys(ctx context.Context) ([]string, error)
	// Load returns one investigation; investigate.ErrNotFound when absent.
	Load(ctx context.Context, key string) (*Record, error)
	// Artifact returns the bytes of an artifact digest the source holds for
	// the investigation key; investigate.ErrNotFound when it does not.
	Artifact(ctx context.Context, key, digest string) ([]byte, error)
	// Writable reports whether Update works.
	Writable() bool
	// Update loads the run record, applies fn and stores the result
	// atomically with respect to other updates; ErrReadOnly when not writable.
	Update(ctx context.Context, key string, fn func(*domain.Investigation) error) error
	// Describe names the source for headers and logs.
	Describe() string
}

// Open opens the investigations below dir, detecting the layout:
//
//   - a state directory (<dir>/investigations/runs) or its investigations
//     directory: the live store, writable;
//   - an evaluation results directory (<dir>/<run>/run.json, each run packed
//     as cases.tar.gz or unpacked as cases/<case>/): read-only;
//   - a single evaluation run directory (<dir>/run.json): read-only.
func Open(dir string) (Source, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("reportui: a store directory is required")
	}
	if st, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("reportui: %w", err)
	} else if !st.IsDir() {
		return nil, fmt.Errorf("reportui: %s is not a directory", dir)
	}
	switch {
	case isDir(filepath.Join(dir, "investigations", "runs")):
		return NewStoreSource(dir)
	case filepath.Base(filepath.Clean(dir)) == "investigations" && isDir(filepath.Join(dir, "runs")):
		return NewStoreSource(filepath.Dir(filepath.Clean(dir)))
	case isFile(filepath.Join(dir, "run.json")):
		return OpenRuns([]string{dir})
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reportui: %w", err)
	}
	var runs []string
	for _, e := range ents {
		if e.IsDir() && isFile(filepath.Join(dir, e.Name(), "run.json")) {
			runs = append(runs, filepath.Join(dir, e.Name()))
		}
	}
	if len(runs) > 0 {
		return OpenRuns(runs)
	}
	// A state directory without investigations yet: the live store (empty).
	return NewStoreSource(dir)
}

func isDir(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }
func isFile(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

// --- the live store (<state>/investigations) ---------------------------------------------

// StoreSource serves the investigation store of DESIGN.md §2.3 through the
// store package: reads verify what they return, and a review is written with
// PutInvestigation (validated, atomic, the audit trail only grows).
type StoreSource struct {
	st *store.Store
	mu sync.Mutex
}

// NewStoreSource opens the investigation store below a state directory.
func NewStoreSource(stateRoot string) (*StoreSource, error) {
	st, err := store.New(stateRoot)
	if err != nil {
		return nil, err
	}
	return &StoreSource{st: st}, nil
}

// Describe implements Source.
func (s *StoreSource) Describe() string { return "investigation store " + s.st.Dir() }

// Writable implements Source.
func (s *StoreSource) Writable() bool { return true }

// Keys implements Source: the investigation ids under runs/.
func (s *StoreSource) Keys(ctx context.Context) ([]string, error) {
	ents, err := os.ReadDir(filepath.Join(s.st.Dir(), "runs"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, e := range ents {
		if n := e.Name(); !e.IsDir() && strings.HasPrefix(n, domain.InvestigationIDPrefix) && strings.HasSuffix(n, ".json") {
			keys = append(keys, strings.TrimSuffix(n, ".json"))
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// Load implements Source. A bundle that cannot be read leaves Bundle nil
// (the page says so); the run record alone is still shown.
func (s *StoreSource) Load(ctx context.Context, key string) (*Record, error) {
	if !validID(key) {
		return nil, fmt.Errorf("investigation %q: %w", key, investigate.ErrNotFound)
	}
	inv, err := s.st.GetInvestigation(ctx, key)
	if err != nil {
		return nil, err
	}
	rec := &Record{Key: key, Investigation: inv, Origin: s.Describe()}
	if b, err := s.st.GetBundle(ctx, inv.Bundle); err == nil {
		rec.Bundle = b
	}
	return rec, nil
}

// Artifact implements Source: the CAS bytes, digest verified by the store.
func (s *StoreSource) Artifact(ctx context.Context, _ string, digest string) ([]byte, error) {
	rc, err := s.st.Open(ctx, digest)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Update implements Source.
func (s *StoreSource) Update(ctx context.Context, key string, fn func(*domain.Investigation) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.Load(ctx, key)
	if err != nil {
		return err
	}
	if err := fn(rec.Investigation); err != nil {
		return err
	}
	return s.st.PutInvestigation(ctx, rec.Investigation)
}

func validID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\`) && !strings.Contains(id, "..")
}

func readJSON(p string, v any) error {
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return investigate.ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// --- stored evaluation runs (eval/investigations/results/<run>/) -------------------------

// RunsSource serves stored evaluation runs read-only. Each run holds its
// per-case records packed in cases.tar.gz (cases/<case>/<file>) or unpacked in
// cases/<case>/; the key of a case is "<run>/<case>".
type RunsSource struct {
	runs  map[string]*runDir
	order []string
}

type runDir struct {
	dir   string
	info  RunInfo
	cases []string
}

// OpenRuns opens evaluation run directories (each containing run.json).
func OpenRuns(dirs []string) (*RunsSource, error) {
	s := &RunsSource{runs: map[string]*runDir{}}
	for _, d := range dirs {
		var info RunInfo
		if err := readJSON(filepath.Join(d, "run.json"), &info); err != nil {
			return nil, fmt.Errorf("reportui: %s/run.json: %w", d, err)
		}
		if info.ID == "" {
			info.ID = filepath.Base(d)
		}
		files, err := readRunFiles(d, "")
		if err != nil {
			return nil, err
		}
		cases := map[string]bool{}
		for name := range files {
			if c, f, ok := strings.Cut(name, "/"); ok && f == "investigation.json" {
				cases[c] = true
			}
		}
		r := &runDir{dir: d, info: info}
		for c := range cases {
			r.cases = append(r.cases, c)
		}
		sort.Strings(r.cases)
		if !validID(info.ID) {
			return nil, fmt.Errorf("reportui: run id %q is not a path segment", info.ID)
		}
		s.runs[info.ID] = r
		s.order = append(s.order, info.ID)
	}
	sort.Strings(s.order)
	return s, nil
}

// Describe implements Source.
func (s *RunsSource) Describe() string {
	return fmt.Sprintf("%d stored evaluation run(s), read-only", len(s.order))
}

// Writable implements Source.
func (s *RunsSource) Writable() bool { return false }

// Update implements Source.
func (s *RunsSource) Update(context.Context, string, func(*domain.Investigation) error) error {
	return ErrReadOnly
}

// Keys implements Source.
func (s *RunsSource) Keys(ctx context.Context) ([]string, error) {
	var keys []string
	for _, id := range s.order {
		for _, c := range s.runs[id].cases {
			keys = append(keys, id+"/"+c)
		}
	}
	return keys, nil
}

// Load implements Source.
func (s *RunsSource) Load(ctx context.Context, key string) (*Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runID, c, ok := strings.Cut(key, "/")
	r := s.runs[runID]
	if !ok || r == nil || !validID(c) {
		return nil, fmt.Errorf("investigation %q: %w", key, investigate.ErrNotFound)
	}
	files, err := readRunFiles(r.dir, c)
	if err != nil {
		return nil, err
	}
	raw, ok := files[c+"/investigation.json"]
	if !ok {
		return nil, fmt.Errorf("investigation %q: %w", key, investigate.ErrNotFound)
	}
	var inv domain.Investigation
	if err := json.Unmarshal(raw, &inv); err != nil {
		return nil, fmt.Errorf("%s: investigation.json: %w", key, err)
	}
	info := r.info
	rec := &Record{Key: key, Investigation: &inv, Run: &info, Origin: "evaluation run " + info.Label() + " (read-only)"}
	if raw, ok := files[c+"/bundle.json"]; ok {
		var b domain.EvidenceBundle
		if err := json.Unmarshal(raw, &b); err == nil {
			rec.Bundle = &b
		}
	}
	return rec, nil
}

// Artifact implements Source: a stored run keeps no artifact store, but the
// files packed with the case (graph, transcript, …) are served when their
// digest matches.
func (s *RunsSource) Artifact(ctx context.Context, key, digest string) ([]byte, error) {
	runID, c, ok := strings.Cut(key, "/")
	if r := s.runs[runID]; ok && r != nil && validID(c) {
		files, err := readRunFiles(r.dir, c)
		if err != nil {
			return nil, err
		}
		for _, b := range files {
			if sum := sha256.Sum256(b); "sha256:"+hex.EncodeToString(sum[:]) == digest {
				return b, nil
			}
		}
	}
	return nil, fmt.Errorf("artifact %s: %w", digest, investigate.ErrNotFound)
}

// readRunFiles returns the files of a run's cases as "<case>/<file>" →
// bytes, from cases.tar.gz or the unpacked cases/ directory; only = "" reads
// every case, otherwise the one case.
func readRunFiles(dir, only string) (map[string][]byte, error) {
	out := map[string][]byte{}
	keep := func(name string) (string, bool) {
		rel, ok := strings.CutPrefix(path.Clean(name), "cases/")
		if !ok {
			return "", false
		}
		c, f, ok := strings.Cut(rel, "/")
		if !ok || strings.Contains(f, "/") || (only != "" && c != only) {
			return "", false
		}
		return rel, true
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "cases.tar.gz")); err == nil {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%s/cases.tar.gz: %w", dir, err)
		}
		tr := tar.NewReader(zr)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("%s/cases.tar.gz: %w", dir, err)
			}
			if h.Typeflag != tar.TypeReg {
				continue
			}
			rel, ok := keep(h.Name)
			if !ok {
				continue
			}
			b, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			out[rel] = b
		}
		return out, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	root := filepath.Join(dir, "cases")
	ents, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		if !e.IsDir() || (only != "" && e.Name() != only) {
			continue
		}
		fents, err := os.ReadDir(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, f := range fents {
			if f.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(root, e.Name(), f.Name()))
			if err != nil {
				return nil, err
			}
			out[e.Name()+"/"+f.Name()] = b
		}
	}
	return out, nil
}
