package octen

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
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
		{"zero timeout", []Option{WithAPIKey("k"), WithTimeout(0)}, "WithTimeout", "greater than 0"},
		{"negative timeout", []Option{WithAPIKey("k"), WithTimeout(-time.Second)}, "WithTimeout", "greater than 0"},
		{"negative retries", []Option{WithAPIKey("k"), WithMaxRetries(-1)}, "WithMaxRetries", "non-negative"},
		{"empty header name", []Option{WithAPIKey("k"), WithHeader("", "v")}, "WithHeader", "token"},
		{"header name with space", []Option{WithAPIKey("k"), WithHeader("X Bad", "v")}, "WithHeader", "token"},
		{"header name with colon", []Option{WithAPIKey("k"), WithHeader("X:Bad", "v")}, "WithHeader", "token"},
		{"header name non-ASCII", []Option{WithAPIKey("k"), WithHeader("X-Bäd", "v")}, "WithHeader", "token"},
		{"header value CR", []Option{WithAPIKey("k"), WithHeader("X-A", "a\rb")}, "WithHeader", "CR, LF or NUL"},
		{"header value LF", []Option{WithAPIKey("k"), WithHeader("X-A", "a\nb")}, "WithHeader", "CR, LF or NUL"},
		{"header value NUL", []Option{WithAPIKey("k"), WithHeader("X-A", "a\x00b")}, "WithHeader", "CR, LF or NUL"},
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
	if _, err := NewClient(WithAPIKey("k"), WithMaxRetries(0), WithTimeout(time.Nanosecond), WithHeader("X-Trace_ID.1~", "a b\tc"), WithHTTPClient(&http.Client{})); err != nil {
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

// The effective per-attempt timeout is read off the request context deadline, which the
// transport arms with exactly that value; the slack covers scheduling delay only.
func TestTimeoutPrecedence(t *testing.T) {
	setKey(t)
	type callFn func(*Client, ...RequestOption) error
	search := func(c *Client, o ...RequestOption) error {
		_, err := c.Search.Search(context.Background(), SearchParams{Query: "q"}, o...)
		return err
	}
	broad := func(c *Client, o ...RequestOption) error {
		_, err := c.Search.BroadSearch(context.Background(), BroadSearchParams{Query: "q"}, o...)
		return err
	}
	place := func(c *Client, o ...RequestOption) error {
		_, err := c.PlaceSearch.Search(context.Background(), PlaceSearchParams{Query: "q"}, o...)
		return err
	}
	cases := []struct {
		name   string
		client []Option
		call   callFn
		req    []RequestOption
		want   time.Duration
	}{
		{"search default", nil, search, nil, 30 * time.Second},
		{"place default", nil, place, nil, 30 * time.Second},
		{"broad default", nil, broad, nil, 120 * time.Second},
		{"explicit client timeout applies to search", []Option{WithTimeout(10 * time.Second)}, search, nil, 10 * time.Second},
		{"explicit client timeout applies to broad", []Option{WithTimeout(10 * time.Second)}, broad, nil, 10 * time.Second},
		{"explicit client timeout equal to 30s still applies to broad", []Option{WithTimeout(30 * time.Second)}, broad, nil, 30 * time.Second},
		{"per-request beats default", nil, search, []RequestOption{WithRequestTimeout(5 * time.Second)}, 5 * time.Second},
		{"per-request beats default on broad", nil, broad, []RequestOption{WithRequestTimeout(7 * time.Second)}, 7 * time.Second},
		{"per-request beats client", []Option{WithTimeout(10 * time.Second)}, broad, []RequestOption{WithRequestTimeout(3 * time.Second)}, 3 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cd capturedDeadline
			c, err := NewClient(append([]Option{WithHTTPClient(cd.client())}, tc.client...)...)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if err := tc.call(c, tc.req...); err != nil {
				t.Fatalf("call: %v", err)
			}
			if !cd.ok || cd.left > tc.want || cd.left < tc.want-2*time.Second {
				t.Fatalf("time left on attempt context = %v (deadline set: %v), want just under %v", cd.left, cd.ok, tc.want)
			}
		})
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
