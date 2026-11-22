package octen

import (
	"context"
	"time"
)

const (
	extractPerURLDefault = 30
	extractPerURLMin     = 1
	extractPerURLMax     = 60
	extractHeadroom      = 90 * time.Second
)

// ExtractService serves the Extract endpoint; reach it as Client.Extract.
type ExtractService struct {
	c *core
}

// Extract returns the content of 1 to 20 URLs (POST /extract), one result per URL; match results
// to your input by URL. Its default timeout is the per-URL Timeout, clamped to 1-60 s (30 s when
// unset), plus 90 s, unless the client or the call sets one; a shorter deadline on ctx still wins.
func (s *ExtractService) Extract(ctx context.Context, params ExtractParams, opts ...RequestOption) (*ExtractResponse, error) {
	resp, raw, err := call[ExtractResponse](ctx, s.c, "/extract", params, extractTimeout(params.Timeout), opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}

// extractTimeout clamps the per-URL timeout as the server does and adds headroom for a whole batch.
func extractTimeout(perURL *int) time.Duration {
	secs := extractPerURLDefault
	if perURL != nil {
		secs = min(max(*perURL, extractPerURLMin), extractPerURLMax)
	}
	return time.Duration(secs)*time.Second + extractHeadroom
}
