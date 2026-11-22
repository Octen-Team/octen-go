package octen_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	octen "github.com/Octen-Team/octen-go"
)

// Extract against production (billed per successful URL; about 10 URLs per full run), with
// OCTEN_LIVE=1 and OCTEN_API_KEY. Each option is checked against a control on the same URL, and
// every control asserts the property the option must change, so a control that cannot fail fails itself.
const (
	// A long article with markdown structure and many links.
	extractA = "https://en.wikipedia.org/wiki/Web_scraping"
	// A page with images and a cover image.
	extractB = "https://octen.ai/"
	// Only the max_age_seconds case fetches it, so its cache age comes from earlier runs.
	extractC      = "https://www.iana.org/help/example-domains"
	extractBroken = "https://octen-sdk-live-check.invalid/"
)

var (
	markdownLink    = regexp.MustCompile(`\]\(https?://`)
	markdownHeading = regexp.MustCompile(`(?m)^#{1,6} `)
)

type extractItems map[string]octen.ExtractResult

// callExtract logs the billing line, checks that usage agrees with the items and returns them keyed by URL.
func callExtract(t *testing.T, c *octen.Client, name string, p octen.ExtractParams) (extractItems, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	res, err := c.Extract.Extract(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if res.Data == nil || res.Meta == nil || res.Meta.Usage == nil || res.Meta.Usage.SuccessfulByMode == nil {
		return nil, fmt.Errorf("%s: response lacks data or meta.usage: %s", name, res.RawJSON())
	}
	u := res.Meta.Usage
	usage, _ := json.Marshal(u)
	t.Logf("%s request_id=%s usage=%s", name, res.RequestID, usage)

	items := extractItems{}
	var got []string
	ok, standard, advanced := 0, 0, 0
	for _, r := range res.Data.Results {
		items[r.URL] = r
		got = append(got, r.URL)
		if r.Status == "success" {
			ok++
			switch r.ResolvedMode {
			case "standard":
				standard++
			case "advanced":
				advanced++
			}
		}
	}
	want := append([]string(nil), p.URLs...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		return nil, fmt.Errorf("%s: result URLs %q, want one per requested URL %q", name, got, want)
	}
	if u.TotalURLs != len(p.URLs) || u.SuccessfulURLs != ok || u.SuccessfulByMode.StandardURLs != standard || u.SuccessfulByMode.AdvancedURLs != advanced {
		return nil, fmt.Errorf("%s: usage %s disagrees with the items (%d urls, %d ok, %d standard, %d advanced)",
			name, usage, len(p.URLs), ok, standard, advanced)
	}
	return items, nil
}

func liveExtractCall(t *testing.T, c *octen.Client, name string, p octen.ExtractParams) extractItems {
	t.Helper()
	items, err := callExtract(t, c, name, p)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func extractSuccess(t *testing.T, items extractItems, url, name string) octen.ExtractResult {
	t.Helper()
	r, ok := items[url]
	if !ok || r.Status != "success" {
		t.Fatalf("%s: %s status %q (error_message %q), want success", name, url, r.Status, r.ErrorMessage)
	}
	return r
}

func content(r octen.ExtractResult) string {
	if r.FullContent == nil {
		return ""
	}
	return *r.FullContent
}

func TestLiveExtract(t *testing.T) {
	c := liveClient(t)

	// The no-option batch: A and B as plain standard-mode controls, plus a URL that cannot resolve.
	var once sync.Once
	var plainItems extractItems
	var plainErr error
	plain := func(t *testing.T) extractItems {
		t.Helper()
		once.Do(func() {
			plainItems, plainErr = callExtract(t, c, "plain batch", octen.ExtractParams{URLs: []string{extractA, extractBroken, extractB}})
		})
		if plainErr != nil {
			t.Fatal(plainErr)
		}
		return plainItems
	}

	t.Run("per_url_failure_while_others_succeed", func(t *testing.T) {
		items := plain(t)
		for _, url := range []string{extractA, extractB} {
			r := extractSuccess(t, items, url, "plain")
			if n := len(content(r)); n <= 200 || r.ResolvedMode != "standard" || r.ErrorMessage != "" {
				t.Errorf("%s: full_content %d chars, resolved_mode %q, error_message %q", url, n, r.ResolvedMode, r.ErrorMessage)
			}
		}
		broken := items[extractBroken]
		t.Logf("broken item: status=%q error_message=%q", broken.Status, broken.ErrorMessage)
		if broken.Status != "failed" || broken.ErrorMessage == "" || broken.FullContent != nil {
			t.Errorf("broken item = %+v, want failed with an error_message and no content", broken)
		}
	})

	t.Run("query_returns_highlights_not_full_content", func(t *testing.T) {
		control := extractSuccess(t, plain(t), extractA, "control")
		if control.Highlights != nil || len(content(control)) <= 200 {
			t.Fatalf("control: highlights %q, full_content %d chars; want none and the page", control.Highlights, len(content(control)))
		}
		r := extractSuccess(t, liveExtractCall(t, c, "query", octen.ExtractParams{URLs: []string{extractA}, Query: octen.String("legal issues and court cases")}), extractA, "query")
		t.Logf("highlights=%d", len(r.Highlights))
		if len(r.Highlights) == 0 || r.FullContent != nil {
			t.Fatalf("highlights %d, full_content set %v; want highlights and no full_content", len(r.Highlights), r.FullContent != nil)
		}
	})

	t.Run("mode_advanced_resolves_to_advanced", func(t *testing.T) {
		if m := extractSuccess(t, plain(t), extractA, "control").ResolvedMode; m != "standard" {
			t.Fatalf("control resolved_mode = %q, want standard", m)
		}
		r := extractSuccess(t, liveExtractCall(t, c, "advanced", octen.ExtractParams{URLs: []string{extractA}, Mode: octen.String("advanced")}), extractA, "advanced")
		// The octen Python SDK 0.13.0 docstring says an advanced request can resolve to standard;
		// kept strict so a dropped mode is caught.
		if r.ResolvedMode != "advanced" {
			t.Fatalf("resolved_mode = %q, want advanced", r.ResolvedMode)
		}
	})

	t.Run("format_text_has_no_markdown", func(t *testing.T) {
		control := content(extractSuccess(t, plain(t), extractA, "control"))
		if !markdownLink.MatchString(control) && !markdownHeading.MatchString(control) {
			t.Fatal("control has no markdown, so it cannot tell text from markdown")
		}
		text := content(extractSuccess(t, liveExtractCall(t, c, "text", octen.ExtractParams{URLs: []string{extractA}, Format: octen.String("text")}), extractA, "text"))
		if len(text) <= 200 || markdownLink.MatchString(text) || markdownHeading.MatchString(text) {
			t.Fatalf("text: %d chars, markdown link %v, heading %v", len(text), markdownLink.MatchString(text), markdownHeading.MatchString(text))
		}
	})

	t.Run("include_links_cap_3_vs_defaults_vs_omitted", func(t *testing.T) {
		if n := len(extractSuccess(t, plain(t), extractA, "control").Links); n != 0 {
			t.Fatalf("control without include_links has %d links, want 0", n)
		}
		defaults := extractSuccess(t, liveExtractCall(t, c, "links {}", octen.ExtractParams{URLs: []string{extractA}, IncludeLinks: &octen.ExtractLinksOptions{}}), extractA, "links {}")
		if len(defaults.Links) <= 3 {
			t.Errorf("include_links {} returned %d links, want more than 3", len(defaults.Links))
		}
		capped := extractSuccess(t, liveExtractCall(t, c, "links 3", octen.ExtractParams{URLs: []string{extractA}, IncludeLinks: &octen.ExtractLinksOptions{MaxLinks: octen.Int(3)}}), extractA, "links 3")
		t.Logf("links {}=%d capped=%d", len(defaults.Links), len(capped.Links))
		if len(capped.Links) != 3 {
			t.Fatalf("max_links 3 returned %d links", len(capped.Links))
		}
		for _, l := range capped.Links {
			if l.URL == "" {
				t.Errorf("link without url: %+v", l)
			}
		}
	})

	t.Run("include_images_returns_images_and_cover", func(t *testing.T) {
		control := extractSuccess(t, plain(t), extractB, "control")
		if len(control.Images) != 0 || control.CoverImage != nil {
			t.Fatalf("control: %d images, cover %v; want neither", len(control.Images), control.CoverImage)
		}
		r := extractSuccess(t, liveExtractCall(t, c, "images", octen.ExtractParams{URLs: []string{extractB}, IncludeImages: octen.Bool(true)}), extractB, "images")
		t.Logf("images=%d cover=%v", len(r.Images), r.CoverImage)
		if len(r.Images) == 0 || r.CoverImage == nil || r.CoverImage.URL == "" {
			t.Fatalf("images %d, cover %v; want both", len(r.Images), r.CoverImage)
		}
		for _, img := range r.Images {
			if img.URL == "" {
				t.Errorf("image without url")
			}
		}
	})

	t.Run("max_age_seconds_300_refetches_an_older_copy", func(t *testing.T) {
		const margin = time.Minute
		control := extractSuccess(t, liveExtractCall(t, c, "max-age control", octen.ExtractParams{URLs: []string{extractC}}), extractC, "max-age control")
		cachedAt, err := time.Parse(time.RFC3339, control.TimeLastCrawled)
		if err != nil {
			t.Fatalf("control time_last_crawled %q: %v", control.TimeLastCrawled, err)
		}
		age := time.Since(cachedAt)
		t.Logf("control time_last_crawled=%s age=%s", control.TimeLastCrawled, age.Round(time.Second))
		if age <= 300*time.Second+margin {
			t.Skip("inconclusive: the default-cache copy is not older than 300 s plus margin")
		}
		started := time.Now()
		r := extractSuccess(t, liveExtractCall(t, c, "max-age 300", octen.ExtractParams{URLs: []string{extractC}, MaxAgeSeconds: octen.Int(300)}), extractC, "max-age 300")
		crawledAt, err := time.Parse(time.RFC3339, r.TimeLastCrawled)
		t.Logf("max-age 300 time_last_crawled=%s", r.TimeLastCrawled)
		if err != nil || crawledAt.Before(started.Add(-300*time.Second)) || !crawledAt.After(cachedAt) {
			t.Fatalf("time_last_crawled %q (%v), want within 300 s of the call and newer than the control's %s", r.TimeLastCrawled, err, control.TimeLastCrawled)
		}
	})

	t.Run("rejected_locally_sends_nothing", func(t *testing.T) {
		var sent atomic.Int32
		hc := &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
			sent.Add(1)
			return http.DefaultTransport.RoundTrip(r)
		})}
		counting := liveClient(t, octen.WithHTTPClient(hc))
		tooMany := make([]string, 21)
		for i := range tooMany {
			tooMany[i] = fmt.Sprintf("%s%d", extractBroken, i)
		}
		for _, tc := range []struct {
			name, field string
			urls        []string
		}{
			{"21 URLs", "urls", tooMany},
			{"no URLs", "urls", nil},
			{"a URL of 2049 characters", "urls[0]", []string{extractBroken + strings.Repeat("a", 2049-len(extractBroken))}},
			{"a blank URL", "urls[1]", []string{extractBroken, "  "}},
		} {
			_, err := counting.Extract.Extract(liveContext(t), octen.ExtractParams{URLs: tc.urls})
			var ve *octen.ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Errorf("%s: err %v, want *ValidationError on %s", tc.name, err, tc.field)
			}
		}
		if n := sent.Load(); n != 0 {
			t.Fatalf("%d requests sent, want 0", n)
		}
	})

	// Ranges outside urls are the server's to check; a URL that cannot resolve keeps them unbilled even if accepted.
	t.Run("server_side_bounds_are_400", func(t *testing.T) {
		broken := func(p octen.ExtractParams) octen.ExtractParams { p.URLs = []string{extractBroken}; return p }
		// field is the name the observed message ("Invalid params. ... <field> ...") carries.
		for _, tc := range []struct {
			name, field string
			p           octen.ExtractParams
		}{
			{"include_links.max_links 0", "include_links.max_links", broken(octen.ExtractParams{IncludeLinks: &octen.ExtractLinksOptions{MaxLinks: octen.Int(0)}})},
			{"include_links.max_links 1001", "include_links.max_links", broken(octen.ExtractParams{IncludeLinks: &octen.ExtractLinksOptions{MaxLinks: octen.Int(1001)}})},
			{"a query of 501 characters", "query", broken(octen.ExtractParams{Query: octen.String(strings.Repeat("q", 501))})},
			{"unknown mode", "mode", broken(octen.ExtractParams{Mode: octen.String("fastest")})},
			{"unknown format", "format", broken(octen.ExtractParams{Format: octen.String("html")})},
			{"unknown include_links.scope", "include_links.scope", broken(octen.ExtractParams{IncludeLinks: &octen.ExtractLinksOptions{Scope: octen.String("prefer_none")}})},
		} {
			_, err := c.Extract.Extract(liveContext(t), tc.p)
			t.Logf("%s: %v", tc.name, err)
			var apiErr *octen.APIError
			if !errors.Is(err, octen.ErrBadRequest) || !errors.As(err, &apiErr) || apiErr.StatusCode != 400 {
				t.Errorf("%s: err %v, want a structured 400 *APIError", tc.name, err)
				continue
			}
			if !strings.Contains(apiErr.Message, tc.field) {
				t.Errorf("%s: message %q does not name %s", tc.name, apiErr.Message, tc.field)
			}
		}
	})

	// The reference says out-of-range timeout and max_age_seconds are adjusted, not rejected. The
	// call still succeeds; meta.warning naming the field shows the value reached the server.
	t.Run("out_of_range_timeout_and_max_age_are_adjusted", func(t *testing.T) {
		for _, tc := range []struct {
			name, field string
			p           octen.ExtractParams
		}{
			{"timeout 0", "timeout", octen.ExtractParams{URLs: []string{extractBroken}, Timeout: octen.Int(0)}},
			{"timeout 61", "timeout", octen.ExtractParams{URLs: []string{extractBroken}, Timeout: octen.Int(61)}},
			{"max_age_seconds 0", "max_age_seconds", octen.ExtractParams{URLs: []string{extractBroken}, MaxAgeSeconds: octen.Int(0)}},
		} {
			res, err := c.Extract.Extract(liveContext(t), tc.p)
			if err != nil {
				t.Errorf("%s: err %v, want the value adjusted and the call accepted", tc.name, err)
				continue
			}
			warning := ""
			if res.Meta != nil && res.Meta.Warning != nil {
				warning = *res.Meta.Warning
			}
			t.Logf("%s: request_id=%s warning=%q", tc.name, res.RequestID, warning)
			if !strings.Contains(warning, tc.field) {
				t.Errorf("%s: meta.warning %q does not mention %s", tc.name, warning, tc.field)
			}
		}
	})
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
