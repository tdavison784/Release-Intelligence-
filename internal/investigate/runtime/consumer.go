package runtime

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Consumer observation of a fixture run (lane verify2; PROTOCOL amendment
// 2026-10-17 "B0-v8 outcome"). Whether an object's data changed says nothing
// about whether anything that consumes it notices: a consumer that verifies
// a certificate through its issuer does not observe the certificate's own key
// changing, but does observe its issuer's key changing. A ConsumerCheck plays
// that consumer at each version, right after the version's baseline
// observation: it captures the certificates in Trust, re-issues the Reissue
// objects one after another with a declared exercise (each answered before
// the next), and then verifies the certificate chain in Verify against the
// captured certificates with crypto/x509, in process.
//
// Only PEM CERTIFICATE blocks are read (a declared key holding anything else
// is refused unread past its PEM type), and only the outcome and
// certificate / public-key digests are recorded: no key material is handled
// or stored. Secret data in observations stays HMAC-digested as before.

// ConsumerCheck is one declared consumer observation.
type ConsumerCheck struct {
	ID string `json:"id"`
	// Exercise is the declared exercise ("<product>/<id>") whose action
	// re-issues an object; its reaction must be a field change, awaited per
	// object.
	Exercise string    `json:"exercise"`
	Trust    SecretKey `json:"trust"`
	// Reissue are object ids, re-issued in this order.
	Reissue []string  `json:"reissue"`
	Verify  SecretKey `json:"verify"`
}

// SecretKey is one key of a Secret ("core/v1/Secret/<ns>/<name>").
type SecretKey struct {
	Secret string `json:"secret"`
	Key    string `json:"key"`
}

// Consumer outcomes.
const (
	ConsumerVerified = "verified"
	// ConsumerFailed: the chain does not verify against the captured trust
	// (x509: unknown authority) — the consumer observes the change.
	ConsumerFailed = "failed"
	// ConsumerUndetermined: the check could not decide (an object was not
	// re-issued, a Secret or certificate is missing, any other x509 error).
	ConsumerUndetermined = "undetermined"
)

// ConsumerResult is one check at one version.
type ConsumerResult struct {
	Check   string `json:"check"`
	Phase   string `json:"phase"` // "from" | "to"
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
	// Reissued are the objects re-issued and answered, in order;
	// Unanswered the first one that was not (the check stops there).
	Reissued   []string `json:"reissued,omitempty"`
	Unanswered []string `json:"unanswered,omitempty"`
	// Digests (sha256) of the captured trust certificates and their public
	// keys, and of the verified certificate and its public key.
	TrustCertificates []string `json:"trustCertificates,omitempty"`
	TrustKeys         []string `json:"trustKeys,omitempty"`
	Certificate       string   `json:"certificate,omitempty"`
	CertificateKey    string   `json:"certificateKey,omitempty"`
}

// consumerPoll is how often a re-issued object is checked for its reaction.
const consumerPoll = 3 * time.Second

func validateConsumers(cs []ConsumerCheck, exercises []DeclaredExercise) error {
	var errs []error
	seen := map[string]bool{}
	for _, c := range cs {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("consumer check %q: %s", c.ID, fmt.Sprintf(format, args...)))
		}
		if c.ID == "" || seen[c.ID] {
			bad("needs a unique id")
		}
		seen[c.ID] = true
		var ex *DeclaredExercise
		for i := range exercises {
			if exercises[i].Key() == c.Exercise {
				ex = &exercises[i]
			}
		}
		switch {
		case ex == nil:
			bad("exercise %s is not declared", c.Exercise)
		case ex.Exercise.Reaction == nil || ex.Exercise.Reaction.Kind != catalog.ReactionFieldChanged:
			bad("exercise %s declares no field-change reaction to await per object", c.Exercise)
		}
		if len(c.Reissue) == 0 {
			bad("names no object to re-issue")
		}
		for _, id := range c.Reissue {
			if r, ok := ParseRef(id); !ok || (ex != nil && !ex.targets(r)) {
				bad("re-issue %q is not an object id the exercise targets", id)
			}
		}
		for _, sk := range []SecretKey{c.Trust, c.Verify} {
			if r, ok := ParseRef(sk.Secret); !ok || r.Group != "" || r.Kind != "Secret" || sk.Key == "" {
				bad("%q/%q is not a Secret object id and key", sk.Secret, sk.Key)
			}
		}
	}
	return errors.Join(errs...)
}

