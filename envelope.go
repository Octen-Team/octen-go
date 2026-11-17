package octen

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
	"unicode/utf16"
)

const messageRawLimit = 500

var messageKeys = [...]string{"msg", "message", "detail", "error"}

// findMessage tries msg, message, detail, error in order and recurses into an error object.
func findMessage(obj map[string]any) (string, bool) {
	for _, key := range messageKeys {
		switch v := obj[key].(type) {
		case string:
			if v != "" {
				return v, true
			}
		case map[string]any:
			if key == "error" {
				if s, ok := findMessage(v); ok {
					return s, true
				}
			}
		}
	}
	return "", false
}

// truncateUTF16 keeps the first n UTF-16 code units, like JS String.slice. A pair split by
// the cut leaves a lone surrogate, which a Go string can only hold as U+FFFD.
func truncateUTF16(s string, n int) string {
	units := utf16.Encode([]rune(s))
	if len(units) <= n {
		return s
	}
	return string(utf16.Decode(units[:n]))
}

func extractMessage(parsed any, text string, status int) string {
	if obj, ok := parsed.(map[string]any); ok {
		if s, found := findMessage(obj); found {
			return s
		}
	}
	if raw := truncateUTF16(text, messageRawLimit); raw != "" {
		return raw
	}
	return "HTTP " + strconv.Itoa(status)
}

// numberValue reports whether v is a JSON number, and its value when finite.
// A number outside float64 range (1e999) counts as a number but has no finite value.
func numberValue(v any) (f float64, isNumber, finite bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false, false
	}
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, true, false
	}
	return f, true, true
}

func extractCode(parsed any) *float64 {
	obj, ok := parsed.(map[string]any)
	if !ok {
		return nil
	}
	if f, _, finite := numberValue(obj["code"]); finite {
		return &f
	}
	return nil
}

func extractRequestID(h http.Header, parsed any) string {
	if v := h.Get("x-request-id"); v != "" {
		return v
	}
	if obj, ok := parsed.(map[string]any); ok {
		if s, ok := obj["request_id"].(string); ok {
			return s
		}
	}
	return ""
}

// retryAfterRounded parses Retry-After and rounds up to whole seconds, saturating on overflow.
func retryAfterRounded(h http.Header, now time.Time) *time.Duration {
	d, ok := parseRetryAfter(h.Get("Retry-After"), now)
	if !ok {
		return nil
	}
	if rem := d % time.Second; rem != 0 && d <= math.MaxInt64-(time.Second-rem) {
		d += time.Second - rem
	}
	return &d
}

// stripBOM drops a leading UTF-8 BOM, which fetch's text() also ignores.
func stripBOM(b []byte) []byte { return bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")) }

func decodeJSON(body []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return v, true
}

// judgeResponse returns (nil, object) for a successful envelope and an *APIError otherwise.
// A 2xx is a success only if the body is a non-empty JSON object whose code is not a non-zero number.
func judgeResponse(status int, h http.Header, body []byte) (*APIError, map[string]any) {
	text := stripBOM(body)
	parsed, _ := decodeJSON(text)
	obj, isObj := parsed.(map[string]any)

	if status >= 200 && status < 300 && isObj && len(obj) > 0 {
		f, isNumber, finite := numberValue(obj["code"])
		if !isNumber || (finite && f == 0) {
			return nil, obj
		}
	}

	e := &APIError{
		StatusCode: status,
		Code:       extractCode(parsed),
		Message:    extractMessage(parsed, string(text), status),
		RequestID:  extractRequestID(h, parsed),
		Body:       body,
	}
	if status == 429 {
		e.RetryAfter = retryAfterRounded(h, time.Now())
	}
	return e, nil
}
