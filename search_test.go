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

func TestEndpointPathsAndBodies(t *testing.T) {
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
		}}, {"premier", "/search-premier", `{"query":"q","result_type":"dynamic","count":3}`, func() (string, []byte, error) {
			r, err := c.Search.SearchPremier(ctx, SearchParams{Query: "q", ResultType: String("dynamic"), Count: count})
			if err != nil {
				return "", nil, err
			}
			return r.RequestID, r.RawJSON(), nil
		}},
		{"image", "/image-search", `{"inputs":[{"type":"text","data":"q"}],"count":3}`, func() (string, []byte, error) {
			r, err := c.Search.ImageSearch(ctx, ImageSearchParams{Inputs: []ImageSearchInput{{Type: "text", Data: String("q")}}, Count: count})
			if err != nil {
				return "", nil, err
			}
			return r.RequestID, r.RawJSON(), nil
		}},
		{"video", "/video-search", `{"inputs":[{"type":"text","data":"q"}],"count":3}`, func() (string, []byte, error) {
			r, err := c.Search.VideoSearch(ctx, VideoSearchParams{Inputs: []VideoSearchInput{{Type: "text", Data: String("q")}}, Count: count})
			if err != nil {
				return "", nil, err
			}
			return r.RequestID, r.RawJSON(), nil
		}},
		{"design", "/design-search", `{"query":"q","images":{"count":3}}`, func() (string, []byte, error) {
			r, err := c.Search.DesignSearch(ctx, DesignSearchParams{Query: String("q"), Images: &DesignImageOptions{Count: count}})
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
		}}, {"premier blank", "query", func() error { _, err := c.Search.SearchPremier(ctx, SearchParams{Query: " "}); return err }},
		{"image no inputs", "inputs", func() error { _, err := c.Search.ImageSearch(ctx, ImageSearchParams{}); return err }},
		{"image two inputs", "inputs", func() error {
			_, err := c.Search.ImageSearch(ctx, ImageSearchParams{Inputs: []ImageSearchInput{
				{Type: "text", Data: String("cats")}, {Type: "image", URL: String("https://example.com/a.jpg")},
			}})
			return err
		}},
		{"image blank text", "inputs[0].data", func() error {
			_, err := c.Search.ImageSearch(ctx, ImageSearchParams{Inputs: []ImageSearchInput{{Type: "text", Data: String(" ")}}})
			return err
		}},
		{"image without url or data", "inputs[0]", func() error {
			_, err := c.Search.ImageSearch(ctx, ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image"}}})
			return err
		}},
		{"image url and data", "inputs[0]", func() error {
			_, err := c.Search.ImageSearch(ctx, ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image", URL: String("https://example.com/a.jpg"), Data: String("aGVsbG8=")}}})
			return err
		}},
		{"image unknown type", "inputs[0].type", func() error {
			_, err := c.Search.ImageSearch(ctx, ImageSearchParams{Inputs: []ImageSearchInput{{Type: "video", URL: String("https://example.com/a.mp4")}}})
			return err
		}},
		{"video image type", "inputs[0].type", func() error {
			_, err := c.Search.VideoSearch(ctx, VideoSearchParams{Inputs: []VideoSearchInput{{Type: "image", Data: String("knots")}}})
			return err
		}},
		{"video no inputs", "inputs", func() error { _, err := c.Search.VideoSearch(ctx, VideoSearchParams{}); return err }},
		{"video two inputs", "inputs", func() error {
			_, err := c.Search.VideoSearch(ctx, VideoSearchParams{Inputs: []VideoSearchInput{{Type: "text", Data: String("a")}, {Type: "text", Data: String("b")}}})
			return err
		}},
		{"video blank text", "inputs[0].data", func() error {
			_, err := c.Search.VideoSearch(ctx, VideoSearchParams{Inputs: []VideoSearchInput{{Type: "text"}}})
			return err
		}},
		{"design nothing", "query", func() error { _, err := c.Search.DesignSearch(ctx, DesignSearchParams{}); return err }},
		{"design blank query and group", "query", func() error {
			_, err := c.Search.DesignSearch(ctx, DesignSearchParams{Query: String(" "), Images: &DesignImageOptions{GroupID: String("")}})
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

// Every parameter of the new endpoints reaches the wire under its documented name, each with a
// value no other field uses, and nothing the caller left unset is sent.
func TestNewEndpointsSendEveryParameter(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	ctx := context.Background()
	premier := SearchParams{
		Query: "premier q", ResultType: String("dynamic"), Count: Int(7),
		IncludeDomains: []string{"inc.example"}, ExcludeDomains: []string{"exc.example"},
		IncludeText: []string{"must"}, ExcludeText: []string{"mustnot"},
		TimeBasis: String("published"), TimeRange: String("week"),
		StartTime: String("2026-01-01T00:00:00Z"), EndTime: String("2026-02-01T00:00:00Z"),
		Language:    []string{"de"},
		Highlight:   &HighlightOptions{Enable: Bool(true), MaxTokens: Int(111)},
		FullContent: &FullContentOptions{Enable: Bool(false), MaxTokens: Int(222)},
		Format:      String("markdown"), Safesearch: String("off"), IncludeImages: Bool(true),
	}
	cases := []struct {
		name, path, want string
		call             func() error
	}{
		{"premier", "/search-premier",
			`{"query":"premier q","result_type":"dynamic","count":7,"include_domains":["inc.example"],"exclude_domains":["exc.example"],` +
				`"include_text":["must"],"exclude_text":["mustnot"],"time_basis":"published","time_range":"week",` +
				`"start_time":"2026-01-01T00:00:00Z","end_time":"2026-02-01T00:00:00Z","language":["de"],` +
				`"highlight":{"enable":true,"max_tokens":111},"full_content":{"enable":false,"max_tokens":222},` +
				`"format":"markdown","safesearch":"off","include_images":true}`,
			func() error { _, err := c.Search.SearchPremier(ctx, premier); return err }},
		{"premier minimal", "/search-premier", `{"query":"q"}`,
			func() error { _, err := c.Search.SearchPremier(ctx, SearchParams{Query: "q"}); return err }},
		{"image by url", "/image-search",
			`{"inputs":[{"type":"image","url":"https://example.com/cat.jpg"}],"count":4,"include_domains":["inc.example"],"exclude_domains":["exc.example"],"safesearch":"off"}`,
			func() error {
				_, err := c.Search.ImageSearch(ctx, ImageSearchParams{
					Inputs: []ImageSearchInput{{Type: "image", URL: String("https://example.com/cat.jpg")}},
					Count:  Int(4), IncludeDomains: []string{"inc.example"}, ExcludeDomains: []string{"exc.example"}, Safesearch: String("off"),
				})
				return err
			}},
		{"image by base64 data", "/image-search", `{"inputs":[{"type":"image","data":"aGVsbG8="}]}`,
			func() error {
				_, err := c.Search.ImageSearch(ctx, ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image", Data: String("aGVsbG8=")}}})
				return err
			}},
		{"image by text, defaults not sent", "/image-search", `{"inputs":[{"type":"text","data":"cats"}]}`,
			func() error {
				_, err := c.Search.ImageSearch(ctx, ImageSearchParams{Inputs: []ImageSearchInput{{Type: "text", Data: String("cats")}}})
				return err
			}},
		{"video", "/video-search",
			`{"inputs":[{"type":"text","data":"knots"}],"count":6,"time_range":"m","start_time":"2025-01-01T00:00:00Z","end_time":"2025-06-01T00:00:00Z","safesearch":"off"}`,
			func() error {
				_, err := c.Search.VideoSearch(ctx, VideoSearchParams{
					Inputs: []VideoSearchInput{{Type: "text", Data: String("knots")}}, Count: Int(6), TimeRange: String("m"),
					StartTime: String("2025-01-01T00:00:00Z"), EndTime: String("2025-06-01T00:00:00Z"), Safesearch: String("off"),
				})
				return err
			}},
		{"video minimal", "/video-search", `{"inputs":[{"type":"text","data":"knots"}]}`,
			func() error {
				_, err := c.Search.VideoSearch(ctx, VideoSearchParams{Inputs: []VideoSearchInput{{Type: "text", Data: String("knots")}}})
				return err
			}},
		{"design", "/design-search",
			`{"query":"hero","images":{"enable":true,"count":3,"design_facts":false,"snippet":{"enable":false,"max_tokens":333},"group_id":"pg_1"},` +
				`"web_search":{"enable":false,"count":9,"highlight":{"enable":true,"max_tokens":444},"full_content":{"enable":true,"max_tokens":555},` +
				`"include_domains":["inc.example"],"exclude_domains":["exc.example"],"language":["fr"],"time_basis":"crawled","time_range":"y",` +
				`"start_time":"2024-01-01T00:00:00Z","end_time":"2024-12-31T00:00:00Z"}}`,
			func() error {
				_, err := c.Search.DesignSearch(ctx, DesignSearchParams{
					Query: String("hero"),
					Images: &DesignImageOptions{Enable: Bool(true), Count: Int(3), DesignFacts: Bool(false),
						Snippet: &DesignSnippetOptions{Enable: Bool(false), MaxTokens: Int(333)}, GroupID: String("pg_1")},
					WebSearch: &DesignWebSearchOptions{Enable: Bool(false), Count: Int(9),
						Highlight:      &HighlightOptions{Enable: Bool(true), MaxTokens: Int(444)},
						FullContent:    &FullContentOptions{Enable: Bool(true), MaxTokens: Int(555)},
						IncludeDomains: []string{"inc.example"}, ExcludeDomains: []string{"exc.example"}, Language: []string{"fr"},
						TimeBasis: String("crawled"), TimeRange: String("y"),
						StartTime: String("2024-01-01T00:00:00Z"), EndTime: String("2024-12-31T00:00:00Z")},
				})
				return err
			}},
		{"design group only, no query sent", "/design-search", `{"images":{"group_id":"pg_1"}}`,
			func() error {
				_, err := c.Search.DesignSearch(ctx, DesignSearchParams{Images: &DesignImageOptions{GroupID: String("pg_1")}})
				return err
			}},
		{"design query only", "/design-search", `{"query":"hero"}`,
			func() error {
				_, err := c.Search.DesignSearch(ctx, DesignSearchParams{Query: String("hero")})
				return err
			}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			got := rec.requests()
			if len(got) != i+1 {
				t.Fatalf("server saw %d requests, want %d", len(got), i+1)
			}
			if got[i].path != tc.path || got[i].body != tc.want {
				t.Fatalf("request = %s %s\nwant      %s %s", got[i].path, got[i].body, tc.path, tc.want)
			}
		})
	}
}

// Premier takes the /search request type, so the two bodies must be byte-identical.
func TestSearchPremierBodyMatchesSearch(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	p := SearchParams{Query: "q", Count: Int(2), IncludeDomains: []string{"a.example"}, ResultType: String("standard")}
	if _, err := c.Search.Search(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Search.SearchPremier(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := rec.requests()
	if got[0].path != "/search" || got[1].path != "/search-premier" || got[0].body != got[1].body {
		t.Fatalf("search %s %s, premier %s %s", got[0].path, got[0].body, got[1].path, got[1].body)
	}
}
