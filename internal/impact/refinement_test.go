package impact

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// A scope-refinement rule (lane verify2, D-0025) under the impact join: the
// behaviour rule "a Badge that leaves keyPolicy unset is exposed" narrowed to
// the signer role. The role field joins the same resource scope as the
// behaviour condition, so both speak of one object: an environment whose
// signer pins keyPolicy and whose non-signers leave it unset is not exposed;
// one whose signer leaves it unset is. Without an evidenced default an object
// that leaves spec.signer unset keeps the rule undecidable.
const refinementBadges = `apiVersion: acme.example/v1
kind: Badge
metadata: {name: signer, namespace: ns}
spec: {secretName: signer-tls, signer: true, keyPolicy: Keep}
---
apiVersion: acme.example/v1
kind: Badge
metadata: {name: leaf, namespace: ns}
spec: {secretName: leaf-tls}
---
apiVersion: acme.example/v1
kind: Badge
metadata: {name: leaf2, namespace: ns}
spec: {secretName: leaf2-tls, signer: false}
`

const exposedSigner = `apiVersion: acme.example/v1
kind: Badge
metadata: {name: signer, namespace: ns}
spec: {secretName: signer-tls, signer: true}
`

func TestRefinedRuleSpeaksOfOneObject(t *testing.T) {
	edge := newEdge().edge
	pool := ConditionOptions{Evidence: []domain.Evidence{upstreamDoc}}
	unset := field("spec.keyPolicy", domain.StateUnset)
	signer := field("spec.signer", domain.StateEquals, "true")
	refined := func(def bool) domain.Condition {
		s := signer
		if def {
			s = withDefault(signer, "false", domain.DefaultController, upstreamDoc.ID)
		}
		return badgeScope("", unset, s)
	}
	pinnedSigner := condEnv(t, "", map[string]string{"badges.yaml": refinementBadges})
	unpinnedSigner := condEnv(t, "", map[string]string{"badges.yaml": exposedSigner})
	for name, tc := range map[string]struct {
		c    domain.Condition
		e    *env.Environment
		want Truth
	}{
		"signer pinned, non-signers unset, evidenced default: not exposed": {refined(true), pinnedSigner, False},
		"signer pinned, a leaf omits spec.signer, no default: undecidable": {refined(false), pinnedSigner, Unknown},
		"signer leaves keyPolicy unset: exposed":                           {refined(true), unpinnedSigner, True},
		"signer leaves keyPolicy unset, no default: exposed":               {refined(false), unpinnedSigner, True},
		// two scopes would pair different objects (the pinned signer and an
		// unset leaf): the shape the refinement stage must not write
		"two scopes pair different objects": {domain.Condition{Op: domain.OpAll, Of: []domain.Condition{badgeScope("", unset),
			badgeScope("", signer)}}, pinnedSigner, True},
	} {
		if r := EvaluateConditionWith(tc.c, tc.e, edge, pool); r.Value != tc.want {
			t.Errorf("%s: %s", name, r.Value)
		}
	}
}
