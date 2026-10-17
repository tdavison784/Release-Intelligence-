package bundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/redaction"
)

// SnapshotProducer identifies the snapshot builder in provenance. v2: files
// and facts are redacted with redaction@v2 (redact.go).
const SnapshotProducer = "bundle.snapshot@v2"

// maxFileBytes bounds one collected environment file; a larger file is a
// recorded gap, never a silent truncation.
const maxFileBytes = 8 << 20

// SnapshotBuilder builds the environment snapshot over env.Load (MISSION §A2).
type SnapshotBuilder struct {
	st    investigate.ArtifactStore
	hints []env.ProductHint
	now   func() time.Time
	// kbuild builds kustomizations at snapshot time (default: kustomize).
	kbuild kustomizeBuilder
}

var _ investigate.SnapshotBuilder = (*SnapshotBuilder)(nil)

// NewSnapshotBuilder returns the snapshot builder writing artifacts to st.
func NewSnapshotBuilder(st investigate.ArtifactStore) investigate.SnapshotBuilder {
	return &SnapshotBuilder{st: st, now: pinnedClock, kbuild: realKustomizeBuilder}
}

// NewSnapshotBuilderWithHints is NewSnapshotBuilder with the catalog's product
// hints (env.HintsFromCatalog), so images are recognised as catalog products.
func NewSnapshotBuilderWithHints(st investigate.ArtifactStore, hints []env.ProductHint) investigate.SnapshotBuilder {
	return &SnapshotBuilder{st: st, hints: hints, now: pinnedClock, kbuild: realKustomizeBuilder}
}

