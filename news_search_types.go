package octen

import "encoding/json"

// NewsSubjectOptions controls subject clustering in a news search.
type NewsSubjectOptions struct {
	Enable     *bool `json:"enable,omitempty"`
	Count      *int  `json:"count,omitempty"`
	MaxSubNews *int  `json:"max_sub_news,omitempty"`
}

// NewsSearchParams is the request body of NewsSearch.
type NewsSearchParams struct {
	Query          string              `json:"query"`
	Count          *int                `json:"count,omitempty"`
	IncludeDomains []string            `json:"include_domains,omitempty"`
	ExcludeDomains []string            `json:"exclude_domains,omitempty"`
	TimeBasis      *string             `json:"time_basis,omitempty" enum:"auto,published,crawled"`
	TimeRange      *string             `json:"time_range,omitempty" enum:"day,week,month,year,d,w,m,y"`
	StartTime      *string             `json:"start_time,omitempty"`
	EndTime        *string             `json:"end_time,omitempty"`
	Language       []string            `json:"language,omitempty" enum:"ar,de,en,es,fr,hi,id,it,ja,ko,nl,pl,pt,ru,th,tr,vi,zh"`
	Highlight      *HighlightOptions   `json:"highlight,omitempty"`
	FullContent    *FullContentOptions `json:"full_content,omitempty"`
	Subjects       *NewsSubjectOptions `json:"subjects,omitempty"`
}

// NewsImage is an image attached to a news result.
type NewsImage struct {
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
}

// NewsSearchResult is one news result.
type NewsSearchResult struct {
	Title           string      `json:"title,omitempty"`
	URL             string      `json:"url,omitempty"`
	Highlight       string      `json:"highlight,omitempty"`
	FullContent     string      `json:"full_content,omitempty"`
	Authors         string      `json:"authors,omitempty"`
	TimePublished   string      `json:"time_published,omitempty"`
	TimeLastCrawled string      `json:"time_last_crawled,omitempty"`
	Favicon         string      `json:"favicon,omitempty"`
	CoverImage      *NewsImage  `json:"cover_image,omitempty"`
	Images          []NewsImage `json:"images,omitempty"`
}

// NewsSubjectTopNews is the lead article of a news subject.
type NewsSubjectTopNews struct {
	Title           string      `json:"title,omitempty"`
	URL             string      `json:"url,omitempty"`
	Summary         string      `json:"summary,omitempty"`
	Highlight       string      `json:"highlight,omitempty"`
	FullContent     string      `json:"full_content,omitempty"`
	Authors         string      `json:"authors,omitempty"`
	TimePublished   string      `json:"time_published,omitempty"`
	TimeLastCrawled string      `json:"time_last_crawled,omitempty"`
	Favicon         string      `json:"favicon,omitempty"`
	CoverImage      *NewsImage  `json:"cover_image,omitempty"`
	Images          []NewsImage `json:"images,omitempty"`
}

// NewsSubjectSubNews is a supporting article of a news subject.
type NewsSubjectSubNews struct {
	Title           string      `json:"title,omitempty"`
	URL             string      `json:"url,omitempty"`
	Highlight       string      `json:"highlight,omitempty"`
	FullContent     string      `json:"full_content,omitempty"`
	Authors         string      `json:"authors,omitempty"`
	TimePublished   string      `json:"time_published,omitempty"`
	TimeLastCrawled string      `json:"time_last_crawled,omitempty"`
	Favicon         string      `json:"favicon,omitempty"`
	CoverImage      *NewsImage  `json:"cover_image,omitempty"`
	Images          []NewsImage `json:"images,omitempty"`
}

// NewsSubject is a cluster of articles about one story.
type NewsSubject struct {
	Name       string               `json:"name,omitempty"`
	Summary    string               `json:"summary,omitempty"`
	TimeStart  string               `json:"time_start,omitempty"`
	TimeLatest string               `json:"time_latest,omitempty"`
	TopNews    *NewsSubjectTopNews  `json:"top_news,omitempty"`
	SubNews    []NewsSubjectSubNews `json:"sub_news,omitempty"`
}

// NewsSearchData is the data object of a news search response.
type NewsSearchData struct {
	Query    string             `json:"query,omitempty"`
	Results  []NewsSearchResult `json:"results,omitempty"`
	Subjects []NewsSubject      `json:"subjects,omitempty"`
}

// NewsSearchUsage reports the billable work of a news search call.
type NewsSearchUsage struct {
	NumSearchQueries        int `json:"num_search_queries,omitempty"`
	NumSubjectSearchQueries int `json:"num_subject_search_queries,omitempty"`
	FullContentExtraCount   int `json:"full_content_extra_count,omitempty"`
}

// NewsSearchMeta is the meta object of a news search response.
type NewsSearchMeta struct {
	Usage   *NewsSearchUsage `json:"usage,omitempty"`
	Latency float64          `json:"latency,omitempty"`
	Warning *string          `json:"warning,omitempty"`
}

// NewsSearchResponse is the response of NewsSearch.
type NewsSearchResponse struct {
	Code      json.RawMessage `json:"code,omitempty"`
	Msg       *string         `json:"msg,omitempty"`
	RequestID string          `json:"request_id"`
	Data      *NewsSearchData `json:"data,omitempty"`
	Meta      *NewsSearchMeta `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *NewsSearchResponse) RawJSON() []byte { return r.raw }
