package octen

import "encoding/json"

// WebSearchOptions are the per-query web search options of a broad search.
type WebSearchOptions struct {
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

// BroadSearchParams is the request body of BroadSearch.
type BroadSearchParams struct {
	Query         string            `json:"query"`
	MaxQueries    *int              `json:"max_queries,omitempty"`
	SearchOptions *WebSearchOptions `json:"search_options,omitempty"`
}

// SearchResultGroup holds the results of one expanded query.
type SearchResultGroup struct {
	Query   string         `json:"query,omitempty"`
	Results []SearchResult `json:"results,omitempty"`
	Latency int            `json:"latency,omitempty"`
}

// BroadSearchData is the data object of a broad search response.
type BroadSearchData struct {
	Query         string              `json:"query,omitempty"`
	Queries       []string            `json:"queries,omitempty"`
	SearchResults []SearchResultGroup `json:"search_results,omitempty"`
}

// BroadSearchResponse is the response of BroadSearch.
type BroadSearchResponse struct {
	Code      json.RawMessage  `json:"code,omitempty"`
	Msg       *string          `json:"msg,omitempty"`
	RequestID string           `json:"request_id"`
	Data      *BroadSearchData `json:"data,omitempty"`
	Meta      *SearchMeta      `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *BroadSearchResponse) RawJSON() []byte { return r.raw }
