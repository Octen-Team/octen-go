package octen_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	octen "github.com/Octen-Team/octen-go"
)

// Live parameter matrix: billed like any call, so it runs only with OCTEN_LIVE=1 and OCTEN_API_KEY.
// Negative cases assert a structured 4xx sentinel and never a 5xx; a surprise stays red and is recorded as a finding.

func hostOn(rawURL, domain string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == domain || strings.HasSuffix(h, "."+domain)
}

// matrixOK logs the request ID (not a secret) and the elapsed time for the test record.
func matrixOK(t *testing.T, requestID string, start time.Time) {
	t.Helper()
	if requestID == "" {
		t.Fatal("empty request_id")
	}
	t.Logf("OK request_id=%s elapsed_ms=%d", requestID, time.Since(start).Milliseconds())
}

func TestLiveMatrixSearch(t *testing.T) {
	c := liveClient(t)
	search := func(t *testing.T, p octen.SearchParams) *octen.SearchResponse {
		t.Helper()
		start := time.Now()
		res, err := c.Search.Search(liveContext(t), p)
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil {
			t.Fatal("response has no data")
		}
		return res
	}

	t.Run("count_1_at_most_one_result", func(t *testing.T) {
		res := search(t, octen.SearchParams{Query: "octen search api", Count: octen.Int(1)})
		if n := len(res.Data.Results); n != 1 {
			t.Fatalf("results = %d, want 1", n)
		}
	})
	t.Run("include_domains_every_result_on_domain", func(t *testing.T) {
		res := search(t, octen.SearchParams{Query: "solid-state battery", Count: octen.Int(2), IncludeDomains: []string{"wikipedia.org"}})
		if len(res.Data.Results) == 0 {
			t.Fatal("no results to check")
		}
		for _, r := range res.Data.Results {
			if !hostOn(r.URL, "wikipedia.org") {
				t.Errorf("result %q is outside wikipedia.org", r.URL)
			}
		}
	})
	t.Run("exclude_domains_with_unfiltered_control", func(t *testing.T) {
		control := search(t, octen.SearchParams{Query: "wikipedia", Count: octen.Int(5)})
		controlHasWiki := false
		for _, r := range control.Data.Results {
			controlHasWiki = controlHasWiki || hostOn(r.URL, "wikipedia.org")
		}
		t.Logf("control_has_wikipedia=%v", controlHasWiki)
		res := search(t, octen.SearchParams{Query: "wikipedia", Count: octen.Int(5), ExcludeDomains: []string{"wikipedia.org"}})
		if len(res.Data.Results) == 0 {
			t.Fatal("no results to check")
		}
		for _, r := range res.Data.Results {
			if hostOn(r.URL, "wikipedia.org") {
				t.Errorf("result %q is on the excluded domain", r.URL)
			}
		}
	})
	for _, tr := range []string{"week", "m"} {
		t.Run("time_range_"+tr, func(t *testing.T) {
			search(t, octen.SearchParams{Query: "technology news", Count: octen.Int(1), TimeRange: octen.String(tr)})
		})
	}
	t.Run("language_en", func(t *testing.T) {
		search(t, octen.SearchParams{Query: "octen search api", Count: octen.Int(1), Language: []string{"en"}})
	})
	t.Run("highlight_enabled_result_carries_highlight", func(t *testing.T) {
		res := search(t, octen.SearchParams{Query: "octen search api", Count: octen.Int(1), Highlight: &octen.HighlightOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(100)}})
		if len(res.Data.Results) == 0 || res.Data.Results[0].Highlight == "" {
			t.Fatal("first result has no highlight")
		}
	})
	t.Run("full_content_enabled_result_carries_full_content", func(t *testing.T) {
		res := search(t, octen.SearchParams{Query: "octen search api", Count: octen.Int(1), FullContent: &octen.FullContentOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(100)}})
		if len(res.Data.Results) == 0 || res.Data.Results[0].FullContent == "" {
			t.Fatal("first result has no full_content")
		}
	})
	for _, f := range []string{"markdown", "text"} {
		t.Run("format_"+f, func(t *testing.T) {
			search(t, octen.SearchParams{Query: "octen search api", Count: octen.Int(1), Format: octen.String(f),
				FullContent: &octen.FullContentOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(100)}})
		})
	}
	for _, s := range []string{"off", "strict"} {
		t.Run("safesearch_"+s, func(t *testing.T) {
			search(t, octen.SearchParams{Query: "octen search api", Count: octen.Int(1), Safesearch: octen.String(s)})
		})
	}
	t.Run("include_images", func(t *testing.T) {
		res := search(t, octen.SearchParams{Query: "golden gate bridge", Count: octen.Int(1), IncludeImages: octen.Bool(true)})
		if len(res.Data.Results) > 0 {
			r := res.Data.Results[0]
			t.Logf("images=%d cover_image=%v", len(r.Images), r.CoverImage != nil)
		}
	})
}

