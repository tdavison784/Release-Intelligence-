package fetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// fakeSleep records waits and advances a fake clock instead of sleeping.
type fakeSleep struct {
	mu    sync.Mutex
	waits []time.Duration
	now   time.Time
}

func (f *fakeSleep) sleep(_ context.Context, d time.Duration) error {
	f.mu.Lock()
	f.waits = append(f.waits, d)
	f.now = f.now.Add(d)
	f.mu.Unlock()
	return nil
}

func (f *fakeSleep) clock() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func newTestClient(fs *fakeSleep) *HTTPClient {
	fs.now = time.Unix(1_000_000, 0)
	c := NewHTTPClient(nil, ModeOnline)
	c.sleep = fs.sleep
	c.now = fs.clock
	return c
}

func TestThrottledRetriesHonourRetryAfterThenSucceeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) <= 2 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"errors":[{"code":"TOOMANYREQUESTS"}]}`))
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	fs := &fakeSleep{}
	d, err := Get(context.Background(), newTestClient(fs), srv.URL+"/x", false)
	if err != nil || string(d.Body) != "ok" {
		t.Fatalf("want success after retries, got %v", err)
	}
	if hits != 3 || len(fs.waits) < 2 || fs.waits[0] != 7*time.Second {
		t.Fatalf("hits=%d waits=%v", hits, fs.waits)
	}
}

func TestThrottledAfterRetriesIsDistinctButStillUnavailable(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	fs := &fakeSleep{}
	c := newTestClient(fs)
	c.MaxRetries = 2
	_, err := Get(context.Background(), c, srv.URL+"/x", false)
	if !errors.Is(err, ErrThrottled) || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want throttled (and unavailable), got %v", err)
	}
	if StateFor(err) != domain.SourceThrottled {
		t.Fatalf("state %q", StateFor(err))
	}
	if hits != 3 { // initial + 2 retries
		t.Fatalf("hits=%d", hits)
	}
	// exponential backoff: 1s, 2s
	if len(fs.waits) < 2 || fs.waits[0] != time.Second || fs.waits[1] != 2*time.Second {
		t.Fatalf("waits %v", fs.waits)
	}
}

func TestRetryAfterBeyondCapIsNotWaitedOut(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	fs := &fakeSleep{}
	_, err := Get(context.Background(), newTestClient(fs), srv.URL+"/x", false)
	var fe *Error
	if !errors.As(err, &fe) || fe.RetryAfter != time.Hour || !errors.Is(err, ErrThrottled) {
		t.Fatalf("got %v", err)
	}
	if hits != 1 || len(fs.waits) != 0 {
		t.Fatalf("must fail fast: hits=%d waits=%v", hits, fs.waits)
	}
}

func TestRetriesDisabled(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := newTestClient(&fakeSleep{})
	c.MaxRetries = -1
	if _, err := Get(context.Background(), c, srv.URL, false); !errors.Is(err, ErrThrottled) || hits != 1 {
		t.Fatalf("err=%v hits=%d", err, hits)
	}
}

func TestGitHubQuotaIs403ButThrottled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()
	_, err := Get(context.Background(), newTestClient(&fakeSleep{}), srv.URL, false)
	if !errors.Is(err, ErrThrottled) || errors.Is(err, ErrForbidden) {
		t.Fatalf("exhausted quota must read as throttled, got %v", err)
	}
	var fe *Error
	if !errors.As(err, &fe) || fe.RetryAfter < 59*time.Minute {
		t.Fatalf("reset time not surfaced: %+v", fe)
	}
}

func TestAuthForbiddenAndNotFoundAreDistinguished(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth":
			w.Header().Set("Www-Authenticate", `Bearer realm="https://r/token",service="r"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("denied by policy"))
		}
	}))
	defer srv.Close()
	c := newTestClient(&fakeSleep{})
	_, err := Get(context.Background(), c, srv.URL+"/auth", false)
	if !errors.Is(err, ErrAuthRequired) || !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrForbidden) {
		t.Fatalf("401: %v", err)
	}
	var fe *Error
	if !errors.As(err, &fe) || fe.Detail == "" || fe.Header.Get("Www-Authenticate") == "" {
		t.Fatalf("401 must keep challenge: %+v", fe)
	}
	_, err = Get(context.Background(), c, srv.URL+"/forbidden", false)
	if !errors.Is(err, ErrForbidden) || errors.Is(err, ErrAuthRequired) || errors.Is(err, ErrThrottled) {
		t.Fatalf("403: %v", err)
	}
	if StateFor(err) != domain.SourceUnavailable {
		t.Fatalf("403 state %q", StateFor(err))
	}
}

