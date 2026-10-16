package bundle

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// normalizer rewrites checkout-specific absolute paths to repo-relative ones
// so a snapshot (and the bundle around it) is identified by the content of
// the environment, not by where it was checked out (Principle 6).
type normalizer struct {
	root string
	rep  *strings.Replacer
}

func newNormalizer(repo string) *normalizer {
	n := &normalizer{}
	if repo == "" {
		return n
	}
	root, err := filepath.Abs(repo)
	if err != nil {
		return n
	}
	n.root = root
	roots := []string{root}
	if r, err := filepath.EvalSymlinks(root); err == nil && r != root {
		roots = append(roots, r)
	}
	var pairs []string
	for _, r := range roots {
		pairs = append(pairs, r+string(filepath.Separator), "")
	}
	for _, r := range roots {
		pairs = append(pairs, r, ".")
	}
	n.rep = strings.NewReplacer(pairs...)
	return n
}

// text rewrites absolute repo paths inside free text.
func (n *normalizer) text(s string) string {
	if n.rep == nil {
		return s
	}
	return n.rep.Replace(s)
}

func (n *normalizer) lines(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = n.text(s)
	}
	return out
}

// path maps a file to its repo-relative form ("clusters/prod/x.yaml"); a file
// outside the repository keeps its path.
func (n *normalizer) path(p string) string {
	if n.rep == nil {
		return p
	}
	return n.text(p)
}

// input maps an input path to its logical form: "env/<rel>" below the repo,
// "env/inputs/<name>" elsewhere (the artifact path scheme).
func (n *normalizer) input(p string) string {
	if n.root != "" {
		return logicalPath(n.root, p)
	}
	return p
}

// evidence rewrites evidence URIs to repo-relative ones in place and
// returns the old→new id map (the id derives from the URI).
func (n *normalizer) evidence(ev []domain.Evidence) map[domain.EvidenceID]domain.EvidenceID {
	m := map[domain.EvidenceID]domain.EvidenceID{}
	if n.rep == nil {
		return m
	}
	for i := range ev {
		u := n.text(ev[i].URI)
		if u == ev[i].URI {
			continue
		}
		old := ev[i].ID
		ev[i].URI = u
		ev[i].ID = domain.EvidenceID("ev-" + domain.ShortHash(string(ev[i].Kind), ev[i].URI, ev[i].Locator, ev[i].Excerpt, ev[i].ContentDigest))
		m[old] = ev[i].ID
	}
	return m
}

// facts rewrites the facts JSON: absolute repo paths and remapped evidence ids.
func (n *normalizer) facts(b []byte, ids map[domain.EvidenceID]domain.EvidenceID) []byte {
	if n.rep == nil {
		return b
	}
	s := n.text(string(b))
	for old, nw := range ids {
		s = strings.ReplaceAll(s, string(old), string(nw))
	}
	return []byte(s)
}

var unparsableRE = regexp.MustCompile(`^.+ "([^"]+)": version "[^"]*" is not a parsable version`)

// splitWarnings separates the loader warnings that are evidence gaps from
// those that are recorded notes. A non-semver version of ANOTHER product's
// deployment (an Application that does not deploy the product under
// investigation) is a note: it does not decide the item's family. The
// product's own deployment tracking a branch leaves its deployed version
// unknown: the gap stays. Without a product every warning is a gap (fail
// closed).
func splitWarnings(ws []string, product string) (gaps, notes []string) {
	for _, w := range ws {
		m := unparsableRE.FindStringSubmatch(w)
		if m != nil && product != "" && !strings.EqualFold(m[1], product) {
			notes = append(notes, w)
			continue
		}
		gaps = append(gaps, w)
	}
	return gaps, notes
}
