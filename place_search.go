package octen

import "context"

// PlaceSearchService serves the place search endpoint; reach it as Client.PlaceSearch.
type PlaceSearchService struct {
	c *core
}

// Search runs a place search (POST /place-search).
func (s *PlaceSearchService) Search(ctx context.Context, params PlaceSearchParams, opts ...RequestOption) (*PlaceSearchResponse, error) {
	resp, raw, err := call[PlaceSearchResponse](ctx, s.c, "/place-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}
