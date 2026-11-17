package octen_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	octen "github.com/Octen-Team/octen-go"
)

// fakeAPI starts a local server that answers every request with status and body, so the
// examples run offline with a fixed output. Real code only needs the API key.
func fakeAPI(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

func newExampleClient(srv *httptest.Server, opts ...octen.Option) *octen.Client {
	opts = append([]octen.Option{octen.WithAPIKey("example-key"), octen.WithBaseURL(srv.URL)}, opts...)
	client, err := octen.NewClient(opts...)
	if err != nil {
		panic(err)
	}
	return client
}

func ExampleSearchService_Search() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-1","data":{"query":"golang","results":[{"title":"The Go Programming Language","url":"https://go.dev"}]}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.Search.Search(context.Background(), octen.SearchParams{
		Query: "golang",
		Count: octen.Int(1),
	})
	if err != nil {
		panic(err)
	}
	for _, r := range resp.Data.Results {
		fmt.Println(r.Title, r.URL)
	}
	// Output: The Go Programming Language https://go.dev
}

func ExampleSearchService_BroadSearch() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-2","data":{"query":"ev market","queries":["ev sales 2025","ev battery prices"],"search_results":[{"query":"ev sales 2025","results":[{"title":"EV sales","url":"https://example.com/sales"}]}]}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.Search.BroadSearch(context.Background(), octen.BroadSearchParams{Query: "ev market"})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Data.Queries)
	fmt.Println(resp.Data.SearchResults[0].Results[0].URL)
	// Output:
	// [ev sales 2025 ev battery prices]
	// https://example.com/sales
}

func ExampleSearchService_NewsSearch() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-3","data":{"query":"chips","results":[{"title":"Chip makers rally","url":"https://news.example.com/chips"}]}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.Search.NewsSearch(context.Background(), octen.NewsSearchParams{
		Query:     "chips",
		TimeRange: octen.String("day"),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Data.Results[0].Title)
	// Output: Chip makers rally
}

func ExampleSearchService_BusinessSearch() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-4","data":{"query":"Acme Corp","results":[{"title":"Acme Corp overview","url":"https://example.com/acme"}]}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.Search.BusinessSearch(context.Background(), octen.BusinessSearchParams{Query: "Acme Corp"})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Data.Results[0].Title)
	// Output: Acme Corp overview
}

func ExampleSearchService_PulseSearch() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-5","data":{"query":"latest release","results":[{"title":"Release notes","url":"https://example.com/release"}]}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.Search.PulseSearch(context.Background(), octen.PulseSearchParams{
		Query: "latest release",
		Mode:  octen.String("lite"),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(*resp.Data.Results[0].Title)
	// Output: Release notes
}

func ExamplePlaceSearchService_Search() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-6","data":{"query":"coffee","places":[{"name":"Blue Bottle","address":"1 Main St","rating":4.5}]}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.PlaceSearch.Search(context.Background(), octen.PlaceSearchParams{
		Query:     "coffee",
		Latitude:  octen.Float64(37.77),
		Longitude: octen.Float64(-122.42),
	})
	if err != nil {
		panic(err)
	}
	p := resp.Data.Places[0]
	fmt.Println(p.Name, p.Rating)
	// Output: Blue Bottle 4.5
}

func Example_errorHandling() {
	srv := fakeAPI(401, `{"msg":"invalid api key","request_id":"req-7"}`)
	defer srv.Close()
	client := newExampleClient(srv)
	ctx := context.Background()

	// Bad input is rejected locally; nothing is sent.
	_, err := client.Search.Search(ctx, octen.SearchParams{Query: "   "})
	var verr *octen.ValidationError
	if errors.As(err, &verr) {
		fmt.Println("validation:", verr.Field)
	}

	// Sentinels match the HTTP status; errors.As gives the details.
	_, err = client.Search.Search(ctx, octen.SearchParams{Query: "golang"})
	if errors.Is(err, octen.ErrAuthentication) {
		var apiErr *octen.APIError
		if errors.As(err, &apiErr) {
			fmt.Println("api error:", apiErr.StatusCode, apiErr.Message, apiErr.RequestID)
		}
	}

	// Check *TimeoutError before context.DeadlineExceeded to tell the SDK timeout apart
	// from a deadline on your own context.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	slowClient := newExampleClient(slow, octen.WithMaxRetries(0))
	_, err = slowClient.Search.Search(ctx, octen.SearchParams{Query: "golang"}, octen.WithRequestTimeout(50*time.Millisecond))
	var terr *octen.TimeoutError
	if errors.As(err, &terr) {
		fmt.Println("timeout after", terr.Timeout)
	}
	// Output:
	// validation: query
	// api error: 401 invalid api key req-7
	// timeout after 50ms
}
