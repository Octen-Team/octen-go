package octen

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// errSDKTimeout is the cause of an attempt context expired by the SDK timeout, telling it
// apart from the caller's deadline. Newer net/http returns the cause itself, so it still
// matches context.DeadlineExceeded like on older Go versions.
var errSDKTimeout error = sdkTimeout{}

type sdkTimeout struct{}

func (sdkTimeout) Error() string        { return "octen: sdk timeout" }
func (sdkTimeout) Timeout() bool        { return true }
func (sdkTimeout) Is(target error) bool { return target == context.DeadlineExceeded }

type transport struct {
	baseURL    string
	apiKey     string
	headers    http.Header
	client     *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
	rnd        func() float64
}

// newTransport works on a copy of client so the caller's CheckRedirect stays untouched;
// its Timeout and Transport are kept as they are.
func newTransport(baseURL, apiKey string, headers http.Header, client *http.Client, maxRetries int) *transport {
	c := &http.Client{}
	if client != nil {
		copied := *client
		c = &copied
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &transport{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		headers:    headers,
		client:     c,
		maxRetries: maxRetries,
	}
}

// attemptResult is either a response (err == nil) or a classified error.
type attemptResult struct {
	status    int
	header    http.Header
	body      []byte
	err       error
	retryable bool
}

// post sends body as JSON and returns the raw response body of a successful envelope.
func (t *transport) post(ctx context.Context, path string, body []byte, timeout time.Duration, extra http.Header) ([]byte, error) {
	url := t.baseURL + "/" + strings.TrimLeft(path, "/")
	header := t.buildHeaders(extra)

	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("octen: %w", err)
		}
		res := t.attempt(ctx, url, header, body, timeout)

		var lastErr error
		retryable := res.retryable
		status := 0
		if res.err != nil {
			lastErr = res.err
		} else {
			apiErr, _ := judgeResponse(res.status, res.header, res.body)
			if apiErr == nil {
				return res.body, nil
			}
			if res.status >= 300 && res.status < 400 {
				return nil, redirectError(apiErr, res.header)
			}
			lastErr = apiErr
			status = res.status
			retryable = isRetryableStatus(res.status)
		}
		if !retryable || attempt > t.maxRetries {
			return nil, lastErr
		}

		var retryAfter time.Duration
		hasRetryAfter := false
		if status != 0 {
			retryAfter, hasRetryAfter = parseRetryAfter(res.header.Get("Retry-After"), time.Now())
		}
		delay, giveUp := retryDelay(attempt, status, retryAfter, hasRetryAfter, t.random)
		if giveUp {
			return nil, lastErr
		}
		if err := t.wait(ctx, delay); err != nil {
			return nil, err
		}
	}
}

// attempt runs one request; the SDK timeout stays armed until the body has been read.
func (t *transport) attempt(ctx context.Context, url string, header http.Header, body []byte, timeout time.Duration) attemptResult {
	attemptCtx, cancel := context.WithTimeoutCause(ctx, timeout, errSDKTimeout)
	defer cancel()

	// bytes.NewReader sets GetBody, so net/http may replay only when nothing was written.
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return attemptResult{err: &ConnectionError{Err: err}}
	}
	req.Header = header.Clone()

	resp, err := t.client.Do(req)
	if err != nil {
		return classify(ctx, attemptCtx, timeout, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return classify(ctx, attemptCtx, timeout, err)
	}
	return attemptResult{status: resp.StatusCode, header: resp.Header, body: data}
}

// classify checks the caller's context first: a dial error caused by its cancellation
// would otherwise look like a retryable pre-send failure.
func classify(ctx, attemptCtx context.Context, timeout time.Duration, err error) attemptResult {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return attemptResult{err: fmt.Errorf("octen: %w", ctxErr)}
	}
	if context.Cause(attemptCtx) == errSDKTimeout {
		return attemptResult{err: &TimeoutError{Timeout: timeout, Err: err}}
	}
	return attemptResult{err: &ConnectionError{Err: err}, retryable: isPreSendNetworkError(err)}
}

func (t *transport) buildHeaders(extra http.Header) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json")
	h.Set("User-Agent", "octen-go-sdk/"+Version)
	for _, src := range []http.Header{t.headers, extra} {
		for k, vs := range src {
			h[http.CanonicalHeaderKey(k)] = append([]string(nil), vs...)
		}
	}
	// Written last so no default or per-call header can replace the key.
	h.Set("X-Api-Key", t.apiKey)
	return h
}

func (t *transport) random() float64 {
	if t.rnd != nil {
		return t.rnd()
	}
	return rand.Float64()
}

// wait sleeps for the backoff and reports the caller's cancellation if it happened meanwhile.
func (t *transport) wait(ctx context.Context, d time.Duration) error {
	sleep := t.sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	err := sleep(ctx, d)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("octen: %w", ctxErr)
	}
	if err != nil {
		return fmt.Errorf("octen: %w", err)
	}
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// redirectError keeps the response details but explains why the redirect was not followed.
func redirectError(e *APIError, h http.Header) *APIError {
	msg := "HTTP " + strconv.Itoa(e.StatusCode) + " redirect"
	if loc := h.Get("Location"); loc != "" {
		msg += " to " + loc
	}
	return &APIError{
		StatusCode: e.StatusCode,
		Code:       e.Code,
		Message:    msg + "; redirects are not followed, check baseURL",
		RequestID:  e.RequestID,
		Body:       e.Body,
	}
}
