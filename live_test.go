package octen_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	octen "github.com/Octen-Team/octen-go"
)

// These tests call the production API and are billed: they run only with OCTEN_LIVE=1 and OCTEN_API_KEY.

func liveClient(t *testing.T, opts ...octen.Option) *octen.Client {
	t.Helper()
	key := os.Getenv("OCTEN_API_KEY")
	if os.Getenv("OCTEN_LIVE") != "1" || key == "" {
		t.Skip("live tests need OCTEN_LIVE=1 and OCTEN_API_KEY")
	}
	c, err := octen.NewClient(append([]octen.Option{octen.WithAPIKey(key)}, opts...)...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

// requireRequestID checks the envelope; the request ID is not a secret and is logged for the test record.
func requireRequestID(t *testing.T, id string, hasData bool) {
	t.Helper()
	if id == "" {
		t.Fatal("empty request_id")
	}
	if !hasData {
		t.Fatal("response has no data")
	}
	t.Logf("request_id=%s", id)
}

func TestLiveSearch(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.Search(liveContext(t), octen.SearchParams{Query: "Octen search API", Count: octen.Int(1)})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
}

func TestLiveBroadSearch(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.BroadSearch(liveContext(t), octen.BroadSearchParams{Query: "AI search APIs compared", MaxQueries: octen.Int(1)})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
}

func TestLiveNewsSearch(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.NewsSearch(liveContext(t), octen.NewsSearchParams{Query: "artificial intelligence", Count: octen.Int(1)})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
}

func TestLiveBusinessSearch(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.BusinessSearch(liveContext(t), octen.BusinessSearchParams{Query: "Apple Inc", Count: octen.Int(1)})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
}

func TestLivePulseSearch(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.PulseSearch(liveContext(t), octen.PulseSearchParams{Query: "Octen search API", Count: octen.Int(1)})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
}

func TestLivePlaceSearch(t *testing.T) {
	c := liveClient(t)
	res, err := c.PlaceSearch.Search(liveContext(t), octen.PlaceSearchParams{
		Query:     "coffee",
		Places:    &octen.PlaceCardOptions{Count: octen.Int(1)},
		WebSearch: &octen.PlaceWebSearchOptions{Enable: octen.Bool(false)},
		Latitude:  octen.Float64(35.6812),
		Longitude: octen.Float64(139.7671),
		Radius:    octen.Int(1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
}

func TestLiveSearchPremier(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.SearchPremier(liveContext(t), octen.SearchParams{Query: "Octen search API", Count: octen.Int(2), ResultType: octen.String("dynamic")})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
	if len(res.Data.Results) == 0 {
		t.Fatal("no results")
	}
	if res.Meta == nil || res.Meta.ResultType != "dynamic" {
		t.Fatalf("meta = %+v, want result_type dynamic echoed", res.Meta)
	}
}

func TestLiveImageSearchByText(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.ImageSearch(liveContext(t), octen.ImageSearchParams{
		Inputs: []octen.ImageSearchInput{{Type: "text", Data: octen.String("golden retriever puppy")}},
		Count:  octen.Int(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
	if n := len(res.Data.Results); n == 0 || n > 2 {
		t.Fatalf("results = %d, want 1 or 2", n)
	}
	for _, r := range res.Data.Results {
		if r.URL == "" || r.Width == 0 || r.Height == 0 {
			t.Errorf("incomplete result %+v", r)
		}
	}
}

// Searching by a well-known Commons photo finds that photo again, which a text query for "cat" need not.
func TestLiveImageSearchByURL(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.ImageSearch(liveContext(t), octen.ImageSearchParams{
		Inputs: []octen.ImageSearchInput{{Type: "image", URL: octen.String("https://upload.wikimedia.org/wikipedia/commons/a/a3/June_odd-eyed-cat.jpg")}},
		Count:  octen.Int(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
	found := false
	for _, r := range res.Data.Results {
		found = found || strings.Contains(strings.ToLower(r.URL+" "+r.SourcePage), "odd-eyed")
	}
	if !found {
		t.Fatalf("no result points at the input photo: %+v", res.Data.Results)
	}
}

// Video Search is an invite-only beta; a key without access must surface as ErrPermissionDenied.
func TestLiveVideoSearch(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.VideoSearch(liveContext(t), octen.VideoSearchParams{
		Inputs: []octen.VideoSearchInput{{Type: "text", Data: octen.String("how to tie a bowline knot")}},
		Count:  octen.Int(1),
	})
	if errors.Is(err, octen.ErrPermissionDenied) {
		t.Skipf("the key has no Video Search beta access: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
	if n := len(res.Data.Results); n != 1 {
		t.Fatalf("results = %d, want 1", n)
	}
	if r := res.Data.Results[0]; r.URL == "" || r.DurationSeconds <= 0 {
		t.Errorf("incomplete result %+v", r)
	}
}

func TestLiveIncludeDomains(t *testing.T) {
	c := liveClient(t)
	res, err := c.Search.Search(liveContext(t), octen.SearchParams{
		Query:          "python docs",
		Count:          octen.Int(5),
		IncludeDomains: []string{"python.org"},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, res.RequestID, res.Data != nil)
	// An empty result set would make the check below pass vacuously.
	if len(res.Data.Results) == 0 {
		t.Fatal("no results to check")
	}
	for _, r := range res.Data.Results {
		u, err := url.Parse(r.URL)
		if err != nil {
			t.Fatalf("parse %q: %v", r.URL, err)
		}
		host := strings.ToLower(u.Hostname())
		if host != "python.org" && !strings.HasSuffix(host, ".python.org") {
			t.Errorf("result %q is outside python.org", r.URL)
		}
	}
	t.Logf("checked %d results", len(res.Data.Results))
}

func TestLiveWrongAPIKey(t *testing.T) {
	liveClient(t) // gate only
	c, err := octen.NewClient(octen.WithAPIKey("octen-invalid-key-for-live-test"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Search.Search(liveContext(t), octen.SearchParams{Query: "octen", Count: octen.Int(1)})
	if !errors.Is(err, octen.ErrAuthentication) {
		t.Fatalf("want ErrAuthentication, got %v", err)
	}
	var apiErr *octen.APIError
	if errors.As(err, &apiErr) {
		t.Logf("status=%d request_id=%s", apiErr.StatusCode, apiErr.RequestID)
	}
}

func TestLiveCountZeroIsBadRequest(t *testing.T) {
	c := liveClient(t)
	_, err := c.Search.Search(liveContext(t), octen.SearchParams{Query: "octen", Count: octen.Int(0)})
	if !errors.Is(err, octen.ErrBadRequest) {
		t.Fatalf("want ErrBadRequest, got %v", err)
	}
	var apiErr *octen.APIError
	if errors.As(err, &apiErr) {
		t.Logf("status=%d request_id=%s", apiErr.StatusCode, apiErr.RequestID)
	}
}

type countingTransport struct {
	calls atomic.Int32
	next  http.RoundTripper
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return c.next.RoundTrip(r)
}

// The server needs well over 500 ms for max_queries 10; the timeout fires after the request was sent, so it must not be retried (double billing).
func TestLiveBroadSearchTimeoutIsNotRetried(t *testing.T) {
	ct := &countingTransport{next: http.DefaultTransport}
	c := liveClient(t, octen.WithHTTPClient(&http.Client{Transport: ct}))
	_, err := c.Search.BroadSearch(liveContext(t),
		octen.BroadSearchParams{Query: "history of web search engines", MaxQueries: octen.Int(10)},
		octen.WithRequestTimeout(500*time.Millisecond))
	var te *octen.TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("want *TimeoutError, got %T: %v", err, err)
	}
	t.Logf("round_trips=%d", ct.calls.Load())
	if n := ct.calls.Load(); n != 1 {
		t.Fatalf("round trips = %d, want exactly 1", n)
	}
}