func TestLiveMatrixBroadSearch(t *testing.T) {
	c := liveClient(t)
	t.Run("max_queries_1_with_search_options", func(t *testing.T) {
		start := time.Now()
		res, err := c.Search.BroadSearch(liveContext(t), octen.BroadSearchParams{
			Query:      "history of web search engines",
			MaxQueries: octen.Int(1),
			SearchOptions: &octen.WebSearchOptions{
				Count: octen.Int(1), Language: []string{"en"}, TimeRange: octen.String("year"),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil {
			t.Fatal("response has no data")
		}
		if n := len(res.Data.Queries); n > 1 {
			t.Errorf("queries = %d, want at most 1", n)
		}
		if n := len(res.Data.SearchResults); n > 1 {
			t.Errorf("search_results groups = %d, want at most 1", n)
		}
		for _, g := range res.Data.SearchResults {
			if len(g.Results) > 1 {
				t.Errorf("group %q has %d results, want at most 1", g.Query, len(g.Results))
			}
		}
	})
	t.Run("max_queries_2", func(t *testing.T) {
		start := time.Now()
		res, err := c.Search.BroadSearch(liveContext(t), octen.BroadSearchParams{
			Query:         "compare electric vehicle battery chemistries",
			MaxQueries:    octen.Int(2),
			SearchOptions: &octen.WebSearchOptions{Count: octen.Int(1)},
		})
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil {
			t.Fatal("response has no data")
		}
		if n := len(res.Data.Queries); n > 2 {
			t.Errorf("queries = %d, want at most 2", n)
		}
	})
}

func TestLiveMatrixNewsSearch(t *testing.T) {
	c := liveClient(t)
	t.Run("subjects_count_1_max_sub_news_1", func(t *testing.T) {
		start := time.Now()
		res, err := c.Search.NewsSearch(liveContext(t), octen.NewsSearchParams{
			Query: "artificial intelligence", Count: octen.Int(2),
			Subjects: &octen.NewsSubjectOptions{Enable: octen.Bool(true), Count: octen.Int(1), MaxSubNews: octen.Int(1)},
		})
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil {
			t.Fatal("response has no data")
		}
		t.Logf("subjects=%d", len(res.Data.Subjects))
		if len(res.Data.Subjects) > 1 {
			t.Errorf("subjects = %d, want at most 1", len(res.Data.Subjects))
		}
		for _, s := range res.Data.Subjects {
			if len(s.SubNews) > 1 {
				t.Errorf("subject %q has %d sub_news, want at most 1", s.Name, len(s.SubNews))
			}
		}
	})
}

func TestLiveMatrixBusinessSearch(t *testing.T) {
	c := liveClient(t)
	t.Run("entities_count_1_max_activities_1_max_news_1", func(t *testing.T) {
		start := time.Now()
		res, err := c.Search.BusinessSearch(liveContext(t), octen.BusinessSearchParams{
			Query: "Apple Inc", Count: octen.Int(1),
			Entities: &octen.BusinessEntityOptions{Enable: octen.Bool(true), Count: octen.Int(1), MaxActivities: octen.Int(1), MaxNews: octen.Int(1)},
		})
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil {
			t.Fatal("response has no data")
		}
		t.Logf("entities=%d", len(res.Data.Entities))
		if len(res.Data.Entities) > 1 {
			t.Errorf("entities = %d, want at most 1", len(res.Data.Entities))
		}
		for _, e := range res.Data.Entities {
			var activities, newsItems int
			switch {
			case e.Company != nil:
				activities, newsItems = len(e.Company.Activities), len(e.Company.News)
			case e.Person != nil:
				activities, newsItems = len(e.Person.Activities), len(e.Person.News)
			}
			t.Logf("entity type=%s activities=%d news=%d", e.Type, activities, newsItems)
			if activities > 1 || newsItems > 1 {
				t.Errorf("entity has %d activities and %d news, want at most 1 each", activities, newsItems)
			}
		}
	})
}

func TestLiveMatrixPulseSearch(t *testing.T) {
	c := liveClient(t)
	for _, mode := range []string{"lite", "standard"} {
		t.Run("mode_"+mode, func(t *testing.T) {
			start := time.Now()
			res, err := c.Search.PulseSearch(liveContext(t), octen.PulseSearchParams{Query: "octen search api", Mode: octen.String(mode), Count: octen.Int(1)})
			if err != nil {
				t.Fatal(err)
			}
			matrixOK(t, res.RequestID, start)
			if res.Data == nil || res.Meta == nil || res.Meta.Usage == nil {
				t.Fatal("response lacks data or meta.usage")
			}
			t.Logf("billed_mode=%s", res.Meta.Usage.BilledMode)
			// standard may be billed as lite when page details are unavailable, so only lite is asserted.
			if mode == "lite" && res.Meta.Usage.BilledMode != "lite" {
				t.Errorf("billed_mode = %q, want lite", res.Meta.Usage.BilledMode)
			}
			if n := len(res.Data.Results); n > 1 {
				t.Errorf("results = %d, want at most 1", n)
			}
		})
	}
}

func TestLiveMatrixPlaceSearch(t *testing.T) {
	c := liveClient(t)
	t.Run("coordinates_radius_count_max_reviews_within_radius", func(t *testing.T) {
		start := time.Now()
		res, err := c.PlaceSearch.Search(liveContext(t), octen.PlaceSearchParams{
			Query: "coffee shop", Latitude: octen.Float64(47.6062), Longitude: octen.Float64(-122.3321), Radius: octen.Int(2000),
			Places:    &octen.PlaceCardOptions{Count: octen.Int(1), MaxReviews: octen.Int(1)},
			WebSearch: &octen.PlaceWebSearchOptions{Enable: octen.Bool(false)},
		})
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil || len(res.Data.Places) != 1 {
			t.Fatalf("want exactly 1 place, got %+v", res.Data)
		}
		p := res.Data.Places[0]
		t.Logf("distance=%d attributes=%d", p.Distance, len(p.Attributes))
		if p.Distance > 2000 {
			t.Errorf("distance = %d, want at most 2000", p.Distance)
		}
	})
	t.Run("user_location", func(t *testing.T) {
		start := time.Now()
		res, err := c.PlaceSearch.Search(liveContext(t), octen.PlaceSearchParams{
			Query: "coffee shop near me",
			UserLocation: &octen.PlaceUserLocation{
				Type: octen.String("approximate"), City: octen.String("Seattle"), Region: octen.String("Washington"),
				Country: octen.String("US"), Timezone: octen.String("America/Los_Angeles"),
			},
			Places:    &octen.PlaceCardOptions{Count: octen.Int(1)},
			WebSearch: &octen.PlaceWebSearchOptions{Enable: octen.Bool(false)},
		})
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil {
			t.Fatal("response has no data")
		}
		t.Logf("places=%d location=%+v", len(res.Data.Places), res.Data.Location)
	})
	t.Run("web_search_enabled_count_1", func(t *testing.T) {
		start := time.Now()
		res, err := c.PlaceSearch.Search(liveContext(t), octen.PlaceSearchParams{
			Query:  "coffee shop in Seattle",
			Places: &octen.PlaceCardOptions{Count: octen.Int(1)},
			WebSearch: &octen.PlaceWebSearchOptions{
				Enable: octen.Bool(true), Count: octen.Int(1),
				Highlight: &octen.HighlightOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(100)},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil || len(res.Data.Results) != 1 {
			t.Fatalf("want exactly 1 web result, got %+v", res.Data)
		}
	})
}

func TestLiveMatrixDesignSearch(t *testing.T) {
	c := liveClient(t)
	design := func(t *testing.T, p octen.DesignSearchParams) *octen.DesignSearchData {
		t.Helper()
		start := time.Now()
		res, err := c.Search.DesignSearch(liveContext(t), p)
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil {
			t.Fatal("response has no data")
		}
		return res.Data
	}

	// Page sections come back grouped by page; this query returned groups on 2026-11-21.
	t.Run("group_lookup_positions_1_2_web_disabled", func(t *testing.T) {
		d := design(t, octen.DesignSearchParams{
			Query:     octen.String("SaaS landing page hero section"),
			Images:    &octen.DesignImageOptions{Count: octen.Int(3)},
			WebSearch: &octen.DesignWebSearchOptions{Enable: octen.Bool(false)},
		})
		if len(d.Results) != 0 {
			t.Errorf("web_search.enable false still returned %d web results", len(d.Results))
		}
		var group *octen.DesignImageGroup
		for _, img := range d.Images {
			// Three or more, so a lookup that ignored count 2 would return a third position.
			if img.Group != nil && img.Group.Total >= 3 {
				group = img.Group
				break
			}
		}
		if group == nil {
			t.Fatalf("none of %d images belongs to a group of three or more", len(d.Images))
		}
		g := design(t, octen.DesignSearchParams{Images: &octen.DesignImageOptions{GroupID: octen.String(group.GroupID), Count: octen.Int(2)}})
		var positions []int
		for _, img := range g.Images {
			if img.Group == nil || img.Group.GroupID != group.GroupID {
				t.Fatalf("image outside the group: %+v", img.Group)
			}
			positions = append(positions, img.Group.Position)
		}
		if len(positions) != 2 || positions[0] != 1 || positions[1] != 2 {
			t.Fatalf("positions = %v, want [1 2]", positions)
		}
	})
}

// Invalid values go to the server on purpose: the SDK checks only blank queries and non-finite numbers locally.
func TestLiveMatrixNegative(t *testing.T) {
	c := liveClient(t)
	q := "octen"
	type negCase struct {
		name string
		run  func(ctx context.Context) error
	}
	search := func(name string, p octen.SearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.Search.Search(ctx, p); return err }}
	}
	broad := func(name string, p octen.BroadSearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.Search.BroadSearch(ctx, p); return err }}
	}
	news := func(name string, p octen.NewsSearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.Search.NewsSearch(ctx, p); return err }}
	}
	biz := func(name string, p octen.BusinessSearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.Search.BusinessSearch(ctx, p); return err }}
	}
	pulse := func(name string, p octen.PulseSearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.Search.PulseSearch(ctx, p); return err }}
	}
	place := func(name string, p octen.PlaceSearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.PlaceSearch.Search(ctx, p); return err }}
	}
	premier := func(name string, p octen.SearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.Search.SearchPremier(ctx, p); return err }}
	}
	image := func(name string, p octen.ImageSearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.Search.ImageSearch(ctx, p); return err }}
	}
	video := func(name string, p octen.VideoSearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.Search.VideoSearch(ctx, p); return err }}
	}
	design := func(name string, p octen.DesignSearchParams) negCase {
		return negCase{name, func(ctx context.Context) error { _, err := c.Search.DesignSearch(ctx, p); return err }}
	}
	textImage := []octen.ImageSearchInput{{Type: "text", Data: octen.String(q)}}
	textVideo := []octen.VideoSearchInput{{Type: "text", Data: octen.String(q)}}
	elevenDomains := make([]string, 11)
	for i := range elevenDomains {
		elevenDomains[i] = "d" + strconv.Itoa(i) + ".example.com"
	}
	cases := []negCase{
		search("search count=0", octen.SearchParams{Query: q, Count: octen.Int(0)}),
		search("search count=101", octen.SearchParams{Query: q, Count: octen.Int(101)}),
		search("search count=-1", octen.SearchParams{Query: q, Count: octen.Int(-1)}),
		search("search time_range=decade", octen.SearchParams{Query: q, TimeRange: octen.String("decade")}),
		search("search safesearch=maybe", octen.SearchParams{Query: q, Safesearch: octen.String("maybe")}),
		search("search format=html", octen.SearchParams{Query: q, Format: octen.String("html")}),
		search("search result_type=fancy", octen.SearchParams{Query: q, ResultType: octen.String("fancy")}),
		search("search language=xx", octen.SearchParams{Query: q, Language: []string{"xx"}}),
		search("search highlight.max_tokens=50 (min 100)", octen.SearchParams{Query: q, Highlight: &octen.HighlightOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(50)}}),
		search("search query of 501 chars (max 500)", octen.SearchParams{Query: strings.Repeat("a", 501)}),
		broad("broadSearch max_queries=0", octen.BroadSearchParams{Query: q, MaxQueries: octen.Int(0)}),
		broad("broadSearch max_queries=31", octen.BroadSearchParams{Query: q, MaxQueries: octen.Int(31)}),
		broad("broadSearch search_options.count=0", octen.BroadSearchParams{Query: q, MaxQueries: octen.Int(1), SearchOptions: &octen.WebSearchOptions{Count: octen.Int(0)}}),
		news("newsSearch count=0", octen.NewsSearchParams{Query: q, Count: octen.Int(0)}),
		news("newsSearch count=101", octen.NewsSearchParams{Query: q, Count: octen.Int(101)}),
		news("newsSearch time_range=decade", octen.NewsSearchParams{Query: q, TimeRange: octen.String("decade")}),
		news("newsSearch subjects.count=6", octen.NewsSearchParams{Query: q, Subjects: &octen.NewsSubjectOptions{Enable: octen.Bool(true), Count: octen.Int(6)}}),
		biz("businessSearch count=0", octen.BusinessSearchParams{Query: q, Count: octen.Int(0)}),
		biz("businessSearch entities.count=21", octen.BusinessSearchParams{Query: q, Entities: &octen.BusinessEntityOptions{Enable: octen.Bool(true), Count: octen.Int(21)}}),
		pulse("pulseSearch mode=turbo", octen.PulseSearchParams{Query: q, Mode: octen.String("turbo")}),
		pulse("pulseSearch count=11", octen.PulseSearchParams{Query: q, Count: octen.Int(11)}),
		pulse("pulseSearch count=0", octen.PulseSearchParams{Query: q, Count: octen.Int(0)}),
		place("placeSearch radius without coordinates", octen.PlaceSearchParams{Query: q, Radius: octen.Int(1000)}),
		place("placeSearch latitude without longitude", octen.PlaceSearchParams{Query: q, Latitude: octen.Float64(47.6)}),
		place("placeSearch latitude=91", octen.PlaceSearchParams{Query: q, Latitude: octen.Float64(91), Longitude: octen.Float64(0)}),
		place("placeSearch radius=50001", octen.PlaceSearchParams{Query: q, Latitude: octen.Float64(47.6), Longitude: octen.Float64(-122.3), Radius: octen.Int(50001)}),
		place("placeSearch user_location.type=exact", octen.PlaceSearchParams{Query: q, UserLocation: &octen.PlaceUserLocation{Type: octen.String("exact")}}),
		place("placeSearch places.count=21", octen.PlaceSearchParams{Query: q, Places: &octen.PlaceCardOptions{Count: octen.Int(21)}}),
		premier("searchPremier count=0", octen.SearchParams{Query: q, Count: octen.Int(0)}),
		premier("searchPremier query of 501 chars (max 500)", octen.SearchParams{Query: strings.Repeat("a", 501)}),
		image("imageSearch count=11", octen.ImageSearchParams{Inputs: textImage, Count: octen.Int(11)}),
		image("imageSearch 11 include_domains (max 10)", octen.ImageSearchParams{Inputs: textImage, IncludeDomains: elevenDomains}),
		video("videoSearch count=11", octen.VideoSearchParams{Inputs: textVideo, Count: octen.Int(11)}),
		design("designSearch query of 501 chars (max 500)", octen.DesignSearchParams{Query: octen.String(strings.Repeat("a", 501))}),
		design("designSearch images.count=101", octen.DesignSearchParams{Query: octen.String(q), Images: &octen.DesignImageOptions{Count: octen.Int(101)}}),
		design("designSearch snippet.max_tokens=50 (min 100)", octen.DesignSearchParams{Query: octen.String(q),
			Images: &octen.DesignImageOptions{Snippet: &octen.DesignSnippetOptions{MaxTokens: octen.Int(50)}}}),
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Video Search is an invite-only beta, so its cases follow the same opt-in as the success cases.
			if strings.HasPrefix(tc.name, "videoSearch") && os.Getenv("OCTEN_LIVE_VIDEO") != "1" {
				t.Skip("Video Search cases need OCTEN_LIVE_VIDEO=1 and a key with beta access")
			}
			start := time.Now()
			err := tc.run(liveContext(t))
			var apiErr *octen.APIError
			if errors.As(err, &apiErr) {
				msg := apiErr.Message
				if len(msg) > 160 {
					msg = msg[:160]
				}
				code := "none"
				if apiErr.Code != nil {
					code = strconv.FormatFloat(*apiErr.Code, 'g', -1, 64)
				}
				t.Logf("status=%d code=%s request_id=%s msg=%q elapsed_ms=%d", apiErr.StatusCode, code, apiErr.RequestID, msg, time.Since(start).Milliseconds())
			} else {
				t.Logf("err=%v elapsed_ms=%d", err, time.Since(start).Milliseconds())
			}
			if err == nil {
				t.Fatal("expected an error, the call succeeded")
			}
			if errors.Is(err, octen.ErrServer) {
				t.Fatalf("got a server error, want a structured 4xx: %v", err)
			}
			if !errors.Is(err, octen.ErrBadRequest) {
				t.Fatalf("want ErrBadRequest, got %T: %v", err, err)
			}
			if apiErr.StatusCode != http.StatusBadRequest && apiErr.StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 400 or 422", apiErr.StatusCode)
			}
		})
	}
}

