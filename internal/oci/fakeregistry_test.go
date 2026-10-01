package oci

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

const testToken = "tok-123"

// fakeRegistry is a small in-memory OCI registry served over httptest. It
// speaks just enough of the Distribution API for the adapter: manifests
// (GET/HEAD), tags/list with Link pagination, blobs and an anonymous token
// endpoint.
type fakeRegistry struct {
	t   *testing.T
	srv *httptest.Server

	// RequireAuth makes /v2/ answer 401 with a Bearer challenge unless the
	// request carries "Authorization: Bearer tok-123".
	RequireAuth bool
	// HeadStatus, when non-zero, is the status returned for every HEAD
	// manifest request (e.g. 405).
	HeadStatus int
	// OmitDigestOnHead drops Docker-Content-Digest from HEAD responses.
	OmitDigestOnHead bool
	// TokenStatus, when non-zero, is the status of the token endpoint.
	TokenStatus int
	// TokenExpiresIn is the expires_in of issued tokens (default 300).
	TokenExpiresIn int
	// Token is the token issued and accepted (default testToken); see setToken.
	Token string
	// PageSize paginates tags/list (0 = no pagination).
	PageSize int
	// QuoteScope adds a comma-containing scope to the challenge.
	QuoteScope bool

	mu        sync.Mutex
	manifests map[string]fakeManifest // "name@ref" -> manifest
	blobs     map[string][]byte       // "name@digest" -> bytes
	tags      map[string][]string     // name -> tags
	hits      map[string]int          // "METHOD kind" -> count
	tokenReqs []url.Values
}

type fakeManifest struct {
	body        []byte
	contentType string
}

func (m fakeManifest) digest() string { return domain.Digest(m.body) }

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	r := &fakeRegistry{
		t:         t,
		manifests: map[string]fakeManifest{},
		blobs:     map[string][]byte{},
		tags:      map[string][]string{},
		hits:      map[string]int{},
	}
	r.srv = httptest.NewServer(http.HandlerFunc(r.handle))
	t.Cleanup(r.srv.Close)
	return r
}

// host is the registry host as it appears in locators ("127.0.0.1:port").
func (r *fakeRegistry) host() string { return strings.TrimPrefix(r.srv.URL, "http://") }

// addManifest registers a manifest under name:ref and returns its digest.
func (r *fakeRegistry) addManifest(name, ref, contentType, body string) string {
	m := fakeManifest{body: []byte(body), contentType: contentType}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manifests[name+"@"+ref] = m
	r.manifests[name+"@"+m.digest()] = m
	return m.digest()
}

func (r *fakeRegistry) addBlob(name string, body []byte) string {
	d := domain.Digest(body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blobs[name+"@"+d] = body
	return d
}

// currentToken is the token the registry issues and accepts.
func (r *fakeRegistry) currentToken() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Token != "" {
		return r.Token
	}
	return testToken
}

// setToken rotates the token (previously issued tokens stop working).
func (r *fakeRegistry) setToken(tok string) {
	r.mu.Lock()
	r.Token = tok
	r.mu.Unlock()
}

func (r *fakeRegistry) hitCount(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits[key]
}

func (r *fakeRegistry) totalHits() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, v := range r.hits {
		n += v
	}
	return n
}

func (r *fakeRegistry) record(key string) {
	r.mu.Lock()
	r.hits[key]++
	r.mu.Unlock()
}

func (r *fakeRegistry) handle(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/token" {
		r.record("token")
		r.mu.Lock()
		r.tokenReqs = append(r.tokenReqs, req.URL.Query())
		r.mu.Unlock()
		if r.TokenStatus != 0 {
			w.WriteHeader(r.TokenStatus)
			return
		}
		exp := r.TokenExpiresIn
		if exp == 0 {
			exp = 300
		}
		json.NewEncoder(w).Encode(map[string]any{"token": r.currentToken(), "expires_in": exp})
		return
	}
	rest, ok := strings.CutPrefix(req.URL.Path, "/v2/")
	if !ok {
		http.NotFound(w, req)
		return
	}
	var name, kind, ref string
	for _, k := range []string{"manifests", "tags", "blobs"} {
		if i := strings.Index(rest, "/"+k+"/"); i > 0 {
			name, kind, ref = rest[:i], k, rest[i+len(k)+2:]
			break
		}
	}
	if kind == "" {
		http.NotFound(w, req)
		return
	}
	if r.RequireAuth && req.Header.Get("Authorization") != "Bearer "+r.currentToken() {
		scope := "repository:" + name + ":pull"
		if r.QuoteScope {
			scope += ",push"
		}
		w.Header().Set("WWW-Authenticate",
			fmt.Sprintf(`Bearer realm="%s/token",service="fake-registry",scope="%s"`, r.srv.URL, scope))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`))
		return
	}
	r.record(req.Method + " " + kind)
	switch kind {
	case "manifests":
		r.serveManifest(w, req, name, ref)
	case "blobs":
		r.mu.Lock()
		b, ok := r.blobs[name+"@"+ref]
		r.mu.Unlock()
		if !ok {
			r.registryError(w, http.StatusNotFound, "BLOB_UNKNOWN")
			return
		}
		w.Write(b)
	case "tags":
		r.serveTags(w, req, name)
	}
}

func (r *fakeRegistry) registryError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"errors":[{"code":%q}]}`, code)
}

func (r *fakeRegistry) serveManifest(w http.ResponseWriter, req *http.Request, name, ref string) {
	if req.Method == http.MethodHead && r.HeadStatus != 0 {
		w.WriteHeader(r.HeadStatus)
		return
	}
	r.mu.Lock()
	m, ok := r.manifests[name+"@"+ref]
	r.mu.Unlock()
	if !ok {
		if req.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		r.registryError(w, http.StatusNotFound, "MANIFEST_UNKNOWN")
		return
	}
	w.Header().Set("Content-Type", m.contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(m.body)))
	if req.Method != http.MethodHead || !r.OmitDigestOnHead {
		w.Header().Set("Docker-Content-Digest", m.digest())
	}
	if req.Method == http.MethodHead {
		return
	}
	w.Write(m.body)
}

func (r *fakeRegistry) serveTags(w http.ResponseWriter, req *http.Request, name string) {
	r.mu.Lock()
	all, ok := r.tags[name]
	r.mu.Unlock()
	if !ok {
		r.registryError(w, http.StatusNotFound, "NAME_UNKNOWN")
		return
	}
	start := 0
	if last := req.URL.Query().Get("last"); last != "" {
		for i, t := range all {
			if t == last {
				start = i + 1
			}
		}
	}
	end := len(all)
	if r.PageSize > 0 && start+r.PageSize < end {
		end = start + r.PageSize
		// Relative Link, like Docker Hub and ghcr.io.
		w.Header().Set("Link", fmt.Sprintf(`</v2/%s/tags/list?last=%s&n=%d>; rel="next"`,
			name, url.QueryEscape(all[end-1]), r.PageSize))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"name": name, "tags": all[start:end]})
}
