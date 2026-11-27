package octen

import (
	"encoding/json"
	"strconv"
)

const embeddingMaxInputs = 1000

// EmbeddingParams is the request body of Embedding. Input must hold 1 to 1000 strings, none blank.
// Model is octen-embedding-0.6b, octen-embedding-4b (the server default) or octen-embedding-8b;
// Dimension defaults to the model's maximum. InputType "query" adds a retrieval prompt; "document"
// or unset adds none.
type EmbeddingParams struct {
	Input     []string `json:"input"`
	Model     *string  `json:"model,omitempty" enum:"octen-embedding-0.6b,octen-embedding-4b,octen-embedding-8b"`
	Dimension *int     `json:"dimension,omitempty"`
	InputType *string  `json:"input_type,omitempty" enum:"query,document"`
}

// validate rejects a blank item locally because the server accepts it and bills it.
func (p EmbeddingParams) validate() error {
	if len(p.Input) == 0 || len(p.Input) > embeddingMaxInputs {
		return &ValidationError{Field: "input", Message: "input is required and must hold 1 to " + strconv.Itoa(embeddingMaxInputs) + " strings."}
	}
	for i, s := range p.Input {
		if isBlank(&s) {
			field := "input[" + strconv.Itoa(i) + "]"
			return &ValidationError{Field: field, Message: field + " must be a non-empty string."}
		}
	}
	return nil
}

// EmbeddingResult is the vector for one input; Index is its position in Input.
type EmbeddingResult struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding,omitempty"`
}

// EmbeddingData is the data object of an Embedding response.
type EmbeddingData struct {
	Results []EmbeddingResult `json:"results,omitempty"`
	Model   string            `json:"model,omitempty"`
}

// EmbeddingUsage reports the input tokens an Embedding call is billed for.
type EmbeddingUsage struct {
	InputTokens int `json:"input_tokens,omitempty"`
}

// EmbeddingMeta is the meta object of an Embedding response. Warning says when the server cut
// an input longer than 10,000 characters.
type EmbeddingMeta struct {
	Usage   *EmbeddingUsage `json:"usage,omitempty"`
	Latency float64         `json:"latency,omitempty"`
	Warning *string         `json:"warning,omitempty"`
}

// EmbeddingResponse is the response of Embedding.
type EmbeddingResponse struct {
	Code      json.RawMessage `json:"code,omitempty"`
	Msg       *string         `json:"msg,omitempty"`
	RequestID string          `json:"request_id"`
	Data      *EmbeddingData  `json:"data,omitempty"`
	Meta      *EmbeddingMeta  `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *EmbeddingResponse) RawJSON() []byte { return r.raw }