// checkConsumers runs every consumer check at one version.
func (s *session) checkConsumers(ctx context.Context, cl Cluster, version string) error {
	for _, c := range s.consumers {
		r := s.consumerCheck(ctx, cl, c, version)
		s.run.Consumers = append(s.run.Consumers, r)
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) consumerCheck(ctx context.Context, cl Cluster, c ConsumerCheck, version string) ConsumerResult {
	r := ConsumerResult{Check: c.ID, Phase: version, Outcome: ConsumerUndetermined}
	var d DeclaredExercise
	for _, x := range s.v.exercises {
		if x.Key() == c.Exercise {
			d = x
		}
	}
	trust, err := readCertificates(ctx, cl, c.Trust)
	if err != nil {
		r.Detail = "trust bundle: " + err.Error()
		return r
	}
	pool := x509.NewCertPool()
	for _, t := range trust {
		pool.AddCert(t)
		r.TrustCertificates = append(r.TrustCertificates, domain.Digest(t.Raw))
		r.TrustKeys = append(r.TrustKeys, domain.Digest(t.RawSubjectPublicKeyInfo))
	}
	settle := time.Duration(d.Exercise.SettleSeconds) * time.Second
	if settle == 0 {
		settle = s.v.settle
	}
	for i, id := range c.Reissue {
		token := s.clusterPrefix() + version + "-" + domain.ShortHash(s.b.ID, version, c.ID, fmt.Sprint(i))[:8]
		answered, why := s.reissue(ctx, cl, d, id, token, settle)
		if !answered {
			r.Unanswered = []string{id}
			r.Detail = fmt.Sprintf("%s was not re-issued (%s): the consumer observation cannot be made", id, why)
			return r
		}
		r.Reissued = append(r.Reissued, id)
	}
	chain, err := readCertificates(ctx, cl, c.Verify)
	if err != nil {
		r.Detail = "verified certificate: " + err.Error()
		return r
	}
	leaf := chain[0]
	r.Certificate, r.CertificateKey = domain.Digest(leaf.Raw), domain.Digest(leaf.RawSubjectPublicKeyInfo)
	inter := x509.NewCertPool()
	for _, x := range chain[1:] {
		inter.AddCert(x)
	}
	_, err = leaf.Verify(x509.VerifyOptions{Roots: pool, Intermediates: inter, CurrentTime: s.v.clock.Now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	var unknown x509.UnknownAuthorityError
	switch {
	case err == nil:
		r.Outcome = ConsumerVerified
		r.Detail = fmt.Sprintf("%s[%s] issued after re-issuing %v verifies against %s[%s] captured before", c.Verify.Secret, c.Verify.Key, c.Reissue, c.Trust.Secret, c.Trust.Key)
	case errors.As(err, &unknown):
		r.Outcome = ConsumerFailed
		r.Detail = fmt.Sprintf("%s[%s] issued after re-issuing %v does not verify against %s[%s] captured before: %s", c.Verify.Secret, c.Verify.Key, c.Reissue, c.Trust.Secret, c.Trust.Key, clip(err.Error(), 300))
	default:
		r.Detail = fmt.Sprintf("x509 verification could not decide (not an authority mismatch): %s", clip(err.Error(), 300))
	}
	return r
}

// reissue applies the exercise's action to one object and waits until its
// declared reaction field changes (bounded by settle).
func (s *session) reissue(ctx context.Context, cl Cluster, d DeclaredExercise, id, token string, settle time.Duration) (bool, string) {
	ref, _ := ParseRef(id)
	path := d.Exercise.Reaction.Path
	obj, err := getObject(ctx, cl, ref)
	if err != nil {
		return false, err.Error()
	}
	before := fieldValue(obj, path)
	sub, patch, err := exercisePatch(d.Exercise, obj, token, s.v.clock.Now())
	if err == nil {
		err = cl.Patch(ctx, ref, sub, patch)
	}
	if err != nil {
		return false, clip(err.Error(), 300)
	}
	for waited := time.Duration(0); waited < settle; waited += consumerPoll {
		if err := s.v.clock.Sleep(ctx, consumerPoll); err != nil {
			return false, err.Error()
		}
		if now, err := getObject(ctx, cl, ref); err == nil {
			if v := fieldValue(now, path); v != "" && v != before {
				return true, ""
			}
		}
	}
	return false, fmt.Sprintf("%s did not change within %s", path, settle)
}

func getObject(ctx context.Context, cl Cluster, ref ObjectRef) (map[string]any, error) {
	items, err := cl.List(ctx, ref.GVK())
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if r, ok := refOf(it); ok && r.Kind == ref.Kind && r.Namespace == ref.Namespace && r.Name == ref.Name {
			return it, nil
		}
	}
	return nil, fmt.Errorf("%s does not exist", ref)
}

// readCertificates parses the PEM certificates of one Secret key. Any other
// PEM block (a private key) refuses the key without parsing it.
func readCertificates(ctx context.Context, cl Cluster, sk SecretKey) ([]*x509.Certificate, error) {
	ref, _ := ParseRef(sk.Secret)
	obj, err := getObject(ctx, cl, ref)
	if err != nil {
		return nil, err
	}
	data, _ := obj["data"].(map[string]any)
	enc, _ := data[sk.Key].(string)
	if enc == "" {
		return nil, fmt.Errorf("%s has no key %s", sk.Secret, sk.Key)
	}
	rest, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, fmt.Errorf("%s[%s] is not base64", sk.Secret, sk.Key)
	}
	var out []*x509.Certificate
	for {
		var b *pem.Block
		if b, rest = pem.Decode(rest); b == nil {
			break
		}
		if b.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("%s[%s] holds a %q PEM block, not certificates: refused (key material is never read)", sk.Secret, sk.Key, b.Type)
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%s[%s]: %v", sk.Secret, sk.Key, err)
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s[%s] holds no PEM certificate", sk.Secret, sk.Key)
	}
	return out, nil
}
