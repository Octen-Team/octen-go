package octen

import "encoding/json"

// DesignSnippetOptions controls the reusable code returned with image results that have it.
type DesignSnippetOptions struct {
	Enable    *bool `json:"enable,omitempty"`
	MaxTokens *int  `json:"max_tokens,omitempty"`
}

// DesignImageOptions controls the image results of a design search. GroupID returns the images
// of that group in group order instead of searching by query; it takes the Group.GroupID of an
// image result.
type DesignImageOptions struct {
	Enable      *bool                 `json:"enable,omitempty"`
	Count       *int                  `json:"count,omitempty"`
	DesignFacts *bool                 `json:"design_facts,omitempty"`
	Snippet     *DesignSnippetOptions `json:"snippet,omitempty"`
	GroupID     *string               `json:"group_id,omitempty"`
}

// DesignWebSearchOptions controls the web results of a design search; it does not affect the image results.
type DesignWebSearchOptions struct {
	Enable         *bool               `json:"enable,omitempty"`
	Count          *int                `json:"count,omitempty"`
	Highlight      *HighlightOptions   `json:"highlight,omitempty"`
	FullContent    *FullContentOptions `json:"full_content,omitempty"`
	IncludeDomains []string            `json:"include_domains,omitempty"`
	ExcludeDomains []string            `json:"exclude_domains,omitempty"`
	Language       []string            `json:"language,omitempty" enum:"ar,de,en,es,fr,hi,id,it,ja,ko,nl,pl,pt,ru,th,tr,vi,zh"`
	TimeBasis      *string             `json:"time_basis,omitempty" enum:"auto,published,crawled"`
	TimeRange      *string             `json:"time_range,omitempty" enum:"day,week,month,year,d,w,m,y"`
	StartTime      *string             `json:"start_time,omitempty"`
	EndTime        *string             `json:"end_time,omitempty"`
}

// DesignSearchParams is the request body of DesignSearch. Query is required unless
// Images.GroupID is set.
type DesignSearchParams struct {
	Query     *string                 `json:"query,omitempty"`
	Images    *DesignImageOptions     `json:"images,omitempty"`
	WebSearch *DesignWebSearchOptions `json:"web_search,omitempty"`
}

func (p DesignSearchParams) validate() error {
	if isBlank(p.Query) && (p.Images == nil || isBlank(p.Images.GroupID)) {
		return &ValidationError{Field: "query", Message: "query is required unless images.group_id is set, and must be a non-empty string."}
	}
	return nil
}

// DesignImageGroup places an image result in its page or deck: Position counts from 1 to Total.
type DesignImageGroup struct {
	GroupID  string         `json:"group_id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Name     string         `json:"name,omitempty"`
	Position int            `json:"position,omitempty"`
	Total    int            `json:"total,omitempty"`
	Master   map[string]any `json:"master,omitempty"`
}

// DesignSnippet is reusable code for an image result.
type DesignSnippet struct {
	Format string `json:"format,omitempty"`
	Code   string `json:"code,omitempty"`
}

// DesignImageResult is one design reference: a web page, page section, app screen or slide.
// DesignFacts holds the design values decided by the server.
type DesignImageResult struct {
	Type        string            `json:"type,omitempty"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	URL         string            `json:"url,omitempty"`
	Thumbnail   string            `json:"thumbnail,omitempty"`
	SourcePage  string            `json:"source_page,omitempty"`
	Width       int               `json:"width,omitempty"`
	Height      int               `json:"height,omitempty"`
	Group       *DesignImageGroup `json:"group,omitempty"`
	DesignFacts map[string]any    `json:"design_facts,omitempty"`
	Snippet     *DesignSnippet    `json:"snippet,omitempty"`
}

// DesignSearchResult is one web result of a design search.
type DesignSearchResult struct {
	Title           string              `json:"title,omitempty"`
	URL             string              `json:"url,omitempty"`
	Highlight       string              `json:"highlight,omitempty"`
	FullContent     string              `json:"full_content,omitempty"`
	Authors         string              `json:"authors,omitempty"`
	TimePublished   string              `json:"time_published,omitempty"`
	TimeLastCrawled string              `json:"time_last_crawled,omitempty"`
	Favicon         string              `json:"favicon,omitempty"`
	CoverImage      *SearchResultImage  `json:"cover_image,omitempty"`
	Images          []SearchResultImage `json:"images,omitempty"`
}

// DesignSearchData is the data object of a design search response.
type DesignSearchData struct {
	Query   string               `json:"query,omitempty"`
	Images  []DesignImageResult  `json:"images,omitempty"`
	Results []DesignSearchResult `json:"results,omitempty"`
}

// DesignSearchUsage reports the billable work of a design search call.
type DesignSearchUsage struct {
	NumSearchQueries int `json:"num_search_queries,omitempty"`
}

// DesignSearchMeta is the meta object of a design search response.
type DesignSearchMeta struct {
	Usage   *DesignSearchUsage `json:"usage,omitempty"`
	Latency float64            `json:"latency,omitempty"`
	Warning *string            `json:"warning,omitempty"`
}

// DesignSearchResponse is the response of DesignSearch.
type DesignSearchResponse struct {
	Code      json.RawMessage   `json:"code,omitempty"`
	Msg       *string           `json:"msg,omitempty"`
	RequestID string            `json:"request_id"`
	Data      *DesignSearchData `json:"data,omitempty"`
	Meta      *DesignSearchMeta `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *DesignSearchResponse) RawJSON() []byte { return r.raw }
