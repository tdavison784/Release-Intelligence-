package main

// Storage of scored investigation runs (eval/investigations/results/<run-id>/):
// run.json, score.json, gates.json and report.txt stay plain; the bulky
// per-case records live in one deterministic cases.tar.gz per run, read
// transparently by -rescore and the replay driver. The legacy layout (a
// cases/<id>/ directory) is still readable, and -pack converts it.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/eval"
)

// invCasesArchive holds cases/<id>/<file> entries; invScoreFile is the
// gzip-compressed score (decisions, metrics, splits, gates).
const (
	invCasesArchive = "cases.tar.gz"
	invScoreFile    = "score.json.gz"
)

func shortCommit(c string) string {
	if len(c) > 10 {
		return c[:10]
	}
	return c
}

func readInvRunMeta(dir string) (*InvRunMeta, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		return nil, err
	}
	var m InvRunMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s/run.json: %w", dir, err)
	}
	return &m, nil
}

// writeInvGzip writes a deterministic gzip file (no name, zero time).
func writeInvGzip(dst string, raw []byte) error {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(dst, buf.Bytes(), 0o644)
}

func readInvGzip(src string) ([]byte, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	return io.ReadAll(zr)
}

// readInvScore reads a run's score: score.json.gz, or a plain score.json.
func readInvScore(dir string) ([]byte, error) {
	if raw, err := readInvGzip(filepath.Join(dir, invScoreFile)); err == nil || !os.IsNotExist(err) {
		return raw, err
	}
	return os.ReadFile(filepath.Join(dir, "score.json"))
}

func invCasePath(caseID, name string) string { return path.Join("cases", caseID, name) }

func invFileBytes(v any) ([]byte, error) {
	if b, ok := v.([]byte); ok {
		return b, nil
	}
	raw, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// invGatesSummary is gates.json: the gate states, autonomy and run identity
// at a glance (derived from score.json and run.json).
func invGatesSummary(meta InvRunMeta, score *eval.InvScore) any {
	return map[string]any{"runId": meta.RunID, "arm": meta.Arm, "commit": meta.Commit, "dirty": meta.Dirty,
		"gates": score.Gates, "autonomy": score.Autonomy}
}

// writeInvArchive writes files as a deterministic tar.gz: sorted names, fixed
// modes, zero times, no owner names — the same records give the same bytes.
func writeInvArchive(dst string, files map[string][]byte) error {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(zw)
	for _, n := range names {
		hdr := &tar.Header{Name: n, Mode: 0o644, Size: int64(len(files[n])), ModTime: time.Unix(0, 0).UTC(), Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(files[n]); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(dst, buf.Bytes(), 0o644)
}

func readInvArchive(src string) (map[string][]byte, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	tr := tar.NewReader(zr)
	out := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", src, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", src, hdr.Name, err)
		}
		out[path.Clean(hdr.Name)] = raw
	}
}

// invRunFiles reads a stored run's per-case files from cases.tar.gz, or from
// the legacy cases/ directory.
type invRunFiles struct {
	dir  string
	arch map[string][]byte // nil = legacy directory layout
}

func openInvRunFiles(dir string) (*invRunFiles, error) {
	rf := &invRunFiles{dir: dir}
	a := filepath.Join(dir, invCasesArchive)
	if _, err := os.Stat(a); err == nil {
		m, err := readInvArchive(a)
		if err != nil {
			return nil, err
		}
		rf.arch = m
		return rf, nil
	}
	if _, err := os.Stat(filepath.Join(dir, "cases")); err != nil {
		return nil, fmt.Errorf("%s: no %s and no cases/ directory", dir, invCasesArchive)
	}
	return rf, nil
}

func (rf *invRunFiles) read(caseID, name string) ([]byte, error) {
	if rf.arch == nil {
		return os.ReadFile(filepath.Join(rf.dir, "cases", caseID, name))
	}
	raw, ok := rf.arch[invCasePath(caseID, name)]
	if !ok {
		return nil, fmt.Errorf("%s: %s: %w", invCasesArchive, invCasePath(caseID, name), os.ErrNotExist)
	}
	return raw, nil
}

