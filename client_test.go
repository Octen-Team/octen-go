package octen

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func setKey(t *testing.T) {
	t.Helper()
	t.Setenv("OCTEN_API_KEY", "env-key")
}

func TestNewClientValidation(t *testing.T) {
	t.Setenv("OCTEN_API_KEY", "")
	cases := []struct {
		name  string
		opts  []Option
		field string
		msg   string
	}{
		{"no key", nil, "WithAPIKey", "OCTEN_API_KEY"},
		{"blank key", []Option{WithAPIKey("  \t")}, "WithAPIKey", "OCTEN_API_KEY"},
		{"key with newline", []Option{WithAPIKey("a\nb")}, "WithAPIKey", "header"},
		{"key with NUL", []Option{WithAPIKey("a\x00b")}, "WithAPIKey", "header"},
		{"key with SOH", []Option{WithAPIKey("a\x01b")}, "WithAPIKey", "header"},
		{"key with DEL", []Option{WithAPIKey("a\x7fb")}, "WithAPIKey", "header"},
		{"empty base URL", []Option{WithAPIKey("k"), WithBaseURL("")}, "WithBaseURL", "absolute http(s) URL"},
		{"relative base URL", []Option{WithAPIKey("k"), WithBaseURL("api.octen.ai")}, "WithBaseURL", "absolute http(s) URL"},
		{"ftp base URL", []Option{WithAPIKey("k"), WithBaseURL("ftp://api.octen.ai")}, "WithBaseURL", "absolute http(s) URL"},
		{"no host", []Option{WithAPIKey("k"), WithBaseURL("https:///v1")}, "WithBaseURL", "absolute http(s) URL"},
		{"empty hostname", []Option{WithAPIKey("k"), WithBaseURL("http://:8080")}, "WithBaseURL", "absolute http(s) URL"},
		{"whitespace in base URL", []Option{WithAPIKey("k"), WithBaseURL("https://api.octen.ai/a b")}, "WithBaseURL", "whitespace"},
		{"query in base URL", []Option{WithAPIKey("k"), WithBaseURL("https://api.octen.ai?x=1")}, "WithBaseURL", "query"},
		{"fragment in base URL", []Option{WithAPIKey("k"), WithBaseURL("https://api.octen.ai/#top")}, "WithBaseURL", "fragment"},
		{"credentials in base URL", []Option{WithAPIKey("k"), WithBaseURL("https://user:pw@api.octen.ai")}, "WithBaseURL", "credentials"},
		{"user only in base URL", []Option{WithAPIKey("k"), WithBaseURL("https://user@api.octen.ai")}, "WithBaseURL", "credentials"},
		{"invalid port", []Option{WithAPIKey("k"), WithBaseURL("http://host:abc")}, "WithBaseURL", "absolute http(s) URL"},
		{"port above 65535", []Option{WithAPIKey("k"), WithBaseURL("http://127.0.0.1:99999")}, "WithBaseURL", "port"},
		{"port zero", []Option{WithAPIKey("k"), WithBaseURL("http://127.0.0.1:0")}, "WithBaseURL", "port"},
		{"missing bracket", []Option{WithAPIKey("k"), WithBaseURL("http://[::1")}, "WithBaseURL", "absolute http(s) URL"},
		{"invalid escape", []Option{WithAPIKey("k"), WithBaseURL("https://api.octen.ai/%zz")}, "WithBaseURL", "absolute http(s) URL"},
		{"control character", []Option{WithAPIKey("k"), WithBaseURL("http://a\x01b")}, "WithBaseURL", "absolute http(s) URL"},
		{"zero timeout", []Option{WithAPIKey("k"), WithTimeout(0)}, "WithTimeout", "greater than 0"},
		{"negative timeout", []Option{WithAPIKey("k"), WithTimeout(-time.Second)}, "WithTimeout", "greater than 0"},
		{"negative retries", []Option{WithAPIKey("k"), WithMaxRetries(-1)}, "WithMaxRetries", "non-negative"},
		{"empty header name", []Option{WithAPIKey("k"), WithHeader("", "v")}, "WithHeader", "token"},
		{"header name with space", []Option{WithAPIKey("k"), WithHeader("X Bad", "v")}, "WithHeader", "token"},
		{"header name with colon", []Option{WithAPIKey("k"), WithHeader("X:Bad", "v")}, "WithHeader", "token"},
		{"header name non-ASCII", []Option{WithAPIKey("k"), WithHeader("X-Bäd", "v")}, "WithHeader", "token"},
		{"header value CR", []Option{WithAPIKey("k"), WithHeader("X-A", "a\rb")}, "WithHeader", "control characters"},
		{"header value LF", []Option{WithAPIKey("k"), WithHeader("X-A", "a\nb")}, "WithHeader", "control characters"},
		{"header value NUL", []Option{WithAPIKey("k"), WithHeader("X-A", "a\x00b")}, "WithHeader", "control characters"},
		{"header value SOH", []Option{WithAPIKey("k"), WithHeader("X-A", "a\x01b")}, "WithHeader", "control characters"},
		{"header value DEL", []Option{WithAPIKey("k"), WithHeader("X-A", "a\x7fb")}, "WithHeader", "control characters"},
		{"x-api-key header", []Option{WithAPIKey("k"), WithHeader("x-api-key", "other")}, "WithHeader", "WithAPIKey"},
		{"X-API-KEY header", []Option{WithAPIKey("k"), WithHeader("X-API-KEY", "other")}, "WithHeader", "WithAPIKey"},
		{"nil http client", []Option{WithAPIKey("k"), WithHTTPClient(nil)}, "WithHTTPClient", "nil"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewClient(tc.opts...)
			if c != nil {
				t.Fatalf("client = %v, want nil on error", c)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %#v, want *ValidationError", err)
			}
			if ve.Field != tc.field || !strings.Contains(ve.Message, tc.msg) {
				t.Fatalf("got field %q message %q; want field %q containing %q", ve.Field, ve.Message, tc.field, tc.msg)
			}
		})
	}
}

