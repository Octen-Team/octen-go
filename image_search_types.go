package octen

import "encoding/json"

// ImageSearchInput is the input of an image search: a text query (Type "text" with Data and no
// URL), or an image (Type "image") given as a public URL or as base64-encoded Data, exactly one of the two.
type ImageSearchInput struct {
	Type string  `json:"type" enum:"text,image"`
	URL  *string `json:"url,omitempty"`
	Data *string `json:"data,omitempty"`
}

// ImageSearchParams is the request body of ImageSearch. Inputs must hold exactly one input.
type ImageSearchParams struct {
	Inputs         []ImageSearchInput `json:"inputs"`
	Count          *int               `json:"count,omitempty"`
	IncludeDomains []string           `json:"include_domains,omitempty"`
	ExcludeDomains []string           `json:"exclude_domains,omitempty"`
	Safesearch     *string            `json:"safesearch,omitempty" enum:"off,strict"`
}

func (p ImageSearchParams) validate() error {
	if len(p.Inputs) != 1 {
		return &ValidationError{Field: "inputs", Message: "inputs must hold exactly one input: one text input or one image input."}
	}
	switch in := p.Inputs[0]; in.Type {
	case "text":
		if isBlank(in.Data) {
			return &ValidationError{Field: "inputs[0].data", Message: "a text input needs a non-empty data string."}
		}
		if in.URL != nil {
			return &ValidationError{Field: "inputs[0].url", Message: "a text input takes data only; url is for image inputs."}
		}
	case "image":
		if (in.URL != nil) == (in.Data != nil) || (isBlank(in.URL) && isBlank(in.Data)) {
			return &ValidationError{Field: "inputs[0]", Message: "an image input needs exactly one of url (a public URL) or data (base64 image data), and it must be non-empty."}
		}
	default:
		return &ValidationError{Field: "inputs[0].type", Message: `type must be "text" or "image".`}
	}
	return nil
}

// ImageSearchResult is one image result.
type ImageSearchResult struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	SourcePage  string `json:"source_page,omitempty"`
	Description string `json:"description,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
}

// ImageSearchData is the data object of an image search response.
type ImageSearchData struct {
	Results []ImageSearchResult `json:"results,omitempty"`
}

// ImageSearchUsage reports the billable work of an image search call.
type ImageSearchUsage struct {
	NumSearchQueries int `json:"num_search_queries,omitempty"`
}

// ImageSearchMeta is the meta object of an image search response.
type ImageSearchMeta struct {
	Usage   *ImageSearchUsage `json:"usage,omitempty"`
	Latency float64           `json:"latency,omitempty"`
	Warning *string           `json:"warning,omitempty"`
}

// ImageSearchResponse is the response of ImageSearch.
type ImageSearchResponse struct {
	Code      json.RawMessage  `json:"code,omitempty"`
	Msg       *string          `json:"msg,omitempty"`
	RequestID string           `json:"request_id"`
	Data      *ImageSearchData `json:"data,omitempty"`
	Meta      *ImageSearchMeta `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *ImageSearchResponse) RawJSON() []byte { return r.raw }