func TestDNSFailureIsDistinguished(t *testing.T) {
	c := newTestClient(&fakeSleep{})
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &net.DNSError{Err: "no such host", Name: "docs.example.invalid", IsNotFound: true}
	})}
	_, err := Get(context.Background(), c, "https://docs.example.invalid/x", false)
	if !errors.Is(err, ErrDNS) || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransientGatewayErrorsAreRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	d, err := Get(context.Background(), newTestClient(&fakeSleep{}), srv.URL, false)
	if err != nil || string(d.Body) != "ok" || hits != 2 {
		t.Fatalf("err=%v hits=%d", err, hits)
	}
}

func TestNonIdempotentRequestsAreNotRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := newTestClient(&fakeSleep{})
	_, err := c.Do(context.Background(), Request{URL: srv.URL, Method: http.MethodPost})
	if !errors.Is(err, ErrThrottled) || hits != 1 {
		t.Fatalf("err=%v hits=%d", err, hits)
	}
}

func TestPerHostConcurrencyIsBounded(t *testing.T) {
	var cur, peak int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&cur, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := NewHTTPClient(nil, ModeOnline)
	c.HostLimits = map[string]int{hostOf(srv.URL): 2}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := Get(context.Background(), c, srv.URL+"/"+strconv.Itoa(i), false); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if peak > 2 || peak < 1 {
		t.Fatalf("peak concurrency %d, want <= 2", peak)
	}
}

func TestThrottledHostMakesLaterRequestsWait(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	fs := &fakeSleep{}
	c := newTestClient(fs)
	// Request A is throttled, waits Retry-After, retries and succeeds. The
	// gate is paused until the end of that wait; a request B started "at the
	// throttle instant" (clock rewound) must wait for the remainder.
	if _, err := Get(context.Background(), c, srv.URL+"/a", false); err != nil {
		t.Fatal(err)
	}
	fs.mu.Lock()
	fs.now = fs.now.Add(-3 * time.Second) // 3s of the 5s pause "left"
	fs.waits = nil
	fs.mu.Unlock()
	if _, err := Get(context.Background(), c, srv.URL+"/b", false); err != nil {
		t.Fatal(err)
	}
	if len(fs.waits) != 1 || fs.waits[0] != 3*time.Second {
		t.Fatalf("second request should wait the remaining pause, waits=%v", fs.waits)
	}
}

func TestParseRetryAfterForms(t *testing.T) {
	now := time.Unix(1_000_000, 0).UTC()
	h := http.Header{}
	h.Set("Retry-After", "12")
	if got := retryAfter(h, now); got != 12*time.Second {
		t.Fatal(got)
	}
	h.Set("Retry-After", now.Add(90*time.Second).Format(http.TimeFormat))
	if got := retryAfter(h, now); got != 90*time.Second {
		t.Fatal(got)
	}
	if got := retryAfter(http.Header{}, now); got != 0 {
		t.Fatal(got)
	}
}

func TestReasonFromErrorAndFromStoredDetail(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&Error{URL: "u", Err: ErrThrottled, Status: 429}, "throttled"},
		{&Error{URL: "u", Err: ErrAuthRequired, Status: 401}, "authentication required"},
		{&Error{URL: "u", Err: ErrForbidden, Status: 403}, "forbidden"},
		{&Error{URL: "u", Err: ErrDNS}, "DNS resolution failed"},
		{&Error{URL: "u", Err: ErrUnavailable, Detail: "connection refused"}, ""},
		{&Error{URL: "u", Err: ErrNotFound}, ""},
	} {
		if got := Reason(tc.err); got != tc.want {
			t.Errorf("Reason(%v) = %q, want %q", tc.err, got, tc.want)
		}
		// the stored text of the error (SourceStatus.Detail) carries the reason too
		if got := Reason(tc.err.Error()); got != tc.want {
			t.Errorf("Reason(%q) = %q, want %q", tc.err.Error(), got, tc.want)
		}
	}
}