func TestNewClientAcceptsValidOptions(t *testing.T) {
	t.Setenv("OCTEN_API_KEY", "")
	for _, base := range []string{
		"https://api.octen.ai", "http://localhost:8080", "HTTPS://API.OCTEN.AI/", "https://gw.example.com/octen//",
		"http://[::1]:9000/prefix", "http://127.0.0.1",
	} {
		if _, err := NewClient(WithAPIKey("k"), WithBaseURL(base)); err != nil {
			t.Errorf("WithBaseURL(%q): %v", base, err)
		}
	}
	if _, err := NewClient(WithAPIKey("k\tk"), WithBaseURL("http://127.0.0.1:65535"), WithMaxRetries(0), WithTimeout(time.Nanosecond), WithHeader("X-Trace_ID.1~", "a b\tc"), WithHTTPClient(&http.Client{})); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}
}

func TestAPIKeyFromEnv(t *testing.T) {
	var seen string
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		seen = r.Header.Get("x-api-key")
		writeOK(w)
	})
	call := func(opts ...Option) {
		t.Helper()
		c, err := NewClient(append([]Option{WithBaseURL(s.URL)}, opts...)...)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		if _, err := c.Search.Search(context.Background(), SearchParams{Query: "q"}); err != nil {
			t.Fatalf("Search: %v", err)
		}
	}

	t.Setenv("OCTEN_API_KEY", "  env-key\n")
	call()
	if seen != "env-key" {
		t.Fatalf("key from env = %q, want env-key (trimmed)", seen)
	}
	call(WithAPIKey("explicit-key"))
	if seen != "explicit-key" {
		t.Fatalf("explicit key = %q, want explicit-key", seen)
	}
	call(WithAPIKey("   "))
	if seen != "env-key" {
		t.Fatalf("blank explicit key = %q, want fallback to env-key", seen)
	}
}

// capturedDeadline answers every request with okBody and records the time left on its context.
type capturedDeadline struct {
	left time.Duration
	ok   bool
}

func (c *capturedDeadline) client() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var d time.Time
		d, c.ok = r.Context().Deadline()
		c.left = time.Until(d)
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(okBody)),
			Request:    r,
		}, nil
	})}
}

// endpointCall drives one SDK method; the table is the list of what the SDK calls.
type endpointCall struct {
	path string
	def  time.Duration
	call func(*Client, ...RequestOption) error
}

