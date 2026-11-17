package octen

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type seenRequest struct {
	path   string
	method string
	body   string
	header http.Header
}

// recorder is a fake API that returns a distinct body per path, with odd spacing and an
// unmodelled field, so RawJSON() matches only if the service layer stores the received bytes.
// It reads request bodies itself, which newFakeServer discards.
type recorder struct {
	*httptest.Server
	hits atomic.Int32
	mu   sync.Mutex
	seen []seenRequest
}

func rawBodyFor(path string) string {
	return "{ \"code\" : 0,\n  \"request_id\" : \"rid" + path + "\", \"unmodelled\": [1, 2],\n \"data\" : {\"query\":\"q\"} }\n"
}

func newRecorder(t *testing.T) *recorder {
	t.Helper()
	rec := &recorder{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.seen = append(rec.seen, seenRequest{path: r.URL.Path, method: r.Method, body: string(body), header: r.Header.Clone()})
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, rawBodyFor(r.URL.Path))
	}))
	t.Cleanup(rec.Close)
	return rec
}

func (r *recorder) requests() []seenRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]seenRequest(nil), r.seen...)
}

func newTestClient(t *testing.T, url string, opts ...Option) *Client {
	t.Helper()
	c, err := NewClient(append([]Option{WithAPIKey("test-key"), WithBaseURL(url)}, opts...)...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestSixEndpointsPathsAndBodies(t *testing.T) {
	rec := newRecorder(t)
	// A trailing slash on the base URL must not double up in the path.
	c := newTestClient(t, rec.URL+"/")
	ctx := context.Background()
	count := Int(3)

	cases := []struct {
		name     string
		path     string
		wantBody string
		call     func() (rid string, raw []byte, err error)
	}{
		{"search", "/search", `{"query":"q","count":3}`, func() (string, []byte, error) {
			r, err := c.Search.Search(ctx, SearchParams{Query: "q", Count: count})
			if err != nil {
				return "", nil, err
			}
			return r.RequestID, r.RawJSON(), nil
		}},
		{"broad", "/broad-search", `{"query":"q","max_queries":3}`, func() (string, []byte, error) {
			r, err := c.Search.BroadSearch(ctx, BroadSearchParams{Query: "q", MaxQueries: count})
			if err != nil {
				return "", nil, err
			}
			return r.RequestID, r.RawJSON(), nil
		}},
		{"news", "/news-search", `{"query":"q","count":3}`, func() (string, []byte, error) {
			r, err := c.Search.NewsSearch(ctx, NewsSearchParams{Query: "q", Count: count})
			if err != nil {
				return "", nil, err
			}
			return r.RequestID, r.RawJSON(), nil
		}},
		{"business", "/business-search", `{"query":"q","count":3}`, func() (string, []byte, error) {
			r, err := c.Search.BusinessSearch(ctx, BusinessSearchParams{Query: "q", Count: count})
			if err != nil {
				return "", nil, err
			}
			return r.RequestID, r.RawJSON(), nil
		}},
		{"pulse", "/pulse-search", `{"query":"q","mode":"lite","count":3}`, func() (string, []byte, error) {
			r, err := c.Search.PulseSearch(ctx, PulseSearchParams{Query: "q", Mode: String("lite"), Count: count})
			if err != nil {
				return "", nil, err
			}
			return r.RequestID, r.RawJSON(), nil
		}},
		{"place", "/place-search", `{"query":"q","places":{"count":3},"latitude":1.5,"longitude":-2}`, func() (string, []byte, error) {
			r, err := c.PlaceSearch.Search(ctx, PlaceSearchParams{Query: "q", Places: &PlaceCardOptions{Count: count}, Latitude: Float64(1.5), Longitude: Float64(-2)})
			if err != nil {
				return "", nil, err
			}
			return r.RequestID, r.RawJSON(), nil
		}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rid, raw, err := tc.call()
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			got := rec.requests()
			if len(got) != i+1 {
				t.Fatalf("server saw %d requests, want %d", len(got), i+1)
			}
			req := got[i]
			if req.method != http.MethodPost || req.path != tc.path {
				t.Fatalf("request = %s %s, want POST %s", req.method, req.path, tc.path)
			}
			if req.body != tc.wantBody {
				t.Fatalf("body = %s, want %s", req.body, tc.wantBody)
			}
			if k := req.header.Get("X-Api-Key"); k != "test-key" {
				t.Fatalf("x-api-key = %q", k)
			}
			if ct := req.header.Get("Content-Type"); ct != "application/json" {
				t.Fatalf("content-type = %q", ct)
			}
			if rid != "rid"+tc.path {
				t.Fatalf("RequestID = %q, want %q", rid, "rid"+tc.path)
			}
			if want := rawBodyFor(tc.path); string(raw) != want {
				t.Fatalf("RawJSON() = %q, want the exact bytes sent by the server %q", raw, want)
			}
		})
	}
}

// RawJSON() must hold the bytes as received even when a BOM was skipped for decoding.
func TestRawJSONKeepsBOM(t *testing.T) {
	body := "\xef\xbb\xbf" + okBody
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { _, _ = io.WriteString(w, body) })
	c := newTestClient(t, s.URL)
	resp, err := c.Search.Search(context.Background(), SearchParams{Query: "q"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if string(resp.RawJSON()) != body || resp.RequestID != "rid-ok" {
		t.Fatalf("RawJSON() = %q, RequestID = %q", resp.RawJSON(), resp.RequestID)
	}
}

func TestRequestBodyIsNotHTMLEscaped(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	if _, err := c.Search.Search(context.Background(), SearchParams{Query: "a<b>&c"}); err != nil {
		t.Fatal(err)
	}
	if got := rec.requests()[0].body; got != `{"query":"a<b>&c"}` {
		t.Fatalf("body = %s", got)
	}
}

func TestValidationSendsNothing(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	ctx := context.Background()

	cases := []struct {
		name  string
		field string
		call  func() error
	}{
		{"search blank", "query", func() error { _, err := c.Search.Search(ctx, SearchParams{Query: " \t"}); return err }},
		{"search empty", "query", func() error { _, err := c.Search.Search(ctx, SearchParams{}); return err }},
		{"broad blank", "query", func() error { _, err := c.Search.BroadSearch(ctx, BroadSearchParams{Query: ""}); return err }},
		{"news blank", "query", func() error { _, err := c.Search.NewsSearch(ctx, NewsSearchParams{}); return err }},
		{"business blank", "query", func() error { _, err := c.Search.BusinessSearch(ctx, BusinessSearchParams{}); return err }},
		{"pulse blank", "query", func() error { _, err := c.Search.PulseSearch(ctx, PulseSearchParams{}); return err }},
		{"place blank", "query", func() error { _, err := c.PlaceSearch.Search(ctx, PlaceSearchParams{}); return err }},
		{"place NaN latitude", "latitude", func() error {
			_, err := c.PlaceSearch.Search(ctx, PlaceSearchParams{Query: "q", Latitude: Float64(math.NaN())})
			return err
		}},
		{"place +Inf longitude", "longitude", func() error {
			_, err := c.PlaceSearch.Search(ctx, PlaceSearchParams{Query: "q", Longitude: Float64(math.Inf(1))})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("err = %#v, want *ValidationError on %q", err, tc.field)
			}
		})
	}
	if got := rec.hits.Load(); got != 0 {
		t.Fatalf("server saw %d requests, want 0", got)
	}
}

func TestDecodeFailureIsAPIError(t *testing.T) {
	const body = `{"code":0,"request_id":"rid-bad","data":{"results":"not an array"}}`
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
	c := newTestClient(t, s.URL)
	resp, err := c.Search.Search(context.Background(), SearchParams{Query: "q"})
	if resp != nil {
		t.Fatalf("resp = %v, want nil", resp)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %#v, want *APIError", err)
	}
	if !strings.HasPrefix(apiErr.Message, "decode response:") {
		t.Fatalf("Message = %q, want prefix %q", apiErr.Message, "decode response:")
	}
	if string(apiErr.Body) != body || apiErr.StatusCode != 200 || apiErr.RequestID != "rid-bad" {
		t.Fatalf("got body %q status %d request id %q", apiErr.Body, apiErr.StatusCode, apiErr.RequestID)
	}
	for _, sentinel := range []error{ErrAuthentication, ErrPermissionDenied, ErrBadRequest, ErrRateLimit, ErrServer} {
		if errors.Is(err, sentinel) {
			t.Fatalf("decode failure matches sentinel %v", sentinel)
		}
	}
	if got := s.hits.Load(); got != 1 {
		t.Fatalf("server saw %d requests, want 1 (not retried)", got)
	}
}

func TestAPIErrorsPassThrough(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"msg":"bad key"}`)
	})
	c := newTestClient(t, s.URL)
	_, err := c.PlaceSearch.Search(context.Background(), PlaceSearchParams{Query: "q"})
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("err = %v, want ErrAuthentication", err)
	}
}

func TestClientHeadersAndRequestHeaders(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL,
		WithHeader("X-Team", "one"), WithHeader("x-team", "two"), WithHeader("X-Keep", "k"))
	_, err := c.Search.Search(context.Background(), SearchParams{Query: "q"},
		WithRequestHeader("X-Keep", "override"), WithRequestHeader("X-Extra", "e"))
	if err != nil {
		t.Fatal(err)
	}
	h := rec.requests()[0].header
	if h.Get("X-Team") != "two" || h.Get("X-Keep") != "override" || h.Get("X-Extra") != "e" {
		t.Fatalf("headers = %v", h)
	}
	if ua := h.Get("User-Agent"); ua != "octen-go-sdk/"+Version {
		t.Fatalf("User-Agent = %q", ua)
	}
}

func TestMaxRetriesOption(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.WriteHeader(500) })
	c := newTestClient(t, s.URL, WithMaxRetries(0))
	if _, err := c.Search.Search(context.Background(), SearchParams{Query: "q"}); !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	if got := s.hits.Load(); got != 1 {
		t.Fatalf("server saw %d requests, want 1", got)
	}
}

func TestCallerHTTPClientIsHonoured(t *testing.T) {
	var used bool
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return nil, errors.New("boom")
	})}
	c := newTestClient(t, "http://example.invalid", WithHTTPClient(hc), WithMaxRetries(0))
	_, err := c.Search.Search(context.Background(), SearchParams{Query: "q"})
	var ce *ConnectionError
	if !errors.As(err, &ce) || !used {
		t.Fatalf("err = %#v, used = %v; want *ConnectionError through the caller's client", err, used)
	}
}

func TestParamsJSONMatchesMarshal(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	p := SearchParams{
		Query: "q", Count: Int(5), IncludeDomains: []string{"a.com"}, Language: []string{"en"},
		Highlight: &HighlightOptions{Enable: Bool(true), MaxTokens: Int(100)}, Safesearch: String("strict"),
	}
	if _, err := c.Search.Search(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(p)
	if got := rec.requests()[0].body; got != string(want) {
		t.Fatalf("body = %s, want %s", got, want)
	}
}
