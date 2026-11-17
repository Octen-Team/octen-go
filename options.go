package octen

import (
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// Option configures a Client; pass options to NewClient.
type Option func(*clientConfig) error

// RequestOption adjusts a single call; pass options after the params of a search method.
type RequestOption func(*requestConfig) error

type clientConfig struct {
	apiKey     string
	baseURL    string
	timeout    time.Duration
	timeoutSet bool
	maxRetries int
	headers    http.Header
	httpClient *http.Client
}

type requestConfig struct {
	timeout time.Duration
	headers http.Header
}

// WithAPIKey sets the API key. A blank key falls back to the OCTEN_API_KEY environment variable.
func WithAPIKey(key string) Option {
	return func(c *clientConfig) error {
		c.apiKey = key
		return nil
	}
}

// WithBaseURL sets the API origin, which defaults to https://api.octen.ai. It must be an
// absolute http or https URL without credentials, query or fragment; trailing slashes are dropped.
func WithBaseURL(baseURL string) Option {
	return func(c *clientConfig) error {
		u, err := checkBaseURL(baseURL)
		if err != nil {
			return err
		}
		c.baseURL = u
		return nil
	}
}

// WithTimeout sets the timeout of each attempt, including reading the response body.
// Once set it applies to every endpoint, BroadSearch included, which otherwise gets 120 s.
func WithTimeout(d time.Duration) Option {
	return func(c *clientConfig) error {
		if err := checkTimeout("WithTimeout", d); err != nil {
			return err
		}
		c.timeout, c.timeoutSet = d, true
		return nil
	}
}

// WithMaxRetries sets how many times a failed attempt is retried; the default is 3.
func WithMaxRetries(n int) Option {
	return func(c *clientConfig) error {
		if n < 0 {
			return &ValidationError{Field: "WithMaxRetries", Message: "maxRetries must be a non-negative integer."}
		}
		c.maxRetries = n
		return nil
	}
}

// WithHeader adds a header to every request. x-api-key is refused: use WithAPIKey.
func WithHeader(name, value string) Option {
	return func(c *clientConfig) error {
		if err := checkHeader("WithHeader", name, value); err != nil {
			return err
		}
		c.headers.Set(name, value)
		return nil
	}
}

// WithHTTPClient sends requests through hc. The SDK works on a copy of it, so hc itself
// is never modified, and its Timeout, if any, applies on top of the SDK timeout.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *clientConfig) error {
		if hc == nil {
			return &ValidationError{Field: "WithHTTPClient", Message: "http client must not be nil."}
		}
		c.httpClient = hc
		return nil
	}
}

// WithRequestTimeout overrides the timeout of each attempt of one call.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(r *requestConfig) error {
		if err := checkTimeout("WithRequestTimeout", d); err != nil {
			return err
		}
		r.timeout = d
		return nil
	}
}

// WithRequestHeader adds a header to one call, replacing a client header of the same name.
func WithRequestHeader(name, value string) RequestOption {
	return func(r *requestConfig) error {
		if err := checkHeader("WithRequestHeader", name, value); err != nil {
			return err
		}
		if r.headers == nil {
			r.headers = http.Header{}
		}
		r.headers.Set(name, value)
		return nil
	}
}

func checkTimeout(field string, d time.Duration) error {
	if d <= 0 {
		return &ValidationError{Field: field, Message: "timeout must be greater than 0."}
	}
	return nil
}

func checkBaseURL(raw string) (string, error) {
	bad := func(msg string) error { return &ValidationError{Field: "WithBaseURL", Message: msg} }
	if strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return "", bad("baseURL must not contain whitespace.")
	}
	if strings.Contains(raw, "?") {
		return "", bad("baseURL must not contain a query.")
	}
	if strings.Contains(raw, "#") {
		return "", bad("baseURL must not contain a fragment.")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", bad("baseURL must be an absolute http(s) URL.")
	}
	if scheme := strings.ToLower(u.Scheme); (scheme != "http" && scheme != "https") || u.Hostname() == "" {
		return "", bad("baseURL must be an absolute http(s) URL.")
	}
	if u.User != nil {
		return "", bad("baseURL must not contain credentials (user:password@); use WithAPIKey.")
	}
	return strings.TrimRight(raw, "/"), nil
}

func checkHeader(field, name, value string) error {
	if !isToken(name) {
		return &ValidationError{Field: field, Message: "header name " + quote(name) + " is not a valid token."}
	}
	if strings.EqualFold(name, "x-api-key") {
		return &ValidationError{Field: field, Message: "x-api-key cannot be set as a header; pass the key with WithAPIKey."}
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return &ValidationError{Field: field, Message: "value of header " + quote(name) + " must not contain CR, LF or NUL."}
	}
	return nil
}

// isToken reports whether s is an RFC 7230 token, the grammar of a header field name.
func isToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return true
}

func quote(s string) string {
	return "\"" + strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s) + "\""
}