// BuildSnapshot loads the environment and stores its files (redacted) and
// parsed facts. An input it cannot fully read is recorded with a non-ok
// status; only an environment nothing can be read from is an error.
func (b *SnapshotBuilder) BuildSnapshot(ctx context.Context, in investigate.EnvironmentInputs) (*investigate.SnapshotResult, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, errors.New("snapshot: the environment name is required")
	}
	ei := env.Inputs{
		Repo: in.Repo, ValuesFiles: in.ValuesFiles, Manifests: in.Manifests, CRDs: in.CRDs, Images: in.Images,
		Inventory: in.Inventory, KubernetesVersion: in.KubernetesVersion, ProductHints: b.hints,
		Product: in.Product, // values are per release: those deploying the product decide
	}
	if ei.Empty() {
		return nil, errors.New("snapshot: no environment input was given (repo, values, manifests, crds, images, inventory or kubernetes version)")
	}
	e, err := env.Load(ei)
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}

	e.Warnings = resolveBuilt(ctx, in.Repo, e.Warnings, b.kbuild)
	norm := newNormalizer(in.Repo)
	e.Warnings = norm.lines(e.Warnings)
	idMap := norm.evidence(e.Evidence)
	gapWarnings, notes := splitWarnings(e.Warnings, in.Product)

	// 1. Files → artifacts (redacted before hashing).
	type collected struct {
		file    env.File
		ref     domain.ArtifactRef
		problem string
	}
	var files []collected
	var allRefs []domain.ArtifactRef
	usedPaths := map[string]string{} // logical path → digest
	for _, f := range e.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c := collected{file: f}
		data, rerr := readBounded(f.Path)
		if rerr != nil {
			c.problem = rerr.Error()
			files = append(files, c)
			continue
		}
		redactedFile := false
		if redactable(f.Path) {
			data, redactedFile = redactYAML(data)
		}
		path := logicalPath(in.Repo, f.Path)
		sum := sha256.Sum256(data)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		if d, dup := usedPaths[path]; dup && d != digest {
			path = path + "." + hex.EncodeToString(sum[:])[:8]
		}
		usedPaths[path] = digest
		kind := domain.ArtifactEnvironmentFile
		if in.Inventory != "" && samePath(f.Path, in.Inventory) {
			kind = domain.ArtifactInventory
		}
		ref, perr := b.st.Put(ctx, investigate.ArtifactSpec{
			Kind: kind, Path: path, Origin: domain.OriginEnvironment, MediaType: mediaTypeOf(f.Path), URI: norm.path(f.Path),
			Producer: SnapshotProducer, Redacted: redactedFile, Tags: envFileTags(norm.path(f.Path)),
		}, data)
		if perr != nil {
			return nil, fmt.Errorf("snapshot: %w", perr)
		}
		c.ref = ref
		files = append(files, c)
		allRefs = append(allRefs, ref)
	}

	// 2. Facts artifact: the parsed env.Environment, secrets withheld.
	facts, err := redactedFacts(e)
	if err != nil {
		return nil, fmt.Errorf("snapshot: facts: %w", err)
	}
	facts = norm.facts(facts, idMap)
	factsRef, err := b.st.Put(ctx, investigate.ArtifactSpec{
		Kind: domain.ArtifactEnvironmentFacts, Path: "derived/snapshot/environment-facts.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:" + SnapshotProducer, Producer: SnapshotProducer, Redacted: true,
		Tags: []string{"environment", "facts", strings.ToLower(in.Name)},
	}, facts)
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	allRefs = append(allRefs, factsRef)

	// 3. Inputs, each with its collection status.
	problems := map[string]string{} // file path → why it is not fully collected
	for _, c := range files {
		if c.problem != "" {
			problems[c.file.Path] = norm.text(c.problem)
		}
	}
	statusOf := func(dim string) (domain.CollectionStatus, string) {
		for _, s := range e.Statuses() {
			if s.Dimension != dim {
				continue
			}
			switch s.Health {
			case env.HealthPartial:
				return domain.CollectionPartial, norm.text(firstN(s.Warnings, 3, "the loader reported problems"))
			case env.HealthAbsent:
				return domain.CollectionUnavailable, "the loader found nothing usable in this input"
			}
		}
		return domain.CollectionOK, ""
	}
	artifactsUnder := func(p string) ([]string, string) {
		var ids, probs []string
		for _, c := range files {
			if !(samePath(c.file.Path, p) || underPath(c.file.Path, p)) {
				continue
			}
			if c.problem != "" {
				probs = append(probs, norm.text(c.problem))
				continue
			}
			ids = append(ids, c.ref.ID)
		}
		return sortedUniq(ids), firstN(probs, 3, "")
	}
	var inputs []domain.SnapshotInput
	add := func(kind domain.SnapshotInputKind, path, value, dim string) {
		si := domain.SnapshotInput{Kind: kind, Path: path, Value: value}
		si.Status, si.Reason = statusOf(dim)
		if path != "" {
			si.Artifacts, _ = artifactsUnder(path)
			if _, probs := artifactsUnder(path); probs != "" {
				si.Status, si.Reason = domain.CollectionPartial, joinReason(si.Reason, probs)
			}
			if len(si.Artifacts) == 0 && si.Status == domain.CollectionOK {
				si.Status, si.Reason = domain.CollectionPartial, "no file of this input was collected"
			}
		}
		si.Path = norm.input(path)
		inputs = append(inputs, si)
	}
	if in.Repo != "" {
		si := domain.SnapshotInput{Kind: domain.InputGitOpsRepo, Path: ".", Status: domain.CollectionOK}
		si.Artifacts, _ = artifactsUnder(in.Repo)
		if len(gapWarnings) > 0 {
			si.Status, si.Reason = domain.CollectionPartial, fmt.Sprintf("%d loader warning(s): %s", len(gapWarnings), firstN(gapWarnings, 3, ""))
		}
		if _, probs := artifactsUnder(in.Repo); probs != "" {
			si.Status, si.Reason = domain.CollectionPartial, joinReason(si.Reason, probs)
		}
		if len(si.Artifacts) == 0 {
			si.Status, si.Reason = domain.CollectionPartial, joinReason(si.Reason, "no values or manifest file was found in the repository")
		}
		if len(notes) > 0 && si.Status == domain.CollectionOK {
			// Recorded, not a gap: it concerns an input that does not decide the item's family.
			si.Reason = "note: " + firstN(notes, 3, "")
		}
		inputs = append(inputs, si)
	}
	for _, p := range in.ValuesFiles {
		add(domain.InputValues, p, "", env.DimValues)
	}
	for _, p := range in.Manifests {
		add(domain.InputManifests, p, "", env.DimManifests)
	}
	for _, p := range in.CRDs {
		add(domain.InputCRDs, p, "", env.DimCRDs)
	}
	for _, p := range in.Images {
		si := domain.SnapshotInput{Kind: domain.InputImages, Path: norm.input(p)}
		si.Status, si.Reason = statusOf(env.DimImages)
		si.Artifacts, _ = artifactsUnder(p) // an image reference, not a file, has none
		inputs = append(inputs, si)
	}
	if in.Inventory != "" {
		add(domain.InputInventory, in.Inventory, "", env.DimProducts)
	}
	if v := strings.TrimSpace(in.KubernetesVersion); v != "" {
		inputs = append(inputs, domain.SnapshotInput{Kind: domain.InputClusterVersion, Value: v, Status: domain.CollectionOK})
	}
	if in.Live != nil {
		inputs = append(inputs, domain.SnapshotInput{Kind: domain.InputLiveCluster, Value: in.Live.Context, Status: domain.CollectionUnavailable,
			Reason: "live observation is not implemented in milestone 1 (DESIGN.md §1.2): the request is rejected"})
	}
	sort.SliceStable(inputs, func(i, j int) bool {
		a, c := inputs[i], inputs[j]
		if a.Kind != c.Kind {
			return a.Kind < c.Kind
		}
		return a.Path+a.Value < c.Path+c.Value
	})

	// 4. The snapshot.
	snap := domain.EnvironmentSnapshot{
		Environment: in.Name, KubernetesVersion: strings.TrimSpace(in.KubernetesVersion), Inputs: inputs,
		Facts: factsRef.ID, Producer: SnapshotProducer, CollectedAt: b.now().UTC().Truncate(time.Second),
	}
	var prov *investigate.RepoProvenance
	if in.Repo != "" {
		var fd []fileDigest
		for _, c := range files {
			if c.problem == "" && underPath(c.file.Path, in.Repo) {
				fd = append(fd, fileDigest{Path: c.ref.Path, Digest: c.ref.Digest})
			}
		}
		snap.Repo = repoRef(ctx, in.Repo, fd)
		prov = &investigate.RepoProvenance{Path: in.Repo, Commit: snap.Repo.Commit, Dirty: snap.Repo.Dirty, URL: snap.Repo.URL}
		// The enclosing commit, the dirty flag, the remote and the checkout path are
		// provenance, not identity: the same environment content in another checkout
		// or at another commit is the same snapshot (Principle 6).
		snap.Repo.Path, snap.Repo.Commit, snap.Repo.Dirty, snap.Repo.URL = ".", "", false, ""
	}
	snap.ID = snap.DerivedID()
	if err := snap.Validate(); err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}

	evidence := redactEvidence(e.Evidence)
	sort.Slice(allRefs, func(i, j int) bool { return allRefs[i].Path < allRefs[j].Path })
	local := &investigate.LocalInputs{Repo: in.Repo}
	for _, p := range in.ValuesFiles {
		local.ValuesFiles = append(local.ValuesFiles, p)
	}
	return &investigate.SnapshotResult{Snapshot: snap, Artifacts: dedupeRefs(allRefs), Evidence: evidence, Provenance: prov, Local: local}, nil
}

