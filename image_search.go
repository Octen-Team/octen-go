package octen

import "context"

// ImageSearchService serves the image search endpoint; reach it as Client.ImageSearch.
type ImageSearchService struct {
	c *core
}

// Search searches for images from one text query or one image URL (POST /image-search).
func (s *ImageSearchService) Search(ctx context.Context, params ImageSearchParams, opts ...RequestOption) (*ImageSearchResponse, error) {
	resp, raw, err := call[ImageSearchResponse](ctx, s.c, "/image-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}
