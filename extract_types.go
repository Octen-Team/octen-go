package octen

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

const (
	extractMaxURLs      = 20
	extractMaxURLLength = 2048
)

// ExtractLinksOptions asks for the links found on each page. An empty value (&ExtractLinksOptions{})
// is sent as {} and uses the server defaults; nil leaves links out.
type ExtractLinksOptions struct {
	Scope    *string `json:"scope,omitempty" enum:"prefer_internal,prefer_external"`
	MaxLinks *int    `json:"max_links,omitempty"`
}

// ExtractParams is the request body of Extract. URLs must hold 1 to 20 URLs, each non-blank and at
// most 2048 characters. Timeout is the per-URL limit on the server, not the HTTP timeout of the call.
type ExtractParams struct {
	URLs          []string             `json:"urls"`
	Mode          *string              `json:"mode,omitempty" enum:"standard,advanced,auto"`
	Query         *string              `json:"query,omitempty"`
	MaxAgeSeconds *int                 `json:"max_age_seconds,omitempty"`
	Format        *string              `json:"format,omitempty" enum:"markdown,text"`
	Timeout       *int                 `json:"timeout,omitempty"`
	IncludeImages *bool                `json:"include_images,omitempty"`
	IncludeVideos *bool                `json:"include_videos,omitempty"`
	IncludeAudio  *bool                `json:"include_audio,omitempty"`
	IncludeLinks  *ExtractLinksOptions `json:"include_links,omitempty"`
}

// validate counts URL length in Unicode code points, like the TypeScript SDK.
func (p ExtractParams) validate() error {
	if len(p.URLs) == 0 || len(p.URLs) > extractMaxURLs {
		return &ValidationError{Field: "urls", Message: "urls is required and must hold 1 to " + strconv.Itoa(extractMaxURLs) + " URLs."}
	}
	for i, u := range p.URLs {
		field := "urls[" + strconv.Itoa(i) + "]"
		if isBlank(&u) {
			return &ValidationError{Field: field, Message: field + " must be a non-empty string."}
		}
		if utf8.RuneCountInString(u) > extractMaxURLLength {
			return &ValidationError{Field: field, Message: field + " is longer than " + strconv.Itoa(extractMaxURLLength) + " characters."}
		}
	}
	return nil
}

// ExtractMediaResource is an image, video or audio resource found on the page.
type ExtractMediaResource struct {
	URL string `json:"url,omitempty"`
}

// ExtractLink is a link found on the page.
type ExtractLink struct {
	URL        string `json:"url,omitempty"`
	AnchorText string `json:"anchor_text,omitempty"`
	IsExternal bool   `json:"is_external,omitempty"`
}

// ExtractPageStructure is the detected page structure; Secondary is nil when no sub-type applies.
type ExtractPageStructure struct {
	Primary   string  `json:"primary,omitempty"`
	Secondary *string `json:"secondary,omitempty"`
}

// ExtractCategory is the detected content category; Secondary is nil when no sub-category applies.
type ExtractCategory struct {
	Primary   string  `json:"primary,omitempty"`
	Secondary *string `json:"secondary,omitempty"`
}

// ExtractResult is the result for one URL. Status is "success" or "failed"; a failed item carries
// ErrorMessage and is not billed. ResolvedMode is the mode a successful URL is billed at.
// PageStructure and Category are nil when the server cannot determine them.
type ExtractResult struct {
	URL             string                 `json:"url,omitempty"`
	Status          string                 `json:"status,omitempty"`
	ResolvedMode    string                 `json:"resolved_mode,omitempty"`
	Title           *string                `json:"title,omitempty"`
	FullContent     *string                `json:"full_content,omitempty"`
	Highlights      []string               `json:"highlights,omitempty"`
	TimePublished   *string                `json:"time_published,omitempty"`
	TimeLastCrawled string                 `json:"time_last_crawled,omitempty"`
	PageStructure   *ExtractPageStructure  `json:"page_structure,omitempty"`
	Category        *ExtractCategory       `json:"category,omitempty"`
	Favicon         *string                `json:"favicon,omitempty"`
	CoverImage      *ExtractMediaResource  `json:"cover_image,omitempty"`
	Images          []ExtractMediaResource `json:"images,omitempty"`
	Videos          []ExtractMediaResource `json:"videos,omitempty"`
	Audio           []ExtractMediaResource `json:"audio,omitempty"`
	Links           []ExtractLink          `json:"links,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
}

// ExtractData is the data object of an Extract response.
type ExtractData struct {
	Results []ExtractResult `json:"results,omitempty"`
}

// ExtractSuccessfulByMode counts the successful URLs by the mode they are billed at.
type ExtractSuccessfulByMode struct {
	StandardURLs int `json:"standard_urls,omitempty"`
	AdvancedURLs int `json:"advanced_urls,omitempty"`
}

// ExtractUsage reports what an Extract call is billed for; failed URLs are not counted as successful.
type ExtractUsage struct {
	TotalURLs        int                      `json:"total_urls,omitempty"`
	SuccessfulURLs   int                      `json:"successful_urls,omitempty"`
	SuccessfulByMode *ExtractSuccessfulByMode `json:"successful_by_mode,omitempty"`
}

// ExtractMeta is the meta object of an Extract response.
type ExtractMeta struct {
	Usage   *ExtractUsage `json:"usage,omitempty"`
	Latency float64       `json:"latency,omitempty"`
	Warning *string       `json:"warning,omitempty"`
}

// ExtractResponse is the response of Extract.
type ExtractResponse struct {
	Code      json.RawMessage `json:"code,omitempty"`
	Msg       *string         `json:"msg,omitempty"`
	RequestID string          `json:"request_id"`
	Data      *ExtractData    `json:"data,omitempty"`
	Meta      *ExtractMeta    `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *ExtractResponse) RawJSON() []byte { return r.raw }
