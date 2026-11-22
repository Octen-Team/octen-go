package octen

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// Every Extract parameter reaches the wire under the name in the API reference
// (https://docs.octen.ai/api-reference/extract), each with a value no other field uses.
func TestExtractSendsEveryParameter(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	ctx := context.Background()
	full := ExtractParams{
		URLs: []string{"https://a.example/1", "https://b.example/2"}, Mode: String("advanced"), Query: String("pricing"),
		MaxAgeSeconds: Int(3600), Format: String("text"), Timeout: Int(45),
		IncludeImages: Bool(true), IncludeVideos: Bool(false), IncludeAudio: Bool(true),
		IncludeLinks: &ExtractLinksOptions{Scope: String("prefer_external"), MaxLinks: Int(7)},
	}
	cases := []struct {
		name, want string
		params     ExtractParams
	}{
		{"every field", `{"urls":["https://a.example/1","https://b.example/2"],"mode":"advanced","query":"pricing",` +
			`"max_age_seconds":3600,"format":"text","timeout":45,"include_images":true,"include_videos":false,` +
			`"include_audio":true,"include_links":{"scope":"prefer_external","max_links":7}}`, full},
		{"unset fields omitted", `{"urls":["https://a.example/1"]}`, ExtractParams{URLs: []string{"https://a.example/1"}}},
		{"empty include_links sent as {}", `{"urls":["https://a.example/1"],"include_links":{}}`,
			ExtractParams{URLs: []string{"https://a.example/1"}, IncludeLinks: &ExtractLinksOptions{}}},
		{"zero values sent when set", `{"urls":["https://a.example/1"],"query":"","max_age_seconds":0,"timeout":0,"include_images":false}`,
			ExtractParams{URLs: []string{"https://a.example/1"}, Query: String(""), MaxAgeSeconds: Int(0), Timeout: Int(0), IncludeImages: Bool(false)}},
		{"mode auto, scope only", `{"urls":["https://a.example/1"],"mode":"auto","include_links":{"scope":"prefer_internal"}}`,
			ExtractParams{URLs: []string{"https://a.example/1"}, Mode: String("auto"), IncludeLinks: &ExtractLinksOptions{Scope: String("prefer_internal")}}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := c.Extract.Extract(ctx, tc.params)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			got := rec.requests()
			if len(got) != i+1 {
				t.Fatalf("server saw %d requests, want %d", len(got), i+1)
			}
			if r := got[i]; r.method != http.MethodPost || r.path != "/extract" || r.body != tc.want {
				t.Fatalf("request = %s %s %s\nwant      POST /extract %s", r.method, r.path, r.body, tc.want)
			}
			if resp.RequestID != "rid/extract" || string(resp.RawJSON()) != rawBodyFor("/extract") {
				t.Fatalf("RequestID = %q, RawJSON = %q", resp.RequestID, resp.RawJSON())
			}
		})
	}
}

func TestExtractValidation(t *testing.T) {
	urls := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "https://example.com/" + strings.Repeat("p", i)
		}
		return out
	}
	// 2048 counts code points like the TypeScript SDK; "é" is two bytes in UTF-8.
	long := func(runes int, r string) string {
		prefix := "https://example.com/"
		return prefix + strings.Repeat(r, runes-len(prefix))
	}
	cases := []struct {
		name  string
		urls  []string
		field string // empty when valid
	}{
		{"nil urls", nil, "urls"},
		{"empty urls", []string{}, "urls"},
		{"one url", urls(1), ""},
		{"20 urls", urls(20), ""},
		{"21 urls", urls(21), "urls"},
		{"empty url", []string{"https://example.com/", ""}, "urls[1]"},
		{"blank url", []string{" \t\n"}, "urls[0]"},
		{"url of 2048 ASCII characters", []string{long(2048, "a")}, ""},
		{"url of 2049 ASCII characters", []string{long(2049, "a")}, "urls[0]"},
		{"url of 2048 code points, 4076 bytes", []string{long(2048, "é")}, ""},
		{"url of 2049 code points", []string{"https://example.com/", long(2049, "é")}, "urls[1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateParams(ExtractParams{URLs: tc.urls})
			if tc.field == "" {
				if err != nil {
					t.Fatalf("valid params rejected: %v", err)
				}
				return
			}
			wantValidationError(t, err, tc.field)
		})
	}
	wantValidationError(t, validateParams(&ExtractParams{}), "urls")
}

