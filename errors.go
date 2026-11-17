package octen

import (
	"errors"
	"fmt"
	"time"
)

// Sentinels for errors.Is against an *APIError; see APIError.Is for the status mapping.
var (
	ErrAuthentication   = errors.New("octen: authentication failed")
	ErrPermissionDenied = errors.New("octen: permission denied")
	ErrBadRequest       = errors.New("octen: bad request")
	ErrRateLimit        = errors.New("octen: rate limited")
	ErrServer           = errors.New("octen: server error")
)

// APIError is returned when the API answers with a non-2xx status, or with a 2xx
// whose body is not a successful envelope.
type APIError struct {
	StatusCode int
	Code       *float64
	Message    string
	RequestID  string
	Body       []byte
	// RetryAfter is set only for 429 responses carrying a valid Retry-After header.
	RetryAfter *time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("octen: %d %s", e.StatusCode, e.Message)
}

// Is maps 401, 403, 400/422, 429 and >=500 to the matching sentinel.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrAuthentication:
		return e.StatusCode == 401
	case ErrPermissionDenied:
		return e.StatusCode == 403
	case ErrBadRequest:
		return e.StatusCode == 400 || e.StatusCode == 422
	case ErrRateLimit:
		return e.StatusCode == 429
	case ErrServer:
		return e.StatusCode >= 500
	}
	return false
}

// TimeoutError reports that one attempt exceeded its timeout.
//
// It also matches context.DeadlineExceeded, so test it with errors.As and a *TimeoutError
// target before looking for your own context's deadline.
type TimeoutError struct {
	Timeout time.Duration
	Err     error
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("octen: request timed out after %s", e.Timeout)
}

func (e *TimeoutError) Unwrap() error { return e.Err }

// ConnectionError wraps a network failure that is not a timeout or cancellation.
type ConnectionError struct {
	Err error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("octen: connection error: %v", e.Err)
}

func (e *ConnectionError) Unwrap() error { return e.Err }

// ValidationError reports invalid input rejected before any request is sent.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("octen: invalid %s: %s", e.Field, e.Message)
}