// packInvRun converts a run stored in an earlier layout: every file under
// cases/ goes into cases.tar.gz and a plain score.json becomes score.json.gz,
// each read back and compared byte for byte before the original is removed;
// gates.json is derived from the score and run.json when missing. Nothing
// else in the run is touched. A run already in the current layout is an error.
func packInvRun(dir string) error {
	did := false
	root := filepath.Join(dir, "cases")
	if _, err := os.Stat(root); err == nil {
		if err := packInvCases(dir, root); err != nil {
			return err
		}
		did = true
	}
	plain := filepath.Join(dir, "score.json")
	if raw, err := os.ReadFile(plain); err == nil {
		dst := filepath.Join(dir, invScoreFile)
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("%s and %s both exist", plain, dst)
		}
		if err := writeInvGzip(dst, raw); err != nil {
			return err
		}
		back, err := readInvGzip(dst)
		if err != nil || !bytes.Equal(back, raw) {
			return fmt.Errorf("%s differs after compressing (%v)", dst, err)
		}
		if err := os.Remove(plain); err != nil {
			return err
		}
		did = true
	}
	if !did {
		return fmt.Errorf("%s: nothing to pack (already cases.tar.gz + score.json.gz)", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "gates.json")); os.IsNotExist(err) {
		meta, err := readInvRunMeta(dir)
		if err != nil {
			return err
		}
		sraw, err := readInvScore(dir)
		if err != nil {
			return err
		}
		var score eval.InvScore
		if err := json.Unmarshal(sraw, &score); err != nil {
			return fmt.Errorf("score: %w", err)
		}
		raw, err := invFileBytes(invGatesSummary(*meta, &score))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "gates.json"), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func packInvCases(dir, root string) error {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = raw
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%s: nothing to pack", root)
	}
	dst := filepath.Join(dir, invCasesArchive)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s exists", dst)
	}
	if err := writeInvArchive(dst, files); err != nil {
		return err
	}
	back, err := readInvArchive(dst)
	if err != nil {
		return err
	}
	if len(back) != len(files) {
		return fmt.Errorf("%s: %d entries read back, %d packed", dst, len(back), len(files))
	}
	for n, raw := range files {
		if !bytes.Equal(back[n], raw) {
			return fmt.Errorf("%s: %s differs after packing", dst, n)
		}
	}
	if !strings.HasSuffix(filepath.Clean(root), string(filepath.Separator)+"cases") {
		return fmt.Errorf("refusing to remove %s", root)
	}
	return os.RemoveAll(root)
}

// invMergeAppend folds the cases this invocation ran into a stored run: the
// run's earlier case records (archive bytes, unchanged) and their re-loaded
// InvRuns join the new ones, run.json gains the new cases and an Appends
// entry. It returns the merged meta, the earlier archive files and every case
// of the run (for scoring).
func (c *cli) invMergeAppend(datasetDir, runDir string, prior *InvRunMeta, cur InvRunMeta, runs map[string]*eval.InvRun, args []string) (InvRunMeta, map[string][]byte, []*eval.InvCase, error) {
	merged := *prior
	var newIDs []string
	for _, m := range cur.Cases {
		newIDs = append(newIDs, m.ID)
	}
	merged.Cases = append(append([]InvRunMetaCase{}, prior.Cases...), cur.Cases...)
	sort.Slice(merged.Cases, func(i, j int) bool { return merged.Cases[i].ID < merged.Cases[j].ID })
	merged.FinishedAt = cur.FinishedAt
	merged.Appends = append(append([]InvRunAppend{}, prior.Appends...), InvRunAppend{
		Args: append([]string{"eval", "-investigations"}, args...), Cases: newIDs, StartedAt: cur.StartedAt, FinishedAt: cur.FinishedAt})
	if merged.ConfigDigest == "" {
		merged.Config, merged.ConfigDigest = cur.Config, cur.ConfigDigest
	}
	rf, err := openInvRunFiles(runDir)
	if err != nil {
		return merged, nil, nil, err
	}
	if rf.arch == nil {
		return merged, nil, nil, fmt.Errorf("-append: run %s uses the legacy cases/ layout; -pack it first", prior.RunID)
	}
	var ids []string
	for _, m := range merged.Cases {
		ids = append(ids, m.ID)
	}
	all, err := eval.LoadInvCases(datasetDir, ids...)
	if err != nil {
		return merged, nil, nil, err
	}
	for _, m := range prior.Cases {
		if m.Error != "" {
			runs[m.ID] = &eval.InvRun{Case: m.ID, Error: m.Error}
			continue
		}
		s, err := loadInvStored(rf, m.ID)
		if err != nil {
			return merged, nil, nil, fmt.Errorf("%s: %w", m.ID, err)
		}
		runs[m.ID] = s.evalRun(m.ID)
	}
	return merged, rf.arch, all, nil
}
