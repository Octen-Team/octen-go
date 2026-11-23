package octen

import "context"

// VideoSearchService serves the video search endpoint; reach it as Client.VideoSearch.
type VideoSearchService struct {
	c *core
}

// Search searches for videos from one text query (POST /video-search). Video Search is an
// invite-only beta: contact Octen to request access; without it the server rejects the call.
func (s *VideoSearchService) Search(ctx context.Context, params VideoSearchParams, opts ...RequestOption) (*VideoSearchResponse, error) {
	resp, raw, err := call[VideoSearchResponse](ctx, s.c, "/video-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}