// Rejected before any request: the counting transport proves nothing reached the network.
func TestLiveMatrixLocalValidation(t *testing.T) {
	ct := &countingTransport{next: http.DefaultTransport}
	c := liveClient(t, octen.WithHTTPClient(&http.Client{Transport: ct}))
	ctx := liveContext(t)
	empties := []struct {
		name string
		run  func() error
	}{
		{"search", func() error { _, err := c.Search.Search(ctx, octen.SearchParams{}); return err }},
		{"broadSearch", func() error { _, err := c.Search.BroadSearch(ctx, octen.BroadSearchParams{}); return err }},
		{"newsSearch", func() error { _, err := c.Search.NewsSearch(ctx, octen.NewsSearchParams{}); return err }},
		{"businessSearch", func() error { _, err := c.Search.BusinessSearch(ctx, octen.BusinessSearchParams{}); return err }},
		{"pulseSearch", func() error { _, err := c.Search.PulseSearch(ctx, octen.PulseSearchParams{}); return err }},
		{"placeSearch", func() error { _, err := c.PlaceSearch.Search(ctx, octen.PlaceSearchParams{}); return err }},
	}
	for _, tc := range empties {
		t.Run("empty_query_"+tc.name, func(t *testing.T) {
			err := tc.run()
			var verr *octen.ValidationError
			if !errors.As(err, &verr) || verr.Field != "query" {
				t.Fatalf("want *ValidationError on query, got %T: %v", err, err)
			}
			t.Logf("field=%s round_trips=%d", verr.Field, ct.calls.Load())
		})
	}
	inputs := []struct {
		name, field string
		run         func() error
	}{
		{"imageSearch_no_input", "inputs", func() error { _, err := c.Search.ImageSearch(ctx, octen.ImageSearchParams{}); return err }},
		{"videoSearch_no_input", "inputs", func() error { _, err := c.Search.VideoSearch(ctx, octen.VideoSearchParams{}); return err }},
		{"designSearch_no_query_or_group", "query", func() error {
			_, err := c.Search.DesignSearch(ctx, octen.DesignSearchParams{Images: &octen.DesignImageOptions{GroupID: octen.String(" ")}})
			return err
		}},
	}
	for _, tc := range inputs {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			var verr *octen.ValidationError
			if !errors.As(err, &verr) || verr.Field != tc.field {
				t.Fatalf("want *ValidationError on %s, got %T: %v", tc.field, err, err)
			}
			t.Logf("field=%s round_trips=%d", verr.Field, ct.calls.Load())
		})
	}
	t.Run("nan_latitude", func(t *testing.T) {
		_, err := c.PlaceSearch.Search(ctx, octen.PlaceSearchParams{Query: "coffee", Latitude: octen.Float64(math.NaN()), Longitude: octen.Float64(0)})
		var verr *octen.ValidationError
		if !errors.As(err, &verr) || verr.Field != "latitude" {
			t.Fatalf("want *ValidationError on latitude, got %T: %v", err, err)
		}
		t.Logf("field=%s round_trips=%d", verr.Field, ct.calls.Load())
	})
	if n := ct.calls.Load(); n != 0 {
		t.Fatalf("round trips = %d, want 0", n)
	}
}

