package octen

import (
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetryDelay  = 60 * time.Second
	baseRetryDelay = time.Second
)

func isRetryableStatus(status int) bool {
	switch status {
	case 408, 429, 500, 502, 503, 504:
		return true
	}
	return false
}

// parseRetryAfter accepts non-negative integer seconds or an HTTP date; a past date yields 0.
// Anything else is rejected so the caller falls back to backoff.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if isASCIIDigits(v) {
		secs, err := strconv.ParseUint(v, 10, 63)
		// Overflow still means "wait too long", which the caller turns into giving up.
		if err != nil || secs > uint64(math.MaxInt64/int64(time.Second)) {
			return time.Duration(math.MaxInt64), true
		}
		return time.Duration(secs) * time.Second, true
	}
	at, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	if d := at.Sub(now); d > 0 {
		return d, true
	}
	return 0, true
}

func isASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// retryDelay: attempt starts at 1 and status 0 means a network error. Retry-After is only
// honoured for 429 and 503, and waiting exactly 60s is allowed but longer gives up.
func retryDelay(attempt, status int, retryAfter time.Duration, hasRetryAfter bool, rnd func() float64) (d time.Duration, giveUp bool) {
	if hasRetryAfter && (status == 429 || status == 503) {
		if retryAfter > maxRetryDelay {
			return 0, true
		}
		return retryAfter, false
	}
	if attempt < 1 {
		attempt = 1
	}
	// Float math keeps large attempt numbers from overflowing before the cap applies.
	backoff := float64(baseRetryDelay) * math.Pow(2, float64(attempt-1)) * (1 + 0.25*rnd())
	if backoff >= float64(maxRetryDelay) {
		return maxRetryDelay, false
	}
	return time.Duration(backoff), false
}

// isPreSendNetworkError is true only for failures before the request reached the server
// (dial or DNS), the only network errors that are safe to retry for non-idempotent calls.
func isPreSendNetworkError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}
