package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// challenge is one parsed WWW-Authenticate challenge.
type challenge struct {
	Scheme string            // canonical-case scheme: "Bearer", "Basic"
	Params map[string]string // lower-case parameter names
}

// parseChallenges parses the values of one or more WWW-Authenticate headers
// (RFC 7235 section 4.1). It is deliberately forgiving: values may be quoted
// or bare, quoted values may contain commas ("pull,push") and several
// challenges may share one header value.
func parseChallenges(values []string) []challenge {
	var out []challenge
	for _, v := range values {
		s := v
		var cur *challenge
		for {
			s = strings.TrimLeft(s, " \t,")
			if s == "" {
				break
			}
			i := 0
			for i < len(s) && isTokenChar(s[i]) {
				i++
			}
			if i == 0 {
				s = s[1:] // stray character, skip it
				continue
			}
			tok := s[:i]
			rest := strings.TrimLeft(s[i:], " \t")
			if cur != nil && strings.HasPrefix(rest, "=") {
				val, remainder := readParamValue(strings.TrimLeft(rest[1:], " \t"))
				cur.Params[strings.ToLower(tok)] = val
				s = remainder
				continue
			}
			out = append(out, challenge{Scheme: canonicalScheme(tok), Params: map[string]string{}})
			cur = &out[len(out)-1]
			s = rest
		}
	}
	return out
}

func canonicalScheme(s string) string {
	if strings.EqualFold(s, "bearer") {
		return "Bearer"
	}
	if strings.EqualFold(s, "basic") {
		return "Basic"
	}
	return s
}

// readParamValue reads a quoted-string or a bare token and returns it with the
// unread remainder of s.
func readParamValue(s string) (value, rest string) {
	if strings.HasPrefix(s, `"`) {
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			switch c := s[i]; {
			case c == '\\' && i+1 < len(s):
				i++
				b.WriteByte(s[i])
			case c == '"':
				return b.String(), s[i+1:]
			default:
				b.WriteByte(c)
			}
		}
		return b.String(), "" // unterminated: take what is there
	}
	i := 0
	for i < len(s) && s[i] != ',' && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	return s[:i], s[i:]
}

// isTokenChar reports whether c is an RFC 7230 token character, minus "="
// and "," which delimit challenge parameters here.
func isTokenChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0
}

// bearerChallenge returns the first Bearer challenge of a response header.
func bearerChallenge(h http.Header) (challenge, bool) {
	for _, ch := range parseChallenges(h.Values("Www-Authenticate")) {
		if ch.Scheme == "Bearer" && ch.Params["realm"] != "" {
			return ch, true
		}
	}
	return challenge{}, false
}

// token is a cached anonymous bearer token.
type token struct {
	value   string
	expires time.Time
}

// defaultTokenLifetime is used when the token response carries no
// "expires_in" (the token specification's default).
const defaultTokenLifetime = 60 * time.Second

// tokenSkew is subtracted from a token's lifetime so a token is never used in
// the last moments of its validity.
const tokenSkew = 5 * time.Second

// maxTokenResponse bounds the token response we are willing to parse.
const maxTokenResponse = 1 << 20

// tokenKey identifies the token cache slot of a repository. Anonymous pull
// tokens are scoped to one repository, so one slot per (endpoint, name).
func tokenKey(r Repository) string { return r.APIBase + "|" + r.Name }

// cachedToken returns a still-valid token for the repository.
func (a *Adapter) cachedToken(r Repository) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.tokens[tokenKey(r)]
	if !ok || !a.now().Before(t.expires) {
		return "", false
	}
	return t.value, true
}

func (a *Adapter) dropToken(r Repository) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.tokens, tokenKey(r))
}