// Parameter matrix for the endpoints added after 0.1.0. Each check asserts the effect; a filter is
// paired with an unfiltered control on the same query that shows what the filter changes.

// hostOf returns the lower-cased host of rawURL without a leading "www.", or "" when it has none.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// hasKana reports Japanese-only script; Han characters alone could be Chinese.
func hasKana(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

type datedResult struct{ url, stamp string }

// Why a result must carry its timestamp under a time filter, for the failure message.
const (
	timeBasisRule = "the reference says results missing the time_basis field are excluded when filtering by time"
	videoTimeRule = "the Video Search reference only says results are filtered by publish time; an undated video is a gap to report"
)

// checkDates requires every result to carry the timestamp and to fall in [from, to].
func checkDates(t *testing.T, field string, rs []datedResult, from, to time.Time, rule string) {
	t.Helper()
	if len(rs) == 0 {
		t.Fatal("no results to check")
	}
	for _, r := range rs {
		ts, err := time.Parse(time.RFC3339, r.stamp)
		if err != nil {
			t.Errorf("result %q has %s %q under a time filter (%s): %v", r.url, field, r.stamp, rule, err)
			continue
		}
		if ts.Before(from) || ts.After(to) {
			t.Errorf("result %q %s %s is outside [%s, %s]", r.url, field, r.stamp, from.Format(time.RFC3339), to.Format(time.RFC3339))
		}
	}
}

// requireControlOutside needs a control result outside the window; without one, the filtered
// check could not fail for a filter the server ignores.
func requireControlOutside(t *testing.T, field string, rs []datedResult, from, to time.Time) {
	t.Helper()
	outside := 0
	for _, r := range rs {
		if ts, err := time.Parse(time.RFC3339, r.stamp); err == nil && (ts.Before(from) || ts.After(to)) {
			outside++
		}
	}
	if outside == 0 {
		t.Fatalf("no control result has %s outside the window, so the filter cannot be shown", field)
	}
	t.Logf("control results with %s outside the window: %d of %d", field, outside, len(rs))
}

func resultText(r octen.SearchResult) string {
	return strings.ToLower(r.Title + "\n" + r.Highlight + "\n" + r.FullContent)
}

func distinctHosts(urls []string) int {
	seen := map[string]bool{}
	for _, u := range urls {
		if h := hostOf(u); h != "" {
			seen[h] = true
		}
	}
	return len(seen)
}

func TestLiveMatrixSearchPremierParams(t *testing.T) {
	c := liveClient(t)
	premier := func(t *testing.T, p octen.SearchParams) *octen.SearchResponse {
		t.Helper()
		start := time.Now()
		res, err := c.Search.SearchPremier(liveContext(t), p)
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil || len(res.Data.Results) == 0 {
			t.Fatal("no results to check")
		}
		return res
	}

	// One unfiltered request is the control for include_text, exclude_text, exclude_domains and include_domains.
	// Safesearch on a benign query has no observable effect; it is sent on every call so it is not a difference.
	const word = "colorado"
	// include_text matches the whole page, so full_content must cover long articles (2000 tokens missed one);
	// three results keep the four requests that carry it affordable.
	base := octen.SearchParams{Query: "best hiking trails", Count: octen.Int(3), Safesearch: octen.String("off"),
		FullContent: &octen.FullContentOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(12000)}}
	control := premier(t, base).Data.Results
	var controlURLs []string
	for _, r := range control {
		controlURLs = append(controlURLs, r.URL)
	}

	t.Run("include_text", func(t *testing.T) {
		lacking := 0
		for _, r := range control {
			if !strings.Contains(resultText(r), word) {
				lacking++
			}
		}
		t.Logf("control results lacking %q: %d of %d", word, lacking, len(control))
		if lacking == 0 {
			t.Fatalf("every control result already contains %q; include_text cannot be shown", word)
		}
		p := base
		p.IncludeText = []string{"Colorado"}
		for _, r := range premier(t, p).Data.Results {
			if !strings.Contains(resultText(r), word) {
				t.Errorf("result %q does not contain the include_text phrase", r.URL)
			}
		}
	})
	t.Run("exclude_text_and_exclude_domains_independent", func(t *testing.T) {
		// The excluded host's control result lacks the word and the word appears on another host,
		// so neither filter can pass on the other's account.
		excluded := ""
		for _, cand := range control {
			if strings.Contains(resultText(cand), word) {
				continue
			}
			for _, other := range control {
				if hostOf(other.URL) != hostOf(cand.URL) && strings.Contains(resultText(other), word) {
					excluded = hostOf(cand.URL)
				}
			}
			if excluded != "" {
				break
			}
		}
		t.Logf("excluded_host=%s", excluded)
		if excluded == "" {
			t.Fatalf("control lacks a host without %q next to one with it; the exclusions cannot be shown", word)
		}
		p := base
		p.ExcludeText = []string{"Colorado"}
		p.ExcludeDomains = []string{excluded}
		for _, r := range premier(t, p).Data.Results {
			if hostOn(r.URL, excluded) {
				t.Errorf("result %q is on the excluded domain", r.URL)
			}
			if strings.Contains(resultText(r), word) {
				t.Errorf("result %q still mentions the excluded text", r.URL)
			}
		}
	})
	t.Run("include_domains_full_content_result_type_standard", func(t *testing.T) {
		if n := distinctHosts(controlURLs); n < 2 {
			t.Fatalf("control shows %d host(s); include_domains cannot be shown", n)
		}
		target := hostOf(control[0].URL)
		t.Logf("include_host=%s", target)
		p := base
		p.IncludeDomains = []string{target}
		p.ResultType = octen.String("standard")
		p.FullContent = &octen.FullContentOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(200)}
		res := premier(t, p)
		if res.Meta == nil || res.Meta.ResultType != "standard" {
			t.Errorf("meta = %+v, want result_type standard echoed", res.Meta)
		}
		for _, r := range res.Data.Results {
			if !hostOn(r.URL, target) {
				t.Errorf("result %q is outside %s", r.URL, target)
			}
			if r.FullContent == "" {
				t.Errorf("result %q has no full_content", r.URL)
			}
		}
	})
	// The reference: format "controls the formatting of highlight outputs". The goldengate.org home
	// page opens with images and links (seen 2026-11-21), so a markdown highlight may keep their syntax.
	t.Run("format_highlight_markdown_vs_text_include_images", func(t *testing.T) {
		p := octen.SearchParams{Query: "golden gate bridge", Count: octen.Int(1), IncludeDomains: []string{"goldengate.org"},
			Highlight: &octen.HighlightOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(300)}}
		md := p
		md.Format, md.IncludeImages = octen.String("markdown"), octen.Bool(true)
		txt := p
		txt.Format = octen.String("text")
		mr, xr := premier(t, md), premier(t, txt)
		m, x := mr.Data.Results[0], xr.Data.Results[0]
		if m.URL != x.URL {
			t.Fatalf("the two calls returned different pages (%s, %s); the formats cannot be compared", m.URL, x.URL)
		}
		syntax := func(s string) bool {
			return strings.Contains(s, "](") || strings.Contains(s, "**") || strings.Contains(s, "\n#")
		}
		t.Logf("highlight chars markdown=%d text=%d", len(m.Highlight), len(x.Highlight))
		if m.Highlight == x.Highlight {
			t.Logf("FORMAT NOT OBSERVABLE: identical highlights for %s (request_ids %s, %s)", m.URL, mr.RequestID, xr.RequestID)
		} else if !syntax(m.Highlight) || syntax(x.Highlight) {
			t.Errorf("highlights differ but not by markdown syntax: markdown %q / text %q", m.Highlight, x.Highlight)
		}
		if m.CoverImage == nil && len(m.Images) == 0 {
			t.Error("include_images true: no cover_image or images")
		}
		if x.CoverImage != nil || len(x.Images) != 0 {
			t.Error("include_images unset: result still carries images")
		}
	})
	t.Run("language_ja", func(t *testing.T) {
		for _, r := range premier(t, octen.SearchParams{Query: "Tokyo travel guide", Count: octen.Int(3), Language: []string{"ja"}}).Data.Results {
			if !hasKana(r.Title + r.Highlight) {
				t.Errorf("result %q (%q) has no kana", r.URL, r.Title)
			}
		}
	})
	t.Run("time_basis_published_start_end_result_type_dynamic", func(t *testing.T) {
		from, to := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)
		res := premier(t, octen.SearchParams{Query: "artificial intelligence regulation", Count: octen.Int(3), ResultType: octen.String("dynamic"),
			TimeBasis: octen.String("published"), StartTime: octen.String(from.Format(time.RFC3339)), EndTime: octen.String(to.Format(time.RFC3339))})
		if res.Meta == nil || res.Meta.ResultType != "dynamic" {
			t.Errorf("meta = %+v, want result_type dynamic echoed", res.Meta)
		}
		var rs []datedResult
		for _, r := range res.Data.Results {
			rs = append(rs, datedResult{r.URL, r.TimePublished})
		}
		checkDates(t, "time_published", rs, from, to, timeBasisRule)
	})
	t.Run("time_basis_crawled_time_range_week_with_control", func(t *testing.T) {
		from, to := time.Now().AddDate(0, 0, -8), time.Now().Add(time.Hour)
		p := octen.SearchParams{Query: "history of the printing press", Count: octen.Int(5)}
		var ctl []datedResult
		for _, r := range premier(t, p).Data.Results {
			ctl = append(ctl, datedResult{r.URL, r.TimeLastCrawled})
		}
		requireControlOutside(t, "time_last_crawled", ctl, from, to)
		p.TimeBasis, p.TimeRange = octen.String("crawled"), octen.String("week")
		var rs []datedResult
		for _, r := range premier(t, p).Data.Results {
			rs = append(rs, datedResult{r.URL, r.TimeLastCrawled})
		}
		checkDates(t, "time_last_crawled", rs, from, to, timeBasisRule)
	})
}