func TestExtractValidationSendsNothing(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = "https://example.com/"
	}
	for _, tc := range []struct {
		name, field string
		p           ExtractParams
	}{
		{"no urls", "urls", ExtractParams{}},
		{"21 urls", "urls", ExtractParams{URLs: tooMany}},
		{"blank url", "urls[1]", ExtractParams{URLs: []string{"https://example.com/", " "}}},
		{"2049 chars", "urls[0]", ExtractParams{URLs: []string{strings.Repeat("a", 2049)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := c.Extract.Extract(context.Background(), tc.p)
			if resp != nil {
				t.Fatalf("resp = %v, want nil", resp)
			}
			wantValidationError(t, err, tc.field)
		})
	}
	if got := rec.hits.Load(); got != 0 {
		t.Fatalf("server saw %d requests, want 0", got)
	}
}

// The body follows the response example of the API reference, plus a failed item and the null
// values the reference documents for title, page_structure, category and cover_image.
const extractBody = `{"code":0,"msg":"success","request_id":"req_abc123def456","data":{"results":[
 {"url":"https://octen.ai/","status":"success","resolved_mode":"standard","title":"Octen","full_content":"# Search",
  "highlights":null,"time_published":null,"time_last_crawled":"2026-07-03T09:35:43Z",
  "page_structure":{"primary":"Index Page","secondary":"Home Page"},
  "category":{"primary":"Computers, Electronics & Technology","secondary":null},
  "favicon":"https://octen.ai/favicon.ico","cover_image":{"url":"https://octen.ai/cover.png"},
  "images":[{"url":"https://octen.ai/a.png"},{"url":"https://octen.ai/b.png"}],
  "videos":[{"url":"https://octen.ai/v.mp4"}],"audio":[{"url":"https://octen.ai/a.mp3"}],
  "links":[{"url":"https://octen.ai/pricing","anchor_text":"Pricing","is_external":false},
           {"url":"https://github.com/Octen-Team","anchor_text":"","is_external":true}]},
 {"url":"https://docs.octen.ai/x","status":"success","resolved_mode":"advanced","title":null,"full_content":null,
  "highlights":["first","second"],"time_published":"2024-10-15T00:00:00Z","time_last_crawled":"2026-07-03T09:34:29Z",
  "page_structure":null,"category":null,"favicon":null,"cover_image":null},
 {"url":"https://broken.invalid/","status":"failed","error_message":"Failed to fetch the URL"}]},
 "meta":{"usage":{"total_urls":3,"successful_urls":2,"successful_by_mode":{"standard_urls":1,"advanced_urls":1}},"latency":1832,"warning":""}}`

