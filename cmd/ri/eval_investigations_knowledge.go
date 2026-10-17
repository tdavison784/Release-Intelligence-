package main

// Knowledge pinning for `ri eval -investigations` (PROTOCOL.md §3): every
// scored run names the knowledge snapshot it ran with by content digest, and
// refuses the live knowledge/ tree unless that is asked for explicitly, so a
// later knowledge change can never silently contaminate an arm.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/eval"
)

// noKnowledgeDigest is KnowledgeDigest of a run with knowledge switched off
// (-knowledge ""), distinguishable from any tree digest.
const noKnowledgeDigest = "none"

// knowledgeTreeDigest is a content digest of a knowledge directory: sha256
// over the sorted slash-separated relative path and bytes of every regular
// file (not only the *.json records the loader reads, so a stray file shows
// up too). Empty dirs and modes do not count; a symlinked file is read
// through.
func knowledgeTreeDigest(dir string) (string, error) {
	if dir == "" {
		return noKnowledgeDigest, nil
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("knowledge %q: %w", dir, err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("knowledge %q: not a directory", dir)
	}
	var files []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, p := range files {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return "", err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(raw))
		h.Write(raw)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// liveKnowledgeRoots are the directories that count as "the live knowledge/
// tree": ./knowledge, the work tree's knowledge/ and the primary checkout's.
func liveKnowledgeRoots() []string {
	roots := []string{"knowledge"}
	git := func(args ...string) string {
		out, err := exec.Command("git", args...).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	if top := git("rev-parse", "--show-toplevel"); top != "" {
		roots = append(roots, filepath.Join(top, "knowledge"))
	}
	if common := git("rev-parse", "--path-format=absolute", "--git-common-dir"); common != "" {
		roots = append(roots, filepath.Join(filepath.Dir(common), "knowledge"))
	}
	return roots
}

func samePath(a, b string) bool {
	canon := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return filepath.Clean(p)
	}
	return canon(a) == canon(b)
}

// checkEvalKnowledge refuses an investigation eval whose knowledge is not an
// explicit choice, or is the live knowledge tree without allowLive.
func checkEvalKnowledge(explicit bool, dir string, allowLive bool, liveRoots []string) error {
	if allowLive {
		return nil
	}
	if !explicit {
		return fmt.Errorf("%w: -knowledge is required (a snapshot directory, e.g. from scripts/phase5/snapshot.sh); the default live knowledge/ tree would silently contaminate a scored run — pass -allow-live-knowledge to use it deliberately", app.ErrUsage)
	}
	if dir == "" {
		return nil // knowledge switched off explicitly
	}
	for _, r := range liveRoots {
		if samePath(dir, r) {
			return fmt.Errorf("%w: -knowledge %s is the live knowledge/ tree; use a snapshot (scripts/phase5/snapshot.sh) or pass -allow-live-knowledge", app.ErrUsage, dir)
		}
	}
	return nil
}

// agentVerdictInputs extracts the agent's provisional verdicts from a stored
// investigation for scoring. STUB: PROTOCOL §8 Investigation.AgentVerdicts
// is added by lane `verdict`; until that merges there is nothing to read, so
// arms store no verdicts and the verdict table stays empty. Replace the body
// with a loop over inv.AgentVerdicts (item, class, confidence, mechanism).
func agentVerdictInputs(s *invStored) []eval.VerdictInput { return nil }

func samePathAny(dir string, roots []string) bool {
	for _, r := range roots {
		if samePath(dir, r) {
			return true
		}
	}
	return false
}