// max_tokens 100 (the minimum) against 512 (the server default): dropping the cap makes the two calls equal.
func TestLiveMatrixSearchPremierHighlightCap(t *testing.T) {
	c := liveClient(t)
	highlights := func(t *testing.T, maxTokens int) map[string]string {
		t.Helper()
		start := time.Now()
		res, err := c.Search.SearchPremier(liveContext(t), octen.SearchParams{Query: "solid-state battery", Count: octen.Int(3),
			Highlight: &octen.HighlightOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(maxTokens)}})
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		out := map[string]string{}
		if res.Data != nil {
			for _, r := range res.Data.Results {
				out[r.URL] = r.Highlight
			}
		}
		return out
	}
	large, small := highlights(t, 512), highlights(t, 100)
	compared, shorter := 0, 0
	for u, sh := range small {
		l, ok := large[u]
		if !ok {
			continue
		}
		compared++
		t.Logf("%s highlight chars cap100=%d cap512=%d", u, len(sh), len(l))
		if sh == "" || len(sh) > len(l) {
			t.Errorf("result %q: capped highlight (%d chars) is empty or longer than the default one (%d)", u, len(sh), len(l))
		}
		if len(sh) < len(l) {
			shorter++
		}
	}
	if compared == 0 {
		t.Fatal("the two calls share no result URL")
	}
	if shorter == 0 {
		t.Errorf("max_tokens 100 shortened none of %d highlights compared with 512", compared)
	}
}

