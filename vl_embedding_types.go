package octen

import (
	"encoding/json"
	"strconv"
)

const vlEmbeddingMaxContents = 20

// VLEmbeddingParams is the request body of VL Embedding. Model is octen-vl-embedding or
// octen-vl-embedding-large. EnableFusion merges all contents into one vector instead of one per item;
// FPS (0 to 1) sets how many video frames are sampled per second. The server checks image and video
// counts, Dimension and FPS.
type VLEmbeddingParams struct {
	Model        string           `json:"model" enum:"octen-vl-embedding,octen-vl-embedding-large"`
	Input        VLEmbeddingInput `json:"input"`
	EnableFusion *bool            `json:"enable_fusion,omitempty"`
	Dimension    *int             `json:"dimension,omitempty"`
	FPS          *float64         `json:"fps,omitempty"`
	Instruct     *string          `json:"instruct,omitempty"`
}

// VLEmbeddingInput holds 1 to 20 content items.
type VLEmbeddingInput struct {
	Contents []VLEmbeddingContent `json:"contents"`
}

// VLEmbeddingContent is one item to embed; set exactly one of Text, Image or Video (an image or
// video URL). TextContent, ImageContent and VideoContent build one.
type VLEmbeddingContent struct {
	Text  *string `json:"text,omitempty"`
	Image *string `json:"image,omitempty"`
	Video *string `json:"video,omitempty"`
}

// TextContent returns a content item holding text.
func TextContent(text string) VLEmbeddingContent { return VLEmbeddingContent{Text: &text} }

// ImageContent returns a content item holding an image URL.
func ImageContent(url string) VLEmbeddingContent { return VLEmbeddingContent{Image: &url} }

// VideoContent returns a content item holding a video URL.
func VideoContent(url string) VLEmbeddingContent { return VLEmbeddingContent{Video: &url} }

// validate checks only what the API reference requires: a model, and 1 to 20 items each with one value.
func (p VLEmbeddingParams) validate() error {
	if isBlank(&p.Model) {
		return &ValidationError{Field: "model", Message: "model is required and must be a non-empty string."}
	}
	contents := p.Input.Contents
	if len(contents) == 0 || len(contents) > vlEmbeddingMaxContents {
		return &ValidationError{Field: "input.contents", Message: "input.contents is required and must hold 1 to " +
			strconv.Itoa(vlEmbeddingMaxContents) + " items."}
	}
	for i, c := range contents {
		set, value := 0, (*string)(nil)
		for _, v := range []*string{c.Text, c.Image, c.Video} {
			if v != nil {
				set, value = set+1, v
			}
		}
		if set != 1 || isBlank(value) {
			field := "input.contents[" + strconv.Itoa(i) + "]"
			return &ValidationError{Field: field, Message: field + " must set exactly one of text, image or video to a non-empty string."}
		}
	}
	return nil
}

// VLEmbeddingResult is one vector. Type is "vl" for a per-item vector, whose Index is the item's
// position in Contents, or "fusion" for the single fused vector.
type VLEmbeddingResult struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding,omitempty"`
	Type      string    `json:"type,omitempty"`
}

// VLEmbeddingData is the data object of a VL Embedding response.
type VLEmbeddingData struct {
	Results []VLEmbeddingResult `json:"results,omitempty"`
	Model   string              `json:"model,omitempty"`
}

// VLEmbeddingUsage reports what a VL Embedding call is billed for; Duration is the video length in seconds.
type VLEmbeddingUsage struct {
	InputTokens int `json:"input_tokens,omitempty"`
	TextTokens  int `json:"text_tokens,omitempty"`
	ImageTokens int `json:"image_tokens,omitempty"`
	ImageCount  int `json:"image_count,omitempty"`
	Duration    int `json:"duration,omitempty"`
}

// VLEmbeddingMeta is the meta object of a VL Embedding response.
type VLEmbeddingMeta struct {
	Usage   *VLEmbeddingUsage `json:"usage,omitempty"`
	Warning *string           `json:"warning,omitempty"`
}

// VLEmbeddingResponse is the response of VL Embedding.
type VLEmbeddingResponse struct {
	Code      json.RawMessage  `json:"code,omitempty"`
	Msg       *string          `json:"msg,omitempty"`
	RequestID string           `json:"request_id"`
	Data      *VLEmbeddingData `json:"data,omitempty"`
	Meta      *VLEmbeddingMeta `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *VLEmbeddingResponse) RawJSON() []byte { return r.raw }
