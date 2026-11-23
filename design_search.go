package octen

import "context"

// DesignSearchService serves the design search endpoint; reach it as Client.DesignSearch.
type DesignSearchService struct {
	c *core
}

// Search finds design references (POST /design-search): screenshots of web pages, page
// sections, app screens and slides with their design values, plus web results on the same query.
func (s *DesignSearchService) Search(ctx context.Context, params DesignSearchParams, opts ...RequestOption) (*DesignSearchResponse, error) {
	resp, raw, err := call[DesignSearchResponse](ctx, s.c, "/design-search", params, defaultTimeout, opts)
	if err != nil {
		return nil, err
	}
	resp.raw = raw
	return resp, nil
}