func TestLiveMatrixImageSearchParams(t *testing.T) {
	c := liveClient(t)
	image := func(t *testing.T, p octen.ImageSearchParams) []octen.ImageSearchResult {
		t.Helper()
		start := time.Now()
		res, err := c.Search.ImageSearch(liveContext(t), p)
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil || len(res.Data.Results) == 0 {
			t.Fatal("no results to check")
		}
		return res.Data.Results
	}
	// Safesearch on a benign query has no observable effect; every call sends the same value.
	in := []octen.ImageSearchInput{{Type: "text", Data: octen.String("Golden Gate Bridge")}}
	control := image(t, octen.ImageSearchParams{Inputs: in, Count: octen.Int(5), Safesearch: octen.String("off")})
	var pages []string
	for _, r := range control {
		if hostOf(r.SourcePage) == "" {
			t.Fatalf("control result %q has no parseable source_page %q", r.URL, r.SourcePage)
		}
		if r.URL == "" || r.Width == 0 || r.Height == 0 {
			t.Errorf("incomplete control result %+v", r)
		}
		pages = append(pages, r.SourcePage)
	}
	if n := distinctHosts(pages); n < 2 {
		t.Fatalf("control shows %d source host(s); the domain filters cannot be shown", n)
	}
	target := hostOf(control[0].SourcePage)
	t.Logf("control results=%d target_host=%s", len(control), target)

	t.Run("include_domains", func(t *testing.T) {
		for _, r := range image(t, octen.ImageSearchParams{Inputs: in, Count: octen.Int(5), Safesearch: octen.String("off"), IncludeDomains: []string{target}}) {
			if !hostOn(r.SourcePage, target) {
				t.Errorf("source_page %q is outside %s", r.SourcePage, target)
			}
		}
	})
	t.Run("exclude_domains", func(t *testing.T) {
		for _, r := range image(t, octen.ImageSearchParams{Inputs: in, Count: octen.Int(5), Safesearch: octen.String("off"), ExcludeDomains: []string{target}}) {
			if hostOn(r.SourcePage, target) {
				t.Errorf("source_page %q is on the excluded domain", r.SourcePage)
			}
		}
	})
}

