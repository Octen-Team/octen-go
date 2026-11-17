package octen

import "encoding/json"

// PulseSearchParams is the request body of PulseSearch.
type PulseSearchParams struct {
	Query         string              `json:"query"`
	Mode          *string             `json:"mode,omitempty" enum:"lite,standard"`
	Count         *int                `json:"count,omitempty"`
	Highlight     *HighlightOptions   `json:"highlight,omitempty"`
	FullContent   *FullContentOptions `json:"full_content,omitempty"`
	Safesearch    *string             `json:"safesearch,omitempty" enum:"off,strict"`
	IncludeImages *bool               `json:"include_images,omitempty"`
}

// PulseSearchResult is one Pulse result; Title is null for some pages.
type PulseSearchResult struct {
	Title           *string             `json:"title,omitempty"`
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

// PulseSearchData is the data object of a Pulse search response.
type PulseSearchData struct {
	Query   string              `json:"query,omitempty"`
	Results []PulseSearchResult `json:"results,omitempty"`
}

// PulseSearchUsage reports the billable work of a Pulse search call.
type PulseSearchUsage struct {
	NumSearchQueries      int    `json:"num_search_queries,omitempty"`
	FullContentExtraCount int    `json:"full_content_extra_count,omitempty"`
	BilledMode            string `json:"billed_mode,omitempty"`
}

// PulseSearchMeta is the meta object of a Pulse search response.
type PulseSearchMeta struct {
	Usage   *PulseSearchUsage `json:"usage,omitempty"`
	Latency float64           `json:"latency,omitempty"`
	Warning *string           `json:"warning,omitempty"`
}

// PulseSearchResponse is the response of PulseSearch.
type PulseSearchResponse struct {
	Code      json.RawMessage  `json:"code,omitempty"`
	Msg       *string          `json:"msg,omitempty"`
	RequestID string           `json:"request_id"`
	Data      *PulseSearchData `json:"data,omitempty"`
	Meta      *PulseSearchMeta `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *PulseSearchResponse) RawJSON() []byte { return r.raw }