func TestExtractResponseDecodes(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, extractBody)
	})
	c := newTestClient(t, s.URL)
	resp, err := c.Extract.Extract(context.Background(), ExtractParams{URLs: []string{"https://octen.ai/"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.RequestID != "req_abc123def456" || resp.Msg == nil || *resp.Msg != "success" || string(resp.RawJSON()) != extractBody {
		t.Fatalf("envelope: %q %v", resp.RequestID, resp.Msg)
	}
	rs := resp.Data.Results
	if len(rs) != 3 {
		t.Fatalf("results = %d, want 3", len(rs))
	}
	ok := rs[0]
	if ok.URL != "https://octen.ai/" || ok.Status != "success" || ok.ResolvedMode != "standard" || *ok.Title != "Octen" ||
		*ok.FullContent != "# Search" || ok.Highlights != nil || ok.TimePublished != nil || ok.TimeLastCrawled != "2026-07-03T09:35:43Z" ||
		*ok.Favicon != "https://octen.ai/favicon.ico" || ok.ErrorMessage != "" {
		t.Errorf("first result: %+v", ok)
	}
	if ok.PageStructure.Primary != "Index Page" || *ok.PageStructure.Secondary != "Home Page" ||
		ok.Category.Primary != "Computers, Electronics & Technology" || ok.Category.Secondary != nil {
		t.Errorf("classification: %+v %+v", ok.PageStructure, ok.Category)
	}
	if ok.CoverImage.URL != "https://octen.ai/cover.png" || len(ok.Images) != 2 || ok.Images[1].URL != "https://octen.ai/b.png" ||
		ok.Videos[0].URL != "https://octen.ai/v.mp4" || ok.Audio[0].URL != "https://octen.ai/a.mp3" {
		t.Errorf("media: %+v", ok)
	}
	wantLinks := []ExtractLink{
		{URL: "https://octen.ai/pricing", AnchorText: "Pricing", IsExternal: false},
		{URL: "https://github.com/Octen-Team", AnchorText: "", IsExternal: true},
	}
	if !reflect.DeepEqual(ok.Links, wantLinks) {
		t.Errorf("links = %+v", ok.Links)
	}
	hl := rs[1]
	if hl.ResolvedMode != "advanced" || hl.Title != nil || hl.FullContent != nil || !reflect.DeepEqual(hl.Highlights, []string{"first", "second"}) ||
		*hl.TimePublished != "2024-10-15T00:00:00Z" || hl.PageStructure != nil || hl.Category != nil || hl.Favicon != nil ||
		hl.CoverImage != nil || hl.Images != nil || hl.Links != nil {
		t.Errorf("highlights result: %+v", hl)
	}
	failed := rs[2]
	if failed.Status != "failed" || failed.ErrorMessage != "Failed to fetch the URL" || failed.ResolvedMode != "" || failed.FullContent != nil {
		t.Errorf("failed result: %+v", failed)
	}
	u := resp.Meta.Usage
	if u.TotalURLs != 3 || u.SuccessfulURLs != 2 || u.SuccessfulByMode.StandardURLs != 1 || u.SuccessfulByMode.AdvancedURLs != 1 ||
		resp.Meta.Latency != 1832 || resp.Meta.Warning == nil || *resp.Meta.Warning != "" {
		t.Errorf("meta: %+v usage %+v", resp.Meta, u)
	}
}

func TestExtractErrorMapping(t *testing.T) {
	cases := []struct {
		status   int
		body     string
		sentinel error
	}{
		{400, `{"code":400,"msg":"Invalid params. Missing parameter urls","request_id":"req_400"}`, ErrBadRequest},
		{401, `{"code":401,"msg":"Invalid API Key","request_id":"req_401"}`, ErrAuthentication},
		{403, `{"code":403,"msg":"Insufficient balance in account","request_id":"req_403"}`, ErrPermissionDenied},
	}
	for _, tc := range cases {
		s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		})
		c := newTestClient(t, s.URL)
		resp, err := c.Extract.Extract(context.Background(), ExtractParams{URLs: []string{"https://example.com/"}})
		var apiErr *APIError
		if resp != nil || !errors.Is(err, tc.sentinel) || !errors.As(err, &apiErr) {
			t.Fatalf("%d: resp %v err %v", tc.status, resp, err)
		}
		var want struct {
			Msg       string `json:"msg"`
			RequestID string `json:"request_id"`
		}
		_ = json.Unmarshal([]byte(tc.body), &want)
		if apiErr.StatusCode != tc.status || apiErr.Message != want.Msg || apiErr.RequestID != want.RequestID {
			t.Errorf("%d: %+v", tc.status, apiErr)
		}
		if got := s.hits.Load(); got != 1 {
			t.Errorf("%d: server saw %d requests, want 1", tc.status, got)
		}
	}
}