func TestLiveMatrixVideoSearchParams(t *testing.T) {
	c := liveVideoClient(t)
	video := func(t *testing.T, p octen.VideoSearchParams) []octen.VideoSearchResult {
		t.Helper()
		start := time.Now()
		res, err := c.Search.VideoSearch(liveContext(t), p)
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil || len(res.Data.Results) == 0 {
			t.Fatal("no results to check")
		}
		return res.Data.Results
	}
	dated := func(rs []octen.VideoSearchResult) []datedResult {
		var out []datedResult
		for _, r := range rs {
			out = append(out, datedResult{r.URL, r.TimePublished})
		}
		return out
	}
	in := []octen.VideoSearchInput{{Type: "text", Data: octen.String("SpaceX rocket launch")}}
	// Safesearch on a benign query has no observable effect; both calls send the same value.
	t.Run("time_range_month_with_control_safesearch_strict", func(t *testing.T) {
		from, to := time.Now().AddDate(0, 0, -32), time.Now().Add(time.Hour)
		p := octen.VideoSearchParams{Inputs: in, Count: octen.Int(3), Safesearch: octen.String("strict")}
		ctl := video(t, p)
		for _, r := range ctl {
			if r.URL == "" || r.DurationSeconds <= 0 {
				t.Errorf("incomplete control result %+v", r)
			}
		}
		requireControlOutside(t, "time_published", dated(ctl), from, to)
		p.TimeRange = octen.String("month")
		checkDates(t, "time_published", dated(video(t, p)), from, to, videoTimeRule)
	})
	t.Run("start_end", func(t *testing.T) {
		from, to := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 12, 31, 23, 59, 59, 0, time.UTC)
		rs := video(t, octen.VideoSearchParams{Inputs: in, Count: octen.Int(3),
			StartTime: octen.String(from.Format(time.RFC3339)), EndTime: octen.String(to.Format(time.RFC3339))})
		checkDates(t, "time_published", dated(rs), from, to, videoTimeRule)
	})
}

