package octen

import "encoding/json"

// PlaceCardOptions controls place cards in a place search.
type PlaceCardOptions struct {
	Count      *int `json:"count,omitempty"`
	MaxReviews *int `json:"max_reviews,omitempty"`
}

// PlaceWebSearchOptions controls the optional web search that accompanies a place search.
type PlaceWebSearchOptions struct {
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

// PlaceUserLocation is an approximate user location.
type PlaceUserLocation struct {
	Type     *string `json:"type,omitempty" enum:"approximate"`
	City     *string `json:"city,omitempty"`
	Region   *string `json:"region,omitempty"`
	Country  *string `json:"country,omitempty"`
	Timezone *string `json:"timezone,omitempty"`
}

// PlaceSearchParams is the request body of PlaceSearch.
type PlaceSearchParams struct {
	Query        string                 `json:"query"`
	Places       *PlaceCardOptions      `json:"places,omitempty"`
	WebSearch    *PlaceWebSearchOptions `json:"web_search,omitempty"`
	UserLocation *PlaceUserLocation     `json:"user_location,omitempty"`
	Latitude     *float64               `json:"latitude,omitempty"`
	Longitude    *float64               `json:"longitude,omitempty"`
	Radius       *int                   `json:"radius,omitempty"`
}

// PlaceSearchLocation is the location the search was resolved to.
type PlaceSearchLocation struct {
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

// Place is one place card. Attributes holds amenity flags decided by the server.
type Place struct {
	Name         string         `json:"name,omitempty"`
	Category     string         `json:"category,omitempty"`
	Address      string         `json:"address,omitempty"`
	Latitude     float64        `json:"latitude,omitempty"`
	Longitude    float64        `json:"longitude,omitempty"`
	Distance     int            `json:"distance,omitempty"`
	Phone        string         `json:"phone,omitempty"`
	Website      string         `json:"website,omitempty"`
	OpeningHours []string       `json:"opening_hours,omitempty"`
	Status       string         `json:"status,omitempty"`
	Rating       float64        `json:"rating,omitempty"`
	ReviewCount  int            `json:"review_count,omitempty"`
	Attributes   map[string]any `json:"attributes,omitempty"`
}

// PlaceSearchResult is one web result of a place search.
type PlaceSearchResult struct {
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

// PlaceSearchData is the data object of a place search response.
type PlaceSearchData struct {
	Query    string               `json:"query,omitempty"`
	Location *PlaceSearchLocation `json:"location,omitempty"`
	Places   []Place              `json:"places,omitempty"`
	Results  []PlaceSearchResult  `json:"results,omitempty"`
}

// PlaceSearchUsage reports the billable work of a place search call.
type PlaceSearchUsage struct {
	NumSearchQueries      int `json:"num_search_queries,omitempty"`
	FullContentExtraCount int `json:"full_content_extra_count,omitempty"`
}

// PlaceSearchMeta is the meta object of a place search response.
type PlaceSearchMeta struct {
	Usage   *PlaceSearchUsage `json:"usage,omitempty"`
	Latency float64           `json:"latency,omitempty"`
	Warning *string           `json:"warning,omitempty"`
}

// PlaceSearchResponse is the response of PlaceSearch.
type PlaceSearchResponse struct {
	Code      json.RawMessage  `json:"code,omitempty"`
	Msg       *string          `json:"msg,omitempty"`
	RequestID string           `json:"request_id"`
	Data      *PlaceSearchData `json:"data,omitempty"`
	Meta      *PlaceSearchMeta `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *PlaceSearchResponse) RawJSON() []byte { return r.raw }
