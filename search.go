package octen

import (
	"context"
	"time"
)

// broadSearchTimeout is longer because BroadSearch fans out into several searches.
const broadSearchTimeout = 120 * time.Second

// SearchService groups the search endpoints; reach it as Client.Search.
type SearchService struct {
	c *core
}

// Search runs a web search (POST /search).
func (s *SearchService) Search(ctx context.Context, params SearchParams, opts ...RequestOption) (*SearchResponse, error) {
	resp, raw, err := call[SearchResponse](ctx, s.c, "/search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}

// BroadSearch expands the query into several searches (POST /broad-search). Its default
// timeout is 120 s unless the client or the call sets one.
func (s *SearchService) BroadSearch(ctx context.Context, params BroadSearchParams, opts ...RequestOption) (*BroadSearchResponse, error) {
	resp, raw, err := call[BroadSearchResponse](ctx, s.c, "/broad-search", params, broadSearchTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}

// NewsSearch runs a news search (POST /news-search).
func (s *SearchService) NewsSearch(ctx context.Context, params NewsSearchParams, opts ...RequestOption) (*NewsSearchResponse, error) {
	resp, raw, err := call[NewsSearchResponse](ctx, s.c, "/news-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}

// BusinessSearch runs a business search (POST /business-search).
func (s *SearchService) BusinessSearch(ctx context.Context, params BusinessSearchParams, opts ...RequestOption) (*BusinessSearchResponse, error) {
	resp, raw, err := call[BusinessSearchResponse](ctx, s.c, "/business-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}

// PulseSearch runs a Pulse search (POST /pulse-search).
func (s *SearchService) PulseSearch(ctx context.Context, params PulseSearchParams, opts ...RequestOption) (*PulseSearchResponse, error) {
	resp, raw, err := call[PulseSearchResponse](ctx, s.c, "/pulse-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}