func TestLiveMatrixDesignSearchParams(t *testing.T) {
	c := liveClient(t)
	design := func(t *testing.T, p octen.DesignSearchParams) *octen.DesignSearchData {
		t.Helper()
		start := time.Now()
		res, err := c.Search.DesignSearch(liveContext(t), p)
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Data == nil {
			t.Fatal("response has no data")
		}
		return res.Data
	}
	query := octen.String("SaaS pricing page")
	// webOnly turns the image results off, which every such call also checks.
	webOnly := func(t *testing.T, w *octen.DesignWebSearchOptions) []octen.DesignSearchResult {
		t.Helper()
		d := design(t, octen.DesignSearchParams{Query: query, Images: &octen.DesignImageOptions{Enable: octen.Bool(false)}, WebSearch: w})
		if len(d.Images) != 0 {
			t.Errorf("images.enable false still returned %d images", len(d.Images))
		}
		if len(d.Results) == 0 {
			t.Fatal("no web results to check")
		}
		if w.Count != nil && len(d.Results) > *w.Count {
			t.Errorf("web results = %d, want at most web_search.count %d", len(d.Results), *w.Count)
		}
		return d.Results
	}
	dated := func(rs []octen.DesignSearchResult) []datedResult {
		var out []datedResult
		for _, r := range rs {
			out = append(out, datedResult{r.URL, r.TimePublished})
		}
		return out
	}

	// One request with default options is the control for every check below.
	// Server defaults: 5 images and 5 web results, above the caps of 3 checked below.
	control := design(t, octen.DesignSearchParams{Query: query})
	if len(control.Results) == 0 {
		t.Fatal("control has no web results")
	}
	var controlURLs []string
	for _, r := range control.Results {
		controlURLs = append(controlURLs, r.URL)
	}

	// max_tokens 100 against the server defaults (512 highlight, 2048 full content), so dropping a cap makes the calls equal.
	t.Run("web_highlight_and_full_content_caps", func(t *testing.T) {
		withCaps := func(highlight, full int) map[string]octen.DesignSearchResult {
			out := map[string]octen.DesignSearchResult{}
			for _, r := range webOnly(t, &octen.DesignWebSearchOptions{Count: octen.Int(3),
				Highlight:   &octen.HighlightOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(highlight)},
				FullContent: &octen.FullContentOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(full)}}) {
				out[r.URL] = r
			}
			return out
		}
		large, small := withCaps(512, 2048), withCaps(100, 100)
		compared, shorterH, shorterF := 0, 0, 0
		for u, sr := range small {
			lr, ok := large[u]
			if !ok {
				continue
			}
			compared++
			t.Logf("%s highlight %d/%d full_content %d/%d chars (capped/default)", u, len(sr.Highlight), len(lr.Highlight), len(sr.FullContent), len(lr.FullContent))
			if sr.Highlight == "" || sr.FullContent == "" || len(sr.Highlight) > len(lr.Highlight) || len(sr.FullContent) > len(lr.FullContent) {
				t.Errorf("result %q: a capped field is empty or longer than the default one", u)
			}
			if len(sr.Highlight) < len(lr.Highlight) {
				shorterH++
			}
			if len(sr.FullContent) < len(lr.FullContent) {
				shorterF++
			}
		}
		if compared == 0 {
			t.Fatal("the two calls share no result URL")
		}
		if shorterH == 0 || shorterF == 0 {
			t.Errorf("of %d results, max_tokens 100 shortened %d highlights and %d full contents; want at least one each", compared, shorterH, shorterF)
		}
	})
	t.Run("design_facts_and_snippet_off", func(t *testing.T) {
		withFacts, withSnippet := 0, 0
		for _, img := range control.Images {
			if img.DesignFacts != nil {
				withFacts++
			}
			if img.Snippet != nil && img.Snippet.Code != "" {
				withSnippet++
			}
		}
		t.Logf("control images=%d with_design_facts=%d with_snippet=%d", len(control.Images), withFacts, withSnippet)
		if withFacts == 0 || withSnippet == 0 {
			t.Fatalf("control images lack design_facts (%d) or snippet (%d)", withFacts, withSnippet)
		}
		d := design(t, octen.DesignSearchParams{Query: query,
			Images:    &octen.DesignImageOptions{Count: octen.Int(2), DesignFacts: octen.Bool(false), Snippet: &octen.DesignSnippetOptions{Enable: octen.Bool(false)}},
			WebSearch: &octen.DesignWebSearchOptions{Enable: octen.Bool(false)}})
		if len(d.Images) == 0 {
			t.Fatal("no images to check")
		}
		for _, img := range d.Images {
			if img.DesignFacts != nil || img.Snippet != nil {
				t.Errorf("image %q still has design_facts or snippet", img.URL)
			}
		}
		if len(d.Results) != 0 {
			t.Errorf("web_search.enable false still returned %d web results", len(d.Results))
		}
	})
	t.Run("images_count_web_highlight_full_content_time_range_year", func(t *testing.T) {
		from, to := time.Now().AddDate(-1, 0, -1), time.Now().Add(time.Hour)
		requireControlOutside(t, "time_published", dated(control.Results), from, to)
		if len(control.Images) <= 3 || len(control.Results) <= 3 {
			t.Fatalf("control images = %d, web results = %d; both must exceed the cap of 3", len(control.Images), len(control.Results))
		}
		d := design(t, octen.DesignSearchParams{Query: query,
			Images:    &octen.DesignImageOptions{Count: octen.Int(3)},
			WebSearch: &octen.DesignWebSearchOptions{Count: octen.Int(3), TimeBasis: octen.String("published"), TimeRange: octen.String("year")}})
		if n := len(d.Images); n == 0 || n > 3 {
			t.Errorf("images = %d, want 1..3 for images.count 3", n)
		}
		if n := len(d.Results); n > 3 {
			t.Errorf("web results = %d, want at most 3", n)
		}
		checkDates(t, "time_published", dated(d.Results), from, to, timeBasisRule)
	})
	t.Run("web_time_basis_published_start_end", func(t *testing.T) {
		from, to := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
		rs := webOnly(t, &octen.DesignWebSearchOptions{Count: octen.Int(3), TimeBasis: octen.String("published"),
			StartTime: octen.String(from.Format(time.RFC3339)), EndTime: octen.String(to.Format(time.RFC3339))})
		checkDates(t, "time_published", dated(rs), from, to, timeBasisRule)
	})
	t.Run("web_language_ja", func(t *testing.T) {
		for _, r := range webOnly(t, &octen.DesignWebSearchOptions{Count: octen.Int(3), Language: []string{"ja"}}) {
			if !hasKana(r.Title + r.Highlight) {
				t.Errorf("web result %q (%q) has no kana", r.URL, r.Title)
			}
		}
	})
	t.Run("web_exclude_domains", func(t *testing.T) {
		excluded := hostOf(control.Results[0].URL)
		t.Logf("excluded_host=%s", excluded)
		for _, r := range webOnly(t, &octen.DesignWebSearchOptions{Count: octen.Int(5), ExcludeDomains: []string{excluded}}) {
			if hostOn(r.URL, excluded) {
				t.Errorf("web result %q is on the excluded domain", r.URL)
			}
		}
	})
	t.Run("web_include_domains", func(t *testing.T) {
		if n := distinctHosts(controlURLs); n < 2 {
			t.Fatalf("control shows %d host(s); include_domains cannot be shown", n)
		}
		target := hostOf(control.Results[0].URL)
		t.Logf("include_host=%s", target)
		for _, r := range webOnly(t, &octen.DesignWebSearchOptions{Count: octen.Int(5), IncludeDomains: []string{target}}) {
			if !hostOn(r.URL, target) {
				t.Errorf("web result %q is outside %s", r.URL, target)
			}
		}
	})
}