// The default HTTP timeout follows the per-URL timeout in the body: clamped to the documented 1-60 s,
// 30 s when unset, plus 90 s of headroom.
func TestExtractDefaultTimeout(t *testing.T) {
	for _, tc := range []struct {
		perURL *int
		want   time.Duration
	}{
		{nil, 120 * time.Second},
		{Int(30), 120 * time.Second},
		{Int(1), 91 * time.Second},
		{Int(60), 150 * time.Second},
		{Int(45), 135 * time.Second},
		{Int(0), 91 * time.Second},
		{Int(-5), 91 * time.Second},
		{Int(61), 150 * time.Second},
		{Int(math.MaxInt32), 150 * time.Second},
	} {
		if got := extractTimeout(tc.perURL); got != tc.want {
			t.Errorf("extractTimeout(%v) = %v, want %v", deref(tc.perURL), got, tc.want)
		}
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// The effective timeout is read off the attempt context like TestTimeoutPrecedence does.
func TestExtractTimeoutPrecedence(t *testing.T) {
	setKey(t)
	params := func(perURL *int) ExtractParams {
		return ExtractParams{URLs: []string{"https://example.com/"}, Timeout: perURL}
	}
	cases := []struct {
		name   string
		client []Option
		params ExtractParams
		req    []RequestOption
		want   time.Duration
	}{
		{"default", nil, params(nil), nil, 120 * time.Second},
		{"derived from timeout 60", nil, params(Int(60)), nil, 150 * time.Second},
		{"derived from timeout 5", nil, params(Int(5)), nil, 95 * time.Second},
		{"explicit client timeout beats derived", []Option{WithTimeout(30 * time.Second)}, params(Int(60)), nil, 30 * time.Second},
		{"per-request beats derived", nil, params(Int(60)), []RequestOption{WithRequestTimeout(7 * time.Second)}, 7 * time.Second},
		{"per-request beats client", []Option{WithTimeout(30 * time.Second)}, params(nil), []RequestOption{WithRequestTimeout(200 * time.Second)}, 200 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cd capturedDeadline
			c, err := NewClient(append([]Option{WithHTTPClient(cd.client())}, tc.client...)...)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Extract.Extract(context.Background(), tc.params, tc.req...); err != nil {
				t.Fatal(err)
			}
			if !cd.ok || cd.left > tc.want || cd.left < tc.want-2*time.Second {
				t.Fatalf("time left on attempt context = %v (deadline set: %v), want just under %v", cd.left, cd.ok, tc.want)
			}
		})
	}
	t.Run("a shorter caller deadline wins", func(t *testing.T) {
		var cd capturedDeadline
		c, err := NewClient(WithHTTPClient(cd.client()))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := c.Extract.Extract(ctx, params(Int(60))); err != nil {
			t.Fatal(err)
		}
		if !cd.ok || cd.left > 10*time.Second || cd.left < 8*time.Second {
			t.Fatalf("time left = %v, want just under the caller's 10s", cd.left)
		}
	})
}

// Extract follows the shared retry policy: retryable statuses are retried, other 4xx and an SDK
// timeout are not, and WithMaxRetries(0) sends once.
func TestExtractRetryPolicy(t *testing.T) {
	p := ExtractParams{URLs: []string{"https://example.com/"}}
	t.Run("503 then success, same body", func(t *testing.T) {
		var mu sync.Mutex
		var bodies []string
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, string(b))
			n := len(bodies)
			mu.Unlock()
			if n == 1 {
				w.WriteHeader(503)
				return
			}
			writeOK(w)
		}))
		t.Cleanup(s.Close)
		c, rec := newDefaultClient(t, s.URL)
		full := ExtractParams{URLs: []string{"https://a.example/", "https://b.example/"}, Mode: String("advanced"),
			Query: String("q"), IncludeLinks: &ExtractLinksOptions{}}
		if _, err := c.Extract.Extract(context.Background(), full); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(bodies) != 2 || len(rec.got()) != 1 {
			t.Fatalf("requests %d waits %v, want 2 and one wait", len(bodies), rec.got())
		}
		if bodies[0] == "" || bodies[1] != bodies[0] {
			t.Fatalf("retried body %q differs from the first %q", bodies[1], bodies[0])
		}
	})
	t.Run("400 not retried", func(t *testing.T) {
		s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.WriteHeader(400) })
		c, _ := newDefaultClient(t, s.URL)
		if _, err := c.Extract.Extract(context.Background(), p); !errors.Is(err, ErrBadRequest) || s.hits.Load() != 1 {
			t.Fatalf("err %v hits %d", err, s.hits.Load())
		}
	})
	t.Run("max retries 0 sends once", func(t *testing.T) {
		s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.WriteHeader(502) })
		c, _ := newDefaultClient(t, s.URL, WithMaxRetries(0))
		if _, err := c.Extract.Extract(context.Background(), p); !errors.Is(err, ErrServer) || s.hits.Load() != 1 {
			t.Fatalf("err %v hits %d", err, s.hits.Load())
		}
	})
	t.Run("timeout not retried", func(t *testing.T) {
		s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
			stallUntilGone(r, 2*time.Second)
			writeOK(w)
		})
		c, _ := newDefaultClient(t, s.URL)
		_, err := c.Extract.Extract(context.Background(), p, WithRequestTimeout(50*time.Millisecond))
		var te *TimeoutError
		if !errors.As(err, &te) || s.hits.Load() != 1 {
			t.Fatalf("err %#v hits %d, want one *TimeoutError attempt", err, s.hits.Load())
		}
	})
}