// acquireToken performs the anonymous token flow of the Docker/OCI
// distribution token specification for challenge ch and stores the result in
// the in-memory token cache.
//
// The token endpoint is queried through a.tokenClient(), which by construction
// never persists responses (see Adapter): bearer tokens must not end up in
// the on-disk fetch cache.
func (a *Adapter) acquireToken(ctx context.Context, r Repository, ch challenge) (string, error) {
	realm, err := url.Parse(ch.Params["realm"])
	if err != nil || (realm.Scheme != "https" && realm.Scheme != "http") || realm.Host == "" {
		return "", unavailablef("%s: unusable token realm %q", r.Registry, ch.Params["realm"])
	}
	q := realm.Query()
	if svc := ch.Params["service"]; svc != "" {
		q.Set("service", svc)
	}
	scope := ch.Params["scope"]
	if scope == "" {
		scope = "repository:" + r.Name + ":pull"
	}
	for _, s := range strings.Fields(scope) { // scopes are space separated
		q.Add("scope", s)
	}
	realm.RawQuery = q.Encode()

	doc, err := a.tokenClient().Do(ctx, fetch.Request{
		URL:     realm.String(),
		Header:  http.Header{"Accept": []string{"application/json"}},
		NoStore: true, // tokens must never reach the on-disk cache
	})
	if err != nil {
		return "", fmt.Errorf("oci: token for %s: %w", r.Given, asUnavailable(err))
	}
	var resp struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if len(doc.Body) > maxTokenResponse {
		return "", unavailablef("%s: token response too large", r.Registry)
	}
	if err := json.Unmarshal(doc.Body, &resp); err != nil {
		return "", unavailablef("%s: token response is not JSON: %v", r.Registry, err)
	}
	val := resp.Token
	if val == "" {
		val = resp.AccessToken
	}
	if val == "" {
		return "", unavailablef("%s: token response carries no token", r.Registry)
	}
	life := defaultTokenLifetime
	if resp.ExpiresIn > 0 {
		life = time.Duration(resp.ExpiresIn) * time.Second
	}
	if life > tokenSkew {
		life -= tokenSkew
	}
	a.mu.Lock()
	a.tokens[tokenKey(r)] = token{value: val, expires: a.now().Add(life)}
	a.mu.Unlock()
	return val, nil
}

// get performs one registry API request with the anonymous bearer-token flow:
//
//  1. The request goes through the (caching) fetch client. When a token for the
//     repository is already known it is sent proactively. Responses served
//     from the fetch cache never need a token at all: the cache key excludes
//     Authorization, so an --offline replay works without any credentials.
//  2. On a 401 with a "Bearer realm=..." challenge a token is obtained from the
//     realm and the request is repeated once. Registries that allow anonymous
//     access without tokens never take this path.
//  3. If a proactively sent token is rejected it is dropped and step 2 runs.
func (a *Adapter) get(ctx context.Context, r Repository, req fetch.Request) (*fetch.Document, error) {
	sent := false
	if tok, ok := a.cachedToken(r); ok {
		req.Header = withAuth(req.Header, tok)
		sent = true
	}
	doc, err := a.f.Do(ctx, req)
	var fe *fetch.Error
	if err == nil || !errors.As(err, &fe) || fe.Status != http.StatusUnauthorized {
		return doc, a.mapErr(err)
	}
	ch, ok := bearerChallenge(fe.Header)
	if !ok {
		return nil, a.mapErr(err) // 401 without a usable challenge: auth required
	}
	if sent {
		a.dropToken(r)
	}
	tok, terr := a.acquireToken(ctx, r, ch)
	if terr != nil {
		return nil, terr
	}
	req.Header = withAuth(req.Header, tok)
	doc, err = a.f.Do(ctx, req)
	return doc, a.mapErr(err)
}

// withAuth returns a copy of h carrying a bearer Authorization header.
func withAuth(h http.Header, tok string) http.Header {
	out := h.Clone()
	if out == nil {
		out = http.Header{}
	}
	out.Set("Authorization", "Bearer "+tok)
	return out
}

// mapErr normalises a fetch error: server-side failures (5xx) are reported as
// fetch.ErrUnavailable, like unreachable registries, so callers can rely on a
// single sentinel for "could not consult this registry".
func (a *Adapter) mapErr(err error) error {
	if err == nil {
		return nil
	}
	var fe *fetch.Error
	if errors.As(err, &fe) && fe.Status >= 500 && !errors.Is(err, fetch.ErrUnavailable) {
		return fmt.Errorf("%w: %w", fetch.ErrUnavailable, err)
	}
	return err
}

// unavailableError is an error that satisfies errors.Is(err,
// fetch.ErrUnavailable) while deliberately hiding its cause's sentinel: a
// token endpoint answering 404 must not make errors.Is(err, fetch.ErrNotFound)
// true, which callers read as "the artifact does not exist".
type unavailableError struct {
	msg   string
	cause error
}

func (e *unavailableError) Error() string {
	if e.cause == nil {
		return fetch.ErrUnavailable.Error() + ": " + e.msg
	}
	return fetch.ErrUnavailable.Error() + ": " + e.msg + ": " + e.cause.Error()
}

func (e *unavailableError) Is(target error) bool { return target == fetch.ErrUnavailable }

// asUnavailable turns any failure of the token endpoint into an error that
// satisfies fetch.ErrUnavailable: an endpoint that 404s or refuses anonymous
// access means the registry cannot be consulted, not that the artifact is
// missing. Context cancellation and offline errors pass through.
func asUnavailable(err error) error {
	if errors.Is(err, fetch.ErrUnavailable) || errors.Is(err, fetch.ErrOffline) {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &unavailableError{msg: "token endpoint", cause: err}
}

// unavailablef builds an error satisfying fetch.ErrUnavailable.
func unavailablef(format string, args ...any) error {
	return &unavailableError{msg: "oci: " + fmt.Sprintf(format, args...)}
}