// logicalPath maps an environment file to its logical bundle path: relative
// to the repository when it lies below it ("env/<path as in the repo>"),
// otherwise below env/inputs/.
func logicalPath(repo, file string) string {
	if repo != "" {
		if rel, err := filepath.Rel(repo, file); err == nil && !strings.HasPrefix(rel, "..") {
			return "env/" + filepath.ToSlash(rel)
		}
	}
	return "env/inputs/" + filepath.Base(file)
}

func samePath(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }

func underPath(file, dir string) bool {
	rel, err := filepath.Rel(dir, file)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

func readBounded(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if fi.Size() > maxFileBytes {
		return nil, fmt.Errorf("%s: %d bytes exceeds the %d byte collection bound", path, fi.Size(), maxFileBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return data, nil
}

func mediaTypeOf(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml":
		return "application/yaml"
	case ".json":
		return "application/json"
	case ".md":
		return "text/markdown"
	}
	return "text/plain"
}

func envFileTags(path string) []string {
	tags := []string{"environment", strings.ToLower(filepath.Base(path))}
	for _, seg := range strings.Split(filepath.ToSlash(filepath.Dir(path)), "/") {
		if seg != "" && seg != "." {
			tags = append(tags, strings.ToLower(seg))
		}
	}
	return tags
}

func firstN(in []string, n int, empty string) string {
	if len(in) == 0 {
		return empty
	}
	out := in
	if len(out) > n {
		out = out[:n]
	}
	s := strings.Join(out, "; ")
	if len(in) > n {
		s += fmt.Sprintf("; … (%d more)", len(in)-n)
	}
	return s
}

func joinReason(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

func sortedUniq(in []string) []string {
	m := map[string]bool{}
	var out []string
	for _, s := range in {
		if !m[s] {
			m[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func dedupeRefs(in []domain.ArtifactRef) []domain.ArtifactRef {
	seen := map[string]bool{}
	var out []domain.ArtifactRef
	for _, r := range in {
		if !seen[r.ID] {
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	return out
}

// redactedFacts marshals the parsed environment with credential values
// withheld by the rule of the files (package redaction): a ValuesKey or
// field fact loses its Value (and a text block its lines) when the rule
// withholds the value at its path, and an evidence excerpt that assigns a
// credential-named key is replaced.
func redactedFacts(e *env.Environment) ([]byte, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	walkFacts(tree, false)
	return json.MarshalIndent(tree, "", "  ")
}

// walkFacts applies the rule to every fact below n; crd: n belongs to a
// CustomResourceDefinition resource (only private keys are withheld).
func walkFacts(n any, crd bool) {
	switch v := n.(type) {
	case map[string]any:
		if k, _ := v["Kind"].(string); k != "" {
			g, _ := v["Group"].(string)
			crd = crd || redaction.CRDDocument(g+"/", k)
		}
		if p, ok := v["Path"].(string); ok {
			if val, ok := v["Value"].(string); ok && val != "" && withheldFact(p, val, crd) {
				v["Value"] = redactedValue(val)
			}
			if lines, ok := v["Lines"].([]any); ok && !crd && withheldPath(p) {
				for _, l := range lines {
					if lm, ok := l.(map[string]any); ok {
						if t, _ := lm["Text"].(string); t != "" {
							lm["Text"], lm["Withheld"] = "", true
						}
					}
				}
			}
		}
		for _, k := range []string{"excerpt", "Excerpt"} { // domain.Evidence is tagged, env facts are not
			if ex, ok := v[k].(string); ok && ex != "" && sensitiveKey.MatchString(ex) {
				v[k] = "<redacted: credential-like content>"
			}
		}
		for _, c := range v {
			walkFacts(c, crd)
		}
	case []any:
		if !crd {
			withholdCredentialEnv(v)
		}
		for _, c := range v {
			walkFacts(c, crd)
		}
	}
}

// withholdCredentialEnv applies rule (e) to a list of facts: the value of an
// environment variable (…[i].value) whose sibling …[i].name names a
// credential.
func withholdCredentialEnv(facts []any) {
	names := map[string]bool{}
	loc := func(m map[string]any) string {
		if el, _ := m["Element"].(string); el != "" {
			return el
		}
		p, _ := m["Path"].(string)
		return p
	}
	for _, f := range facts {
		m, ok := f.(map[string]any)
		if !ok {
			return
		}
		if prefix, ok := strings.CutSuffix(loc(m), ".name"); ok {
			var name string
			if val, _ := m["Value"].(string); json.Unmarshal([]byte(val), &name) == nil && redaction.CredentialEnvVar(name) {
				names[prefix] = true
			}
		}
	}
	for _, f := range facts {
		m := f.(map[string]any)
		if prefix, ok := strings.CutSuffix(loc(m), ".value"); ok && names[prefix] {
			if val, _ := m["Value"].(string); val != "" {
				m["Value"] = redactedValue(val)
			}
		}
	}
}

// pathKeys splits a fact path into its own key and whether a key above it
// names a credential and holds a mapping or list (rule d).
func pathKeys(path string) (own string, under bool) {
	segs := strings.Split(path, ".")
	for i, seg := range segs {
		name, _, _ := strings.Cut(seg, "[")
		if i < len(segs)-1 && redaction.CredentialContainer(name) {
			under = true
		}
		own = name // a list item's key is the list's
	}
	return own, under
}

// withheldFact applies the rule to a fact's path and JSON-encoded value.
func withheldFact(path, encoded string, crd bool) bool {
	var v any
	if err := json.Unmarshal([]byte(encoded), &v); err != nil {
		return true // not a JSON leaf: nothing proves it harmless
	}
	str, _ := v.(string)
	if crd {
		return redaction.HoldsPrivateKey(str)
	}
	own, under := pathKeys(path)
	return !redaction.Keep(own, redaction.KindOf(v), str, under)
}

// withheldPath reports whether the rule withholds a string at path.
func withheldPath(path string) bool {
	own, under := pathKeys(path)
	return !redaction.Keep(own, redaction.String, "", under)
}

// redactEvidence returns the evidence pool, sorted by id, with credential-like
// excerpts replaced. Ids are kept, so every fact still resolves.
func redactEvidence(in []domain.Evidence) []domain.Evidence {
	out := make([]domain.Evidence, len(in))
	copy(out, in)
	for i := range out {
		if out[i].Excerpt != "" && sensitiveKey.MatchString(out[i].Excerpt) {
			out[i].Excerpt = "<redacted: credential-like content>"
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// NewSnapshotBuilderWithClock is NewSnapshotBuilderWithHints with CollectedAt
// taken from now (see pinnedClock for why the default is the zero time).
func NewSnapshotBuilderWithClock(st investigate.ArtifactStore, hints []env.ProductHint, now func() time.Time) investigate.SnapshotBuilder {
	return &SnapshotBuilder{st: st, hints: hints, now: now, kbuild: realKustomizeBuilder}
}
