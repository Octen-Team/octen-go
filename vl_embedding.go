package octen

import "context"

// VLEmbeddingService serves the VL Embedding endpoint; reach it as Client.VLEmbedding.
type VLEmbeddingService struct {
	c *core
}

// Create embeds text, image and video items in one vector space (POST /vl-embedding): one vector per
// item, or a single fused vector when EnableFusion is set.
func (s *VLEmbeddingService) Create(ctx context.Context, params VLEmbeddingParams, opts ...RequestOption) (*VLEmbeddingResponse, error) {
	resp, raw, err := call[VLEmbeddingResponse](ctx, s.c, "/vl-embedding", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}
