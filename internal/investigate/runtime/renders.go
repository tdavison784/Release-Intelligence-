package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// Renderer re-renders one side of a bundle's environment render target to
// the RAW manifests a cluster can apply. The bundle stores normalized renders
// (empty maps dropped, credential-like values replaced by digest markers) —
// right for diffs and citations, wrong for kubectl apply — so the validator
// re-renders and proves the re-render normalizes to exactly the stored
// artifact before applying anything (verifyRender). The raw bytes are applied
// to the throwaway cluster and never stored.
type Renderer interface {
	RawRender(ctx context.Context, b *domain.EvidenceBundle, target, side string) (*RawRender, error)
}

// RawRender is one raw render.
type RawRender struct {
	Manifests []byte
	// Namespace is the release namespace: where a namespaced object that
	// names none is applied (what helm install -n / a GitOps destination do).
	Namespace string
	// Nondeterministic lists "<object>|<path>" entries that differ between
	// two renders of identical inputs (generated secrets, random names).
	Nondeterministic []string
}

// verifyRender checks that a raw render normalizes to the stored render
// artifact (render.ParseObjects normalizes exactly as the renders were when
// the bundle stored them): same objects, same normalized bodies, except
// objects with nondeterministic content.
func verifyRender(ctx context.Context, st investigate.ArtifactStore, ref domain.ArtifactRef, raw *RawRender) error {
	rc, err := st.Open(ctx, ref.Digest)
	if err != nil {
		return fmt.Errorf("stored render %s (%s): %w", ref.ID, ref.Path, err)
	}
	defer rc.Close()
	stored, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("stored render %s: %w", ref.ID, err)
	}
	if got := domain.Digest(stored); got != ref.Digest {
		return fmt.Errorf("stored render %s: digest %s, manifest says %s", ref.ID, got, ref.Digest)
	}
	want, err := storedObjects(stored)
	if err != nil {
		return fmt.Errorf("stored render %s: %w", ref.ID, err)
	}
	objs, err := render.ParseObjects(raw.Manifests)
	if err != nil {
		return fmt.Errorf("re-render: %w", err)
	}
	nondet := map[string]bool{}
	for _, n := range raw.Nondeterministic {
		obj, _, _ := strings.Cut(n, "|")
		nondet[obj] = true
	}
	got := map[string]string{}
	for _, o := range objs {
		b, _ := json.Marshal(o.Body)
		got[o.ID.String()] = string(b)
	}
	var diffs []string
	for id, body := range want {
		g, ok := got[id]
		switch {
		case !ok:
			diffs = append(diffs, id+" is missing from the re-render")
		case g != body && !nondet[id]:
			diffs = append(diffs, id+" differs")
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			diffs = append(diffs, id+" is not in the stored render")
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		if len(diffs) > 5 {
			diffs = append(diffs[:5], fmt.Sprintf("… %d more", len(diffs)-5))
		}
		return fmt.Errorf("the re-render does not reproduce stored render %s (%s): %s", ref.ID, ref.Path, strings.Join(diffs, "; "))
	}
	return nil
}

// storedObjects parses a stored render stream ("---\n# Object: <id>\n
// [# Source: …\n]<yaml>") into normalized bodies (canonical JSON) by id.
func storedObjects(stream []byte) (map[string]string, error) {
	out := map[string]string{}
	var id string
	var doc bytes.Buffer
	flush := func() error {
		if id == "" {
			return nil
		}
		var body any
		if err := yaml.Unmarshal(doc.Bytes(), &body); err != nil {
			return fmt.Errorf("object %s: %w", id, err)
		}
		if body == nil {
			body = map[string]any{}
		}
		b, _ := json.Marshal(body)
		out[id] = string(b)
		id = ""
		doc.Reset()
		return nil
	}
	sc := bufio.NewScanner(bytes.NewReader(stream))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "---":
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		case id == "" && strings.HasPrefix(line, "# Object: "):
			id = strings.TrimPrefix(line, "# Object: ")
			continue
		case id != "" && doc.Len() == 0 && strings.HasPrefix(line, "# Source: "):
			continue
		}
		if id != "" {
			doc.WriteString(line + "\n")
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, flush()
}
