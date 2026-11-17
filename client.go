package octen

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "https://api.octen.ai"
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
	apiKeyEnv         = "OCTEN_API_KEY"
)

// Client is the entry point of the SDK. It is safe for concurrent use.
type Client struct {
	// Search serves the web, broad, news, business and Pulse search endpoints.
	Search *SearchService
	// PlaceSearch serves the place search endpoint.
	PlaceSearch *PlaceSearchService
}

// NewClient builds a client from the options. Invalid configuration, or a missing API key,
// is reported as a *ValidationError; the key falls back to the OCTEN_API_KEY environment variable.
func NewClient(opts ...Option) (*Client, error) {
	cfg := &clientConfig{
		baseURL:    defaultBaseURL,
		timeout:    defaultTimeout,
		maxRetries: defaultMaxRetries,
		headers:    http.Header{},
	}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	key := strings.TrimSpace(cfg.apiKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv(apiKeyEnv))
	}
	if key == "" {
		return nil, &ValidationError{Field: "WithAPIKey", Message: "An API key is required: pass WithAPIKey or set the " + apiKeyEnv + " environment variable."}
	}
	if strings.ContainsAny(key, "\r\n\x00") {
		return nil, &ValidationError{Field: "WithAPIKey", Message: "API key contains characters that are not valid in an HTTP header."}
	}
	c := &core{
		transport:  newTransport(cfg.baseURL, key, cfg.headers, cfg.httpClient, cfg.maxRetries),
		timeout:    cfg.timeout,
		timeoutSet: cfg.timeoutSet,
	}
	return &Client{Search: &SearchService{c: c}, PlaceSearch: &PlaceSearchService{c: c}}, nil
}

// core is what the services share: the transport and the client-level timeout setting.
type core struct {
	transport  *transport
	timeout    time.Duration
	timeoutSet bool
}

// call validates params, posts them to path and decodes the response into R. The attempt
// timeout is the per-request one, else an explicit client one, else def.
func call[R any](ctx context.Context, c *core, path string, params any, def time.Duration, opts []RequestOption) (*R, []byte, error) {
	if err := validateParams(params); err != nil {
		return nil, nil, err
	}
	var rc requestConfig
	for _, opt := range opts {
		if err := opt(&rc); err != nil {
			return nil, nil, err
		}
	}
	body, err := encodeBody(params)
	if err != nil {
		return nil, nil, err
	}
	timeout := def
	switch {
	case rc.timeout > 0:
		timeout = rc.timeout
	case c.timeoutSet:
		timeout = c.timeout
	}

	res, err := c.transport.postResponse(ctx, path, body, timeout, rc.headers)
	if err != nil {
		return nil, nil, err
	}
	out := new(R)
	text := bytes.TrimPrefix(res.body, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal(text, out); err != nil {
		return nil, nil, decodeError(res, err)
	}
	return out, res.body, nil
}

// encodeBody marshals without HTML escaping so the body reads like the params it came from.
func encodeBody(params any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(params); err != nil {
		return nil, &ValidationError{Field: "params", Message: "params cannot be encoded as JSON: " + err.Error()}
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// decodeError reports a 2xx body that was judged successful but does not fit the typed response.
func decodeError(res *attemptResult, cause error) *APIError {
	e := &APIError{StatusCode: res.status, Message: "decode response: " + cause.Error(), Body: res.body}
	if parsed, ok := decodeJSON(bytes.TrimPrefix(res.body, []byte("\xef\xbb\xbf"))); ok {
		e.Code = extractCode(parsed)
		e.RequestID = extractRequestID(res.header, parsed)
	} else {
		e.RequestID = res.header.Get("x-request-id")
	}
	return e
}