var endpointCalls = []endpointCall{
	{"/search", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.Search.Search(context.Background(), SearchParams{Query: "q"}, o...)
		return err
	}},
	{"/broad-search", 120 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.Search.BroadSearch(context.Background(), BroadSearchParams{Query: "q"}, o...)
		return err
	}},
	{"/news-search", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.Search.NewsSearch(context.Background(), NewsSearchParams{Query: "q"}, o...)
		return err
	}},
	{"/business-search", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.Search.BusinessSearch(context.Background(), BusinessSearchParams{Query: "q"}, o...)
		return err
	}},
	{"/pulse-search", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.Search.PulseSearch(context.Background(), PulseSearchParams{Query: "q"}, o...)
		return err
	}},
	{"/place-search", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.PlaceSearch.Search(context.Background(), PlaceSearchParams{Query: "q"}, o...)
		return err
	}},
	{"/search-premier", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.Search.SearchPremier(context.Background(), SearchParams{Query: "q"}, o...)
		return err
	}},
	{"/image-search", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.ImageSearch.Search(context.Background(), ImageSearchParams{Inputs: []ImageSearchInput{{Type: "text", Data: String("q")}}}, o...)
		return err
	}},
	{"/video-search", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.VideoSearch.Search(context.Background(), VideoSearchParams{Inputs: []VideoSearchInput{{Type: "text", Data: String("q")}}}, o...)
		return err
	}},
	{"/design-search", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.DesignSearch.Search(context.Background(), DesignSearchParams{Query: String("q")}, o...)
		return err
	}},
	{"/extract", 120 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.Extract.Extract(context.Background(), ExtractParams{URLs: []string{"https://example.com/"}}, o...)
		return err
	}},
	{"/embedding", 30 * time.Second, func(c *Client, o ...RequestOption) error {
		_, err := c.Embedding.Create(context.Background(), EmbeddingParams{Input: []string{"a"}}, o...)
		return err
	}},
}

// The effective per-attempt timeout is read off the request context deadline, which the
// transport arms with exactly that value; the slack covers scheduling delay only.
func TestTimeoutPrecedence(t *testing.T) {
	setKey(t)
	type tcase struct {
		name   string
		client []Option
		call   endpointCall
		req    []RequestOption
		want   time.Duration
	}
	var cases []tcase
	for _, ep := range endpointCalls {
		cases = append(cases,
			tcase{ep.path + " default", nil, ep, nil, ep.def},
			tcase{ep.path + " per-request beats default", nil, ep, []RequestOption{WithRequestTimeout(5 * time.Second)}, 5 * time.Second},
			tcase{ep.path + " explicit client timeout", []Option{WithTimeout(10 * time.Second)}, ep, nil, 10 * time.Second},
		)
	}
	broad := endpointCalls[1]
	cases = append(cases,
		tcase{"explicit client timeout equal to 30s still applies to broad", []Option{WithTimeout(30 * time.Second)}, broad, nil, 30 * time.Second},
		tcase{"per-request beats client", []Option{WithTimeout(10 * time.Second)}, broad, []RequestOption{WithRequestTimeout(3 * time.Second)}, 3 * time.Second},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cd capturedDeadline
			c, err := NewClient(append([]Option{WithHTTPClient(cd.client())}, tc.client...)...)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if err := tc.call.call(c, tc.req...); err != nil {
				t.Fatalf("call: %v", err)
			}
			if !cd.ok || cd.left > tc.want || cd.left < tc.want-2*time.Second {
				t.Fatalf("time left on attempt context = %v (deadline set: %v), want just under %v", cd.left, cd.ok, tc.want)
			}
		})
	}
}

func TestRequestHeaderReachesEveryEndpoint(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL, WithHeader("X-Per-Call", "client"))
	for i, ep := range endpointCalls {
		if err := ep.call(c, WithRequestHeader("X-Per-Call", "call\t"+ep.path)); err != nil {
			t.Fatalf("%s: %v", ep.path, err)
		}
		got := rec.requests()[i]
		if got.path != ep.path || !reflect.DeepEqual(got.header.Values("X-Per-Call"), []string{"call\t" + ep.path}) {
			t.Errorf("%s: path %s, X-Per-Call %q", ep.path, got.path, got.header.Values("X-Per-Call"))
		}
	}
}

