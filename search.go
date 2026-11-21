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

// SearchPremier runs a Web Search Premier search (POST /search-premier), which retrieves and
// reranks more deeply than Search. It takes the same params and returns the same response as Search.
func (s *SearchService) SearchPremier(ctx context.Context, params SearchParams, opts ...RequestOption) (*SearchResponse, error) {
	resp, raw, err := call[SearchResponse](ctx, s.c, "/search-premier", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}

// ImageSearch searches for images from one text query or one image URL (POST /image-search).
func (s *SearchService) ImageSearch(ctx context.Context, params ImageSearchParams, opts ...RequestOption) (*ImageSearchResponse, error) {
	resp, raw, err := call[ImageSearchResponse](ctx, s.c, "/image-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}

// VideoSearch searches for videos from one text query (POST /video-search). Video Search is an
// invite-only beta: contact Octen to request access; without it the server rejects the call.
func (s *SearchService) VideoSearch(ctx context.Context, params VideoSearchParams, opts ...RequestOption) (*VideoSearchResponse, error) {
	resp, raw, err := call[VideoSearchResponse](ctx, s.c, "/video-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}

// DesignSearch finds design references (POST /design-search): screenshots of web pages, page
// sections, app screens and slides with their design values, plus web results on the same query.
func (s *SearchService) DesignSearch(ctx context.Context, params DesignSearchParams, opts ...RequestOption) (*DesignSearchResponse, error) {
	resp, raw, err := call[DesignSearchResponse](ctx, s.c, "/design-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}
