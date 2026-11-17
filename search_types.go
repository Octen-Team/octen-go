package octen

import "encoding/json"

// HighlightOptions controls highlight extraction in a request.
type HighlightOptions struct {
	Enable    *bool `json:"enable,omitempty"`
	MaxTokens *int  `json:"max_tokens,omitempty"`
}

// FullContentOptions controls full-content extraction in a request.
type FullContentOptions struct {
	Enable    *bool `json:"enable,omitempty"`
	MaxTokens *int  `json:"max_tokens,omitempty"`
}

// SearchParams is the request body of Search.
type SearchParams struct {
	Query          string              `json:"query"`
	ResultType     *string             `json:"result_type,omitempty" enum:"standard,dynamic"`
	Count          *int                `json:"count,omitempty"`
	IncludeDomains []string            `json:"include_domains,omitempty"`
	ExcludeDomains []string            `json:"exclude_domains,omitempty"`
	IncludeText    []string            `json:"include_text,omitempty"`
	ExcludeText    []string            `json:"exclude_text,omitempty"`
	TimeBasis      *string             `json:"time_basis,omitempty" enum:"auto,published,crawled"`
	TimeRange      *string             `json:"time_range,omitempty" enum:"day,week,month,year,d,w,m,y"`
	StartTime      *string             `json:"start_time,omitempty"`
	EndTime        *string             `json:"end_time,omitempty"`
	Language       []string            `json:"language,omitempty" enum:"ar,de,en,es,fr,hi,id,it,ja,ko,nl,pl,pt,ru,th,tr,vi,zh"`
	Highlight      *HighlightOptions   `json:"highlight,omitempty"`
	FullContent    *FullContentOptions `json:"full_content,omitempty"`
	Format         *string             `json:"format,omitempty" enum:"markdown,text"`
	Safesearch     *string             `json:"safesearch,omitempty" enum:"off,strict"`
	IncludeImages  *bool               `json:"include_images,omitempty"`
}

// SearchResultImage is an image attached to a web result.
type SearchResultImage struct {
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
}

// SearchResult is one web result.
type SearchResult struct {
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

// SearchData is the data object of a search response.
type SearchData struct {
	Query   string         `json:"query,omitempty"`
	Results []SearchResult `json:"results,omitempty"`
}

// SearchUsage reports the billable work of a search call.
type SearchUsage struct {
	NumSearchQueries      int `json:"num_search_queries,omitempty"`
	FullContentExtraCount int `json:"full_content_extra_count,omitempty"`
}

// SearchMeta is the meta object of a search response.
type SearchMeta struct {
	Usage      *SearchUsage `json:"usage,omitempty"`
	ResultType string       `json:"result_type,omitempty"`
	Latency    float64      `json:"latency,omitempty"`
	Warning    *string      `json:"warning,omitempty"`
}

// SearchResponse is the response of Search.
type SearchResponse struct {
	Code      json.RawMessage `json:"code,omitempty"`
	Msg       *string         `json:"msg,omitempty"`
	RequestID string          `json:"request_id"`
	Data      *SearchData     `json:"data,omitempty"`
	Meta      *SearchMeta     `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *SearchResponse) RawJSON() []byte { return r.raw }