func TestTimeoutIsPerRequestOnTheWire(t *testing.T) {
	setKey(t)
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		stallUntilGone(r, 2*time.Second)
		writeOK(w)
	})
	c, err := NewClient(WithBaseURL(s.URL), WithMaxRetries(0), WithTimeout(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Search.Search(context.Background(), SearchParams{Query: "q"}, WithRequestTimeout(100*time.Millisecond))
	var te *TimeoutError
	if !errors.As(err, &te) || te.Timeout != 100*time.Millisecond {
		t.Fatalf("err = %#v, want *TimeoutError with Timeout 100ms", err)
	}
}

func TestRequestOptionValidation(t *testing.T) {
	setKey(t)
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { writeOK(w) })
	c, err := NewClient(WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		opt   RequestOption
		field string
	}{
		{"zero timeout", WithRequestTimeout(0), "WithRequestTimeout"},
		{"negative timeout", WithRequestTimeout(-1), "WithRequestTimeout"},
		{"bad header name", WithRequestHeader("bad name", "v"), "WithRequestHeader"},
		{"bad header value", WithRequestHeader("X-A", "a\r\nInjected: 1"), "WithRequestHeader"},
		{"header value SOH", WithRequestHeader("X-A", "a\x01b"), "WithRequestHeader"},
		{"header value DEL", WithRequestHeader("X-A", "a\x7fb"), "WithRequestHeader"},
		{"x-api-key", WithRequestHeader("X-Api-Key", "other"), "WithRequestHeader"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Search.Search(context.Background(), SearchParams{Query: "q"}, tc.opt)
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("err = %#v, want *ValidationError for %s", err, tc.field)
			}
		})
	}
	if got := s.hits.Load(); got != 0 {
		t.Fatalf("server saw %d requests, want 0", got)
	}
}

func TestNonFiniteDoesNotReachTransport(t *testing.T) {
	setKey(t)
	c, err := NewClient(WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("transport reached")
		return nil, errors.New("unexpected")
	})}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.PlaceSearch.Search(context.Background(), PlaceSearchParams{Query: "q", Latitude: Float64(math.NaN())})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "latitude" {
		t.Fatalf("err = %#v, want *ValidationError on latitude", err)
	}
}

// newDefaultClient builds a client through NewClient with default retries, recording
// backoff waits instead of sleeping.
func newDefaultClient(t *testing.T, url string, opts ...Option) (*Client, *sleepRecorder) {
	t.Helper()
	c := newTestClient(t, url, opts...)
	rec := &sleepRecorder{}
	tr := c.Search.c.transport
	tr.sleep = rec.sleep
	tr.rnd = func() float64 { return 0 }
	return c, rec
}

func TestDefaultRetryCountIsThree(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.WriteHeader(503) })
	c, rec := newDefaultClient(t, s.URL)
	if _, err := c.Search.Search(context.Background(), SearchParams{Query: "q"}); !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	if got := s.hits.Load(); got != 4 {
		t.Fatalf("server saw %d requests, want 4", got)
	}
	if got, want := rec.got(), []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; !reflect.DeepEqual(got, want) {
		t.Fatalf("backoff waits = %v, want %v", got, want)
	}
}

func TestWithMaxRetriesTwo(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.WriteHeader(503) })
	c, rec := newDefaultClient(t, s.URL, WithMaxRetries(2))
	if _, err := c.Search.Search(context.Background(), SearchParams{Query: "q"}); !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	if got := s.hits.Load(); got != 3 || len(rec.got()) != 2 {
		t.Fatalf("requests = %d, sleeps = %d; want 3, 2", got, len(rec.got()))
	}
}

// No sleep is injected: the real backoff must end when the caller's context does.
func TestCallerDeadlineInterruptsRealBackoff(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
	})
	c := newTestClient(t, s.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Search.Search(ctx, SearchParams{Query: "q"})
	elapsed := time.Since(start)
	var te *TimeoutError
	if !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &te) {
		t.Fatalf("err = %v, want the caller's context.DeadlineExceeded", err)
	}
	if got := s.hits.Load(); got != 1 || elapsed >= time.Second {
		t.Fatalf("requests = %d after %v; want 1 in under 1s", got, elapsed)
	}
}

func TestBaseURLPathPrefix(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL+"/prefix//")
	if _, err := c.Search.Search(context.Background(), SearchParams{Query: "q"}); err != nil {
		t.Fatal(err)
	}
	if got := rec.requests()[0].path; got != "/prefix/search" {
		t.Fatalf("path = %q, want /prefix/search", got)
	}
}

// Header names are case-insensitive, so a later WithHeader replaces an earlier one.
func TestDuplicateClientHeaderKeepsLast(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL, WithHeader("X-A", "1"), WithHeader("x-a", "2"))
	if _, err := c.Search.Search(context.Background(), SearchParams{Query: "q"}); err != nil {
		t.Fatal(err)
	}
	if got := rec.requests()[0].header.Values("X-A"); !reflect.DeepEqual(got, []string{"2"}) {
		t.Fatalf("X-A = %q, want exactly [\"2\"]", got)
	}
}
