package octen_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestLiveMatrixSearchPremier(t *testing.T) {
	c := liveClient(t)
	t.Run("include_domains_full_content_result_type_standard", func(t *testing.T) {
		start := time.Now()
		res, err := c.Search.SearchPremier(liveContext(t), octen.SearchParams{
			Query: "EU AI Act guidance", Count: octen.Int(3), ResultType: octen.String("standard"),
			IncludeDomains: []string{"europa.eu"}, Language: []string{"en"}, Format: octen.String("markdown"),
			FullContent: &octen.FullContentOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(300)},
		})
		if err != nil {
			t.Fatal(err)
		}
		matrixOK(t, res.RequestID, start)
		if res.Meta == nil || res.Meta.ResultType != "standard" {
			t.Errorf("meta = %+v, want result_type standard echoed", res.Meta)
		}
		if res.Data == nil || len(res.Data.Results) == 0 {
			t.Fatal("no results to check")
		}
		for _, r := range res.Data.Results {
			if !hostOn(r.URL, "europa.eu") {
				t.Errorf("result %q is outside europa.eu", r.URL)
			}
			if r.FullContent == "" {
				t.Errorf("result %q has no full_content", r.URL)
			}
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
	t.Run("defaults_then_group_lookup_positions_1_2", func(t *testing.T) {
		d := design(t, octen.DesignSearchParams{
			Query:     octen.String("SaaS landing page hero section"),
			Images:    &octen.DesignImageOptions{Count: octen.Int(2)},
			WebSearch: &octen.DesignWebSearchOptions{Count: octen.Int(1)},
		})
		if len(d.Images) == 0 || len(d.Results) == 0 {
			t.Fatalf("images = %d, results = %d, want both", len(d.Images), len(d.Results))
		}
		groupID, withFacts, withSnippet := "", 0, 0
		for _, img := range d.Images {
			if img.URL == "" {
				t.Errorf("image without url: %+v", img)
			}
			if img.Group != nil && groupID == "" {
				groupID = img.Group.GroupID
			}
			if img.DesignFacts != nil {
				withFacts++
			}
			if img.Snippet != nil && img.Snippet.Code != "" {
				withSnippet++
			}
		}
		// The control for the switched-off case below: by default the fields are there.
		t.Logf("with_design_facts=%d with_snippet=%d", withFacts, withSnippet)
		if withFacts == 0 || withSnippet == 0 {
			t.Errorf("default images lack design_facts (%d) or snippet (%d)", withFacts, withSnippet)
		}
		if groupID == "" {
			t.Fatal("no image carries a group")
		}
		g := design(t, octen.DesignSearchParams{Images: &octen.DesignImageOptions{GroupID: octen.String(groupID), Count: octen.Int(2)}})
		var positions []int
		for _, img := range g.Images {
			if img.Group == nil || img.Group.GroupID != groupID {
				t.Fatalf("image outside the group: %+v", img.Group)
			}
			positions = append(positions, img.Group.Position)
		}
		if len(positions) != 2 || positions[0] != 1 || positions[1] != 2 {
			t.Fatalf("positions = %v, want [1 2]", positions)
		}
	})
	t.Run("design_facts_and_snippet_off_web_include_domains", func(t *testing.T) {
		d := design(t, octen.DesignSearchParams{
			Query:     octen.String("SaaS pricing page"),
			Images:    &octen.DesignImageOptions{Count: octen.Int(2), DesignFacts: octen.Bool(false), Snippet: &octen.DesignSnippetOptions{Enable: octen.Bool(false)}},
			WebSearch: &octen.DesignWebSearchOptions{Count: octen.Int(2), IncludeDomains: []string{"stripe.com"}},
		})
		if len(d.Images) == 0 || len(d.Results) == 0 {
			t.Fatalf("images = %d, results = %d, want both", len(d.Images), len(d.Results))
		}
		for _, img := range d.Images {
			if img.DesignFacts != nil || img.Snippet != nil {
				t.Errorf("image %q still has design_facts or snippet", img.URL)
			}
		}
		for _, r := range d.Results {
			if !hostOn(r.URL, "stripe.com") {
				t.Errorf("web result %q is outside stripe.com", r.URL)
			}
		}
	})
	t.Run("images_and_web_disabled", func(t *testing.T) {
		d := design(t, octen.DesignSearchParams{
			Query:     octen.String("SaaS pricing page"),
			Images:    &octen.DesignImageOptions{Enable: octen.Bool(false)},
			WebSearch: &octen.DesignWebSearchOptions{Enable: octen.Bool(false)},
		})
		if len(d.Images) != 0 || len(d.Results) != 0 {
			t.Fatalf("images = %d, results = %d, want none", len(d.Images), len(d.Results))
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
