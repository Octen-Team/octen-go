package octen

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestRetryableStatuses(t *testing.T) {
	for _, s := range []int{408, 429, 500, 502, 503, 504} {
		if !isRetryableStatus(s) {
			t.Errorf("status %d should be retryable", s)
		}
	}
	for _, s := range []int{400, 401, 403, 404, 422, 501, 505} {
		if isRetryableStatus(s) {
			t.Errorf("status %d should not be retryable", s)
		}
	}
}

func TestBackoff(t *testing.T) {
	zero := func() float64 { return 0 }
	one := func() float64 { return 1 }
	cases := []struct {
		attempt int
		rnd     func() float64
		want    time.Duration
	}{
		{1, zero, 1 * time.Second},
		{2, zero, 2 * time.Second},
		{3, zero, 4 * time.Second},
		{7, zero, 60 * time.Second}, // 64s capped
		{1, one, 1250 * time.Millisecond},
		{20, one, 60 * time.Second}, // large attempts must not overflow past the cap
	}
	for _, c := range cases {
		d, giveUp := retryDelay(c.attempt, 500, 0, false, c.rnd)
		if giveUp || d != c.want {
			t.Errorf("attempt %d: got (%v, %v), want (%v, false)", c.attempt, d, giveUp, c.want)
		}
	}
}

func TestNetworkErrorUsesBackoff(t *testing.T) {
	// Status 0 means a network error: a stray Retry-After must not apply.
	d, giveUp := retryDelay(1, 0, 5*time.Second, true, func() float64 { return 0 })
	if giveUp || d != time.Second {
		t.Fatalf("got (%v, %v), want (1s, false)", d, giveUp)
	}
}

func TestRetryAfterHonouredOnlyFor429And503(t *testing.T) {
	zero := func() float64 { return 0 }
	cases := []struct {
		name       string
		status     int
		retryAfter time.Duration
		wantD      time.Duration
		wantGiveUp bool
	}{
		{"429 5s", 429, 5 * time.Second, 5 * time.Second, false},
		{"503 60s is waited", 503, 60 * time.Second, 60 * time.Second, false},
		{"503 61s gives up", 503, 61 * time.Second, 0, true},
		{"502 ignores Retry-After", 502, 5 * time.Second, time.Second, false},
		{"500 ignores a huge Retry-After", 500, 3600 * time.Second, time.Second, false},
	}
	for _, c := range cases {
		d, giveUp := retryDelay(1, c.status, c.retryAfter, true, zero)
		if giveUp != c.wantGiveUp || (!giveUp && d != c.wantD) {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", c.name, d, giveUp, c.wantD, c.wantGiveUp)
		}
	}
}

// The cap is strict: exactly 60s is waited, anything above gives up. The TS SDK shipped an off-by-one here.
func TestRetryAfterSixtySecondBoundary(t *testing.T) {
	zero := func() float64 { return 0 }
	d, giveUp := retryDelay(1, 429, 60*time.Second, true, zero)
	if giveUp || d != 60*time.Second {
		t.Errorf("60s: got (%v, %v), want (60s, false)", d, giveUp)
	}
	if _, giveUp := retryDelay(1, 429, 61*time.Second, true, zero); !giveUp {
		t.Error("61s should give up")
	}
	if _, giveUp := retryDelay(1, 429, 60*time.Second+time.Millisecond, true, zero); !giveUp {
		t.Error("60.001s should give up")
	}
}

func TestRetryAfterWithoutHeaderUsesBackoff(t *testing.T) {
	d, giveUp := retryDelay(2, 429, 0, false, func() float64 { return 0 })
	if giveUp || d != 2*time.Second {
		t.Fatalf("got (%v, %v), want (2s, false)", d, giveUp)
	}
}

func TestRetryAfterZeroMeansNow(t *testing.T) {
	d, giveUp := retryDelay(1, 429, 0, true, func() float64 { return 0 })
	if giveUp || d != 0 {
		t.Fatalf("got (%v, %v), want (0, false)", d, giveUp)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	ok := []struct {
		in   string
		want time.Duration
	}{
		{"5", 5 * time.Second},
		{" 5 ", 5 * time.Second},
		{"0", 0},
		{now.Add(3 * time.Second).Format(http.TimeFormat), 3 * time.Second},
		{now.Add(-time.Hour).Format(http.TimeFormat), 0},
		{"Tuesday, 17-Nov-26 00:00:07 GMT", 7 * time.Second}, // RFC 850
		{"Tue Nov 17 00:00:09 2026", 9 * time.Second},        // ANSI C
	}
	for _, c := range ok {
		got, valid := parseRetryAfter(c.in, now)
		if !valid || got != c.want {
			t.Errorf("%q: got (%v, %v), want (%v, true)", c.in, got, valid, c.want)
		}
	}
	for _, in := range []string{"5.5", "-1", "+5", "1e3", "3 4", "", "   ", "soon"} {
		if got, valid := parseRetryAfter(in, now); valid {
			t.Errorf("%q: got (%v, true), want invalid", in, got)
		}
	}
}

func TestParseRetryAfterHugeSecondsGivesUp(t *testing.T) {
	for _, in := range []string{"99999999999", "99999999999999999999999"} {
		d, valid := parseRetryAfter(in, time.Now())
		if !valid || d <= maxRetryDelay {
			t.Errorf("%q: got (%v, %v), want a valid delay above the cap", in, d, valid)
		}
	}
}

func TestPreSendNetworkError(t *testing.T) {
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	dns := &net.DNSError{Err: "no such host", Name: "example.invalid"}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"dial OpError", &url.Error{Op: "Post", URL: "https://x", Err: dial}, true},
		{"DNSError", &url.Error{Op: "Post", URL: "https://x", Err: dns}, true},
		{"read OpError", &url.Error{Op: "Post", URL: "https://x", Err: &net.OpError{Op: "read", Err: io.ErrUnexpectedEOF}}, false},
		{"unexpected EOF", io.ErrUnexpectedEOF, false},
		{"deadline exceeded", context.DeadlineExceeded, false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := isPreSendNetworkError(c.err); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
