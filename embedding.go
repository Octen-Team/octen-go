package octen

import "context"

// EmbeddingService serves the Embedding endpoint; reach it as Client.Embedding.
type EmbeddingService struct {
	c *core
}

// Create returns one vector per input text (POST /embedding); match results to your input by Index.
func (s *EmbeddingService) Create(ctx context.Context, params EmbeddingParams, opts ...RequestOption) (*EmbeddingResponse, error) {
	resp, raw, err := call[EmbeddingResponse](ctx, s.c, "/embedding", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}
