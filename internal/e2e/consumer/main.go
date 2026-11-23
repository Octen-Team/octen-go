// Command consumer is built by the e2e test as a standalone module that depends on the SDK
// through a replace directive. It uses only the public API and talks to the server in OCTEN_E2E_URL.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	octen "github.com/Octen-Team/octen-go"
)

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "consumer: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	ctx := context.Background()
	client, err := octen.NewClient(
		octen.WithAPIKey("e2e-key"),
		octen.WithBaseURL(os.Getenv("OCTEN_E2E_URL")),
		octen.WithMaxRetries(0),
	)
	if err != nil {
		fail("NewClient: %v", err)
	}

	web, err := client.Search.Search(ctx, octen.SearchParams{Query: "q", Count: octen.Int(1)})
	if err != nil || web.RequestID != "rid-search" || web.Data == nil || len(web.Data.Results) != 1 {
		fail("Search: %+v %v", web, err)
	}
	fmt.Println("OK search", web.RequestID, web.Data.Results[0].URL)

	broad, err := client.Search.BroadSearch(ctx, octen.BroadSearchParams{Query: "q", MaxQueries: octen.Int(1)})
	if err != nil || broad.RequestID != "rid-broad-search" || broad.Data == nil || len(broad.Data.Queries) != 1 {
		fail("BroadSearch: %+v %v", broad, err)
	}
	fmt.Println("OK broad-search", broad.RequestID)

	news, err := client.Search.NewsSearch(ctx, octen.NewsSearchParams{Query: "q"})
	if err != nil || news.RequestID != "rid-news-search" || news.Data == nil || len(news.Data.Results) != 1 {
		fail("NewsSearch: %+v %v", news, err)
	}
	fmt.Println("OK news-search", news.RequestID)

	biz, err := client.Search.BusinessSearch(ctx, octen.BusinessSearchParams{Query: "q"})
	if err != nil || biz.RequestID != "rid-business-search" || biz.Data == nil || len(biz.Data.Results) != 1 {
		fail("BusinessSearch: %+v %v", biz, err)
	}
	fmt.Println("OK business-search", biz.RequestID)

	pulse, err := client.Search.PulseSearch(ctx, octen.PulseSearchParams{Query: "q", Mode: octen.String("lite")})
	if err != nil || pulse.RequestID != "rid-pulse-search" || pulse.Meta == nil || pulse.Meta.Usage == nil || pulse.Meta.Usage.BilledMode != "lite" {
		fail("PulseSearch: %+v %v", pulse, err)
	}
	fmt.Println("OK pulse-search", pulse.RequestID)

	place, err := client.PlaceSearch.Search(ctx, octen.PlaceSearchParams{Query: "q", Latitude: octen.Float64(1.5), Longitude: octen.Float64(2.5)})
	if err != nil || place.RequestID != "rid-place-search" || place.Data == nil || len(place.Data.Places) != 1 {
		fail("PlaceSearch: %+v %v", place, err)
	}
	fmt.Println("OK place-search", place.RequestID)

	premier, err := client.Search.SearchPremier(ctx, octen.SearchParams{Query: "q", ResultType: octen.String("dynamic")})
	if err != nil || premier.RequestID != "rid-search-premier" || premier.Meta == nil || premier.Meta.ResultType != "dynamic" {
		fail("SearchPremier: %+v %v", premier, err)
	}
	fmt.Println("OK search-premier", premier.RequestID)

	img, err := client.ImageSearch.Search(ctx, octen.ImageSearchParams{Inputs: []octen.ImageSearchInput{{Type: "text", Data: octen.String("q")}}})
	if err != nil || img.RequestID != "rid-image-search" || img.Data == nil || len(img.Data.Results) != 1 || img.Data.Results[0].Width != 10 {
		fail("ImageSearch: %+v %v", img, err)
	}
	fmt.Println("OK image-search", img.RequestID)

	video, err := client.VideoSearch.Search(ctx, octen.VideoSearchParams{Inputs: []octen.VideoSearchInput{{Type: "text", Data: octen.String("q")}}})
	if err != nil || video.RequestID != "rid-video-search" || video.Data == nil || len(video.Data.Results) != 1 || video.Data.Results[0].DurationSeconds != 78 {
		fail("VideoSearch: %+v %v", video, err)
	}
	fmt.Println("OK video-search", video.RequestID)

	design, err := client.DesignSearch.Search(ctx, octen.DesignSearchParams{Images: &octen.DesignImageOptions{GroupID: octen.String("g")}})
	if err != nil || design.RequestID != "rid-design-search" || design.Data == nil || len(design.Data.Images) != 1 ||
		design.Data.Images[0].Group == nil || design.Data.Images[0].Group.Position != 1 {
		fail("DesignSearch: %+v %v", design, err)
	}
	fmt.Println("OK design-search", design.RequestID)

	extract, err := client.Extract.Extract(ctx, octen.ExtractParams{
		URLs:         []string{"https://example.com/a", "https://broken.invalid/"},
		IncludeLinks: &octen.ExtractLinksOptions{},
	})
	if err != nil || extract.RequestID != "rid-extract" || extract.Data == nil || len(extract.Data.Results) != 2 ||
		extract.Data.Results[0].PageStructure != nil || len(extract.Data.Results[0].Links) != 1 ||
		extract.Data.Results[1].ErrorMessage != "failed" || extract.Meta == nil || extract.Meta.Usage == nil ||
		extract.Meta.Usage.SuccessfulByMode == nil || extract.Meta.Usage.SuccessfulByMode.StandardURLs != 1 {
		fail("Extract: %+v %v", extract, err)
	}
	fmt.Println("OK extract", extract.RequestID)

	// The server answers query "bad" with a structured 400.
	_, err = client.Search.Search(ctx, octen.SearchParams{Query: "bad"})
	var apiErr *octen.APIError
	if !errors.Is(err, octen.ErrBadRequest) || errors.Is(err, octen.ErrServer) || !errors.As(err, &apiErr) {
		fail("error path: %T %v", err, err)
	}
	if apiErr.StatusCode != 400 || apiErr.RequestID != "rid-bad" || apiErr.Message != "bad query" {
		fail("error fields: %+v", apiErr)
	}
	fmt.Println("OK error-path", apiErr.StatusCode, apiErr.RequestID)

	// A blank query is rejected locally; the server counts requests, so the test can prove none was sent.
	_, err = client.Search.Search(ctx, octen.SearchParams{Query: " "})
	var verr *octen.ValidationError
	if !errors.As(err, &verr) || verr.Field != "query" {
		fail("validation: %T %v", err, err)
	}
	fmt.Println("OK validation", verr.Field)

	_, err = client.Extract.Extract(ctx, octen.ExtractParams{URLs: []string{"https://example.com/a", " "}})
	if !errors.As(err, &verr) || verr.Field != "urls[1]" {
		fail("extract validation: %T %v", err, err)
	}
	fmt.Println("OK validation", verr.Field)
}
