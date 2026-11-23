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

func ExampleSearchService_SearchPremier() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-p","data":{"query":"eu ai act","results":[{"title":"AI Act","url":"https://example.eu/ai-act"}]},"meta":{"result_type":"dynamic"}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.Search.SearchPremier(context.Background(), octen.SearchParams{
		Query:      "eu ai act",
		ResultType: octen.String("dynamic"),
		Count:      octen.Int(3),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Meta.ResultType, resp.Data.Results[0].URL)
	// Output: dynamic https://example.eu/ai-act
}

func ExampleImageSearchService_Search() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-i","data":{"results":[{"title":"Golden retriever","url":"https://example.com/dog.jpg","width":1024,"height":683}]}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	// One input per call: a text query, or {Type: "image", URL: octen.String("https://...")}.
	resp, err := client.ImageSearch.Search(context.Background(), octen.ImageSearchParams{
		Inputs: []octen.ImageSearchInput{{Type: "text", Data: octen.String("golden retriever")}},
		Count:  octen.Int(1),
	})
	if err != nil {
		panic(err)
	}
	r := resp.Data.Results[0]
	fmt.Println(r.Title, r.Width, r.Height)
	// Output: Golden retriever 1024 683
}

func ExampleVideoSearchService_Search() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-v","data":{"results":[{"title":"Bowline knot","url":"https://example.com/watch","duration_seconds":78}]}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.VideoSearch.Search(context.Background(), octen.VideoSearchParams{
		Inputs: []octen.VideoSearchInput{{Type: "text", Data: octen.String("how to tie a bowline")}},
		Count:  octen.Int(1),
	})
	if err != nil {
		panic(err)
	}
	r := resp.Data.Results[0]
	fmt.Println(r.Title, r.DurationSeconds)
	// Output: Bowline knot 78
}

func ExampleDesignSearchService_Search() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-d","data":{"query":"pitch deck slide","images":[{"type":"slide","url":"https://example.com/s.png","group":{"group_id":"deck_1","type":"deck","position":19,"total":28}}],"results":[]}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.DesignSearch.Search(context.Background(), octen.DesignSearchParams{
		Query:     octen.String("pitch deck slide"),
		Images:    &octen.DesignImageOptions{Count: octen.Int(1)},
		WebSearch: &octen.DesignWebSearchOptions{Enable: octen.Bool(false)},
	})
	if err != nil {
		panic(err)
	}
	img := resp.Data.Images[0]
	// For the rest of the deck, send DesignSearchParams{Images: &octen.DesignImageOptions{GroupID: octen.String(img.Group.GroupID)}}.
	fmt.Println(img.Type, img.Group.Position, img.Group.Total)
	// Output: slide 19 28
}

func ExampleExtractService_Extract() {
	srv := fakeAPI(200, `{"code":0,"request_id":"req-e","data":{"results":[`+
		`{"url":"https://example.com/","status":"success","resolved_mode":"standard","title":"Example","full_content":"# Example"},`+
		`{"url":"https://broken.invalid/","status":"failed","error_message":"Failed to fetch the URL"}]},`+
		`"meta":{"usage":{"total_urls":2,"successful_urls":1,"successful_by_mode":{"standard_urls":1,"advanced_urls":0}}}}`)
	defer srv.Close()
	client := newExampleClient(srv)

	resp, err := client.Extract.Extract(context.Background(), octen.ExtractParams{
		URLs:         []string{"https://example.com/", "https://broken.invalid/"},
		Format:       octen.String("markdown"),
		IncludeLinks: &octen.ExtractLinksOptions{}, // {}: links with the server defaults
	})
	if err != nil {
		panic(err)
	}
	// Each URL gets its own status; match results to your input by URL.
	for _, r := range resp.Data.Results {
		if r.Status == "success" {
			fmt.Println(r.URL, r.ResolvedMode, *r.Title)
		} else {
			fmt.Println(r.URL, "failed:", r.ErrorMessage)
		}
	}
	fmt.Println("billed standard URLs:", resp.Meta.Usage.SuccessfulByMode.StandardURLs)
	// Output:
	// https://example.com/ standard Example
	// https://broken.invalid/ failed: Failed to fetch the URL
	// billed standard URLs: 1
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
