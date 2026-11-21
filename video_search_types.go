package octen

import "encoding/json"

// VideoSearchInput is the input of a video search: a text query, with Type "text".
type VideoSearchInput struct {
	Type string  `json:"type" enum:"text"`
	Data *string `json:"data,omitempty"`
}

// VideoSearchParams is the request body of VideoSearch. Inputs must hold exactly one text input.
type VideoSearchParams struct {
	Inputs     []VideoSearchInput `json:"inputs"`
	Count      *int               `json:"count,omitempty"`
	TimeRange  *string            `json:"time_range,omitempty" enum:"day,week,month,year,d,w,m,y"`
	StartTime  *string            `json:"start_time,omitempty"`
	EndTime    *string            `json:"end_time,omitempty"`
	Safesearch *string            `json:"safesearch,omitempty" enum:"off,strict"`
}

func (p VideoSearchParams) validate() error {
	if len(p.Inputs) != 1 {
		return &ValidationError{Field: "inputs", Message: "inputs must hold exactly one text input."}
	}
	if p.Inputs[0].Type != "text" {
		return &ValidationError{Field: "inputs[0].type", Message: `type must be "text".`}
	}
	if isBlank(p.Inputs[0].Data) {
		return &ValidationError{Field: "inputs[0].data", Message: "a text input needs a non-empty data string."}
	}
	return nil
}

// VideoSearchResult is one video result.
type VideoSearchResult struct {
	Title           string `json:"title,omitempty"`
	URL             string `json:"url,omitempty"`
	SourcePage      string `json:"source_page,omitempty"`
	Description     string `json:"description,omitempty"`
	CoverURL        string `json:"cover_url,omitempty"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
	Authors         string `json:"authors,omitempty"`
	TimePublished   string `json:"time_published,omitempty"`
	TimeLastCrawled string `json:"time_last_crawled,omitempty"`
}

// VideoSearchData is the data object of a video search response.
type VideoSearchData struct {
	Results []VideoSearchResult `json:"results,omitempty"`
}

// VideoSearchUsage reports the billable work of a video search call.
type VideoSearchUsage struct {
	NumSearchQueries int `json:"num_search_queries,omitempty"`
}

// VideoSearchMeta is the meta object of a video search response.
type VideoSearchMeta struct {
	Usage   *VideoSearchUsage `json:"usage,omitempty"`
	Latency float64           `json:"latency,omitempty"`
	Warning *string           `json:"warning,omitempty"`
}

// VideoSearchResponse is the response of VideoSearch.
type VideoSearchResponse struct {
	Code      json.RawMessage  `json:"code,omitempty"`
	Msg       *string          `json:"msg,omitempty"`
	RequestID string           `json:"request_id"`
	Data      *VideoSearchData `json:"data,omitempty"`
	Meta      *VideoSearchMeta `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *VideoSearchResponse) RawJSON() []byte { return r.raw }
