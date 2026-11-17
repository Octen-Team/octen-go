package octen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestErrorsAsAndUnwrap(t *testing.T) {
	apiErr, _ := judgeResponse(401, http.Header{}, []byte(`{"msg":"invalid api key","code":401}`))
	wrapped := fmt.Errorf("search: %w", apiErr)
	var got *APIError
	if !errors.As(wrapped, &got) || got.StatusCode != 401 {
		t.Fatalf("errors.As failed: %v", wrapped)
	}
	if !errors.Is(wrapped, ErrAuthentication) || errors.Is(wrapped, ErrServer) {
		t.Fatal("sentinel mapping wrong")
	}
	if got.Error() != "octen: 401 invalid api key" {
		t.Fatalf("Error() = %q", got.Error())
	}

	inner := io.ErrUnexpectedEOF
	te := &TimeoutError{Timeout: 30 * time.Second, Err: inner}
	if !errors.Is(fmt.Errorf("x: %w", te), inner) {
		t.Fatal("TimeoutError must unwrap")
	}
	if te.Error() != "octen: request timed out after 30s" {
		t.Fatalf("Error() = %q", te.Error())
	}
	ce := &ConnectionError{Err: inner}
	if !errors.Is(fmt.Errorf("x: %w", ce), inner) {
		t.Fatal("ConnectionError must unwrap")
	}
	if ce.Error() != "octen: connection error: "+inner.Error() {
		t.Fatalf("Error() = %q", ce.Error())
	}
	ve := &ValidationError{Field: "query", Message: "is required"}
	if ve.Error() != "octen: invalid query: is required" {
		t.Fatalf("Error() = %q", ve.Error())
	}
	var vErr *ValidationError
	if !errors.As(fmt.Errorf("x: %w", ve), &vErr) {
		t.Fatal("errors.As ValidationError failed")
	}
}

func TestErrorStringsNeverContainKey(t *testing.T) {
	const key = "sk-test-secret-key"
	h := http.Header{"X-Api-Key": {key}}
	apiErr, _ := judgeResponse(500, h, []byte("boom"))
	for _, s := range []string{
		apiErr.Error(),
		(&TimeoutError{Timeout: time.Second, Err: io.EOF}).Error(),
		(&ConnectionError{Err: io.EOF}).Error(),
		(&ValidationError{Field: "f", Message: "m"}).Error(),
	} {
		if strings.Contains(s, key) {
			t.Fatalf("%q leaks the key", s)
		}
	}
}

func TestAPIErrorIsMapping(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{401, ErrAuthentication}, {403, ErrPermissionDenied}, {400, ErrBadRequest},
		{422, ErrBadRequest}, {429, ErrRateLimit}, {500, ErrServer}, {599, ErrServer},
	}
	for _, c := range cases {
		if !errors.Is(&APIError{StatusCode: c.status}, c.want) {
			t.Errorf("status %d should match %v", c.status, c.want)
		}
	}
	for _, s := range []int{200, 302, 404, 408} {
		e := &APIError{StatusCode: s}
		for _, sent := range []error{ErrAuthentication, ErrPermissionDenied, ErrBadRequest, ErrRateLimit, ErrServer} {
			if errors.Is(e, sent) {
				t.Errorf("status %d must match no sentinel", s)
			}
		}
	}
}

// The DeadlineExceeded match must hold even when Err does not carry it.
func TestTimeoutErrorMatchesDeadlineExceeded(t *testing.T) {
	te := &TimeoutError{Timeout: time.Second, Err: io.EOF}
	if !errors.Is(fmt.Errorf("x: %w", te), context.DeadlineExceeded) {
		t.Fatal("TimeoutError must match context.DeadlineExceeded")
	}
	if errors.Is(te, context.Canceled) {
		t.Fatal("TimeoutError must not match context.Canceled")
	}
	if !errors.Is(te, io.EOF) {
		t.Fatal("TimeoutError must still unwrap to Err")
	}
}
