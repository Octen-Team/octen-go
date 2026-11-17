package octen

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type sharedCase struct {
	Name    string            `json:"name"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	// A key absent from expect means "not asserted"; JSON null means "assert nil/empty".
	Expect map[string]json.RawMessage `json:"expect"`
}

var classSentinels = map[string]error{
	"OctenAuthenticationError":   ErrAuthentication,
	"OctenPermissionDeniedError": ErrPermissionDenied,
	"OctenBadRequestError":       ErrBadRequest,
	"OctenRateLimitError":        ErrRateLimit,
	"OctenServerError":           ErrServer,
}

func isNull(raw json.RawMessage) bool { return string(raw) == "null" }

func TestSharedErrorCases(t *testing.T) {
	data, err := os.ReadFile("testdata/error_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []sharedCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 48 {
		t.Fatalf("expected 48 shared cases, got %d", len(cases))
	}
	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range c.Headers {
				h.Set(k, v)
			}
			apiErr, obj := judgeResponse(c.Status, h, []byte(c.Body))
			var ok bool
			if err := json.Unmarshal(c.Expect["ok"], &ok); err != nil {
				t.Fatalf("bad expect.ok: %v", err)
			}
			if ok {
				if apiErr != nil {
					t.Fatalf("expected success, got %v", apiErr)
				}
				if len(obj) == 0 {
					t.Fatal("expected a non-empty parsed object")
				}
				return
			}
			if apiErr == nil {
				t.Fatal("expected failure, got success")
			}
			if apiErr.StatusCode != c.Status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, c.Status)
			}
			if string(apiErr.Body) != c.Body {
				t.Errorf("Body not the raw response bytes")
			}
			var class string
			if err := json.Unmarshal(c.Expect["class"], &class); err != nil {
				t.Fatalf("bad expect.class: %v", err)
			}
			if class == "OctenAPIError" {
				for name, s := range classSentinels {
					if errors.Is(apiErr, s) {
						t.Errorf("OctenAPIError case must not match %s", name)
					}
				}
			} else {
				want, known := classSentinels[class]
				if !known {
					t.Fatalf("unknown class %q", class)
				}
				if !errors.Is(apiErr, want) {
					t.Errorf("errors.Is(%s) = false", class)
				}
				for name, s := range classSentinels {
					if s != want && errors.Is(apiErr, s) {
						t.Errorf("%s case also matches %s", class, name)
					}
				}
			}
			if raw, present := c.Expect["code"]; present {
				if isNull(raw) {
					if apiErr.Code != nil {
						t.Errorf("Code = %v, want nil", *apiErr.Code)
					}
				} else {
					var want float64
					if err := json.Unmarshal(raw, &want); err != nil {
						t.Fatal(err)
					}
					if apiErr.Code == nil || *apiErr.Code != want {
						t.Errorf("Code = %v, want %v", apiErr.Code, want)
					}
				}
			}
			if raw, present := c.Expect["message"]; present {
				var want string
				if err := json.Unmarshal(raw, &want); err != nil {
					t.Fatal(err)
				}
				if apiErr.Message != want {
					t.Errorf("Message = %q, want %q", apiErr.Message, want)
				}
			}
			if raw, present := c.Expect["requestId"]; present {
				var want string
				if !isNull(raw) {
					if err := json.Unmarshal(raw, &want); err != nil {
						t.Fatal(err)
					}
				}
				if apiErr.RequestID != want {
					t.Errorf("RequestID = %q, want %q", apiErr.RequestID, want)
				}
			}
			if raw, present := c.Expect["retryAfter"]; present {
				if isNull(raw) {
					if apiErr.RetryAfter != nil {
						t.Errorf("RetryAfter = %v, want nil", *apiErr.RetryAfter)
					}
				} else {
					var secs float64
					if err := json.Unmarshal(raw, &secs); err != nil {
						t.Fatal(err)
					}
					want := time.Duration(secs) * time.Second
					if apiErr.RetryAfter == nil || *apiErr.RetryAfter != want {
						t.Errorf("RetryAfter = %v, want %v", apiErr.RetryAfter, want)
					}
				}
			} else if c.Status != 429 && apiErr.RetryAfter != nil {
				t.Errorf("RetryAfter must only be set for 429")
			}
		})
	}
}

// Behaviour beyond the shared cases, mirroring envelope.ts semantics.
func TestJudgeResponseEdges(t *testing.T) {
	t.Run("tiny exponent code underflows to zero and succeeds", func(t *testing.T) {
		if e, _ := judgeResponse(200, nil, []byte(`{"code":1e-999}`)); e != nil {
			t.Fatalf("got %v", e)
		}
	})
	t.Run("negative zero code succeeds", func(t *testing.T) {
		if e, _ := judgeResponse(200, nil, []byte(`{"code":-0}`)); e != nil {
			t.Fatalf("got %v", e)
		}
	})
	t.Run("trailing garbage is not a JSON object", func(t *testing.T) {
		e, _ := judgeResponse(200, nil, []byte(`{"a":1} x`))
		if e == nil {
			t.Fatal("expected failure")
		}
	})
	t.Run("huge Retry-After saturates instead of overflowing", func(t *testing.T) {
		h := http.Header{"Retry-After": {"99999999999999999999"}}
		e, _ := judgeResponse(429, h, []byte(`{"msg":"x"}`))
		if e == nil || e.RetryAfter == nil || *e.RetryAfter <= 0 {
			t.Fatalf("got %+v", e)
		}
	})
	t.Run("Retry-After HTTP date rounds up to whole seconds", func(t *testing.T) {
		at := time.Now().Add(1500 * time.Millisecond).UTC().Format(http.TimeFormat)
		h := http.Header{"Retry-After": {at}}
		e, _ := judgeResponse(429, h, []byte(`{"msg":"x"}`))
		if e == nil || e.RetryAfter == nil || *e.RetryAfter%time.Second != 0 {
			t.Fatalf("got %+v", e)
		}
	})
	t.Run("500 UTF-16 units: astral char straddling the cut is dropped", func(t *testing.T) {
		body := strings.Repeat("x", 499) + "\U0001F600tail"
		e, _ := judgeResponse(502, nil, []byte(body))
		if e == nil {
			t.Fatal("expected failure")
		}
		if want := strings.Repeat("x", 499) + "�"; e.Message != want {
			t.Fatalf("Message = %q", e.Message)
		}
	})
	t.Run("500 UTF-16 units: astral char fully inside the cut is kept", func(t *testing.T) {
		body := strings.Repeat("x", 498) + "\U0001F600tail"
		e, _ := judgeResponse(502, nil, []byte(body))
		if want := strings.Repeat("x", 498) + "\U0001F600"; e.Message != want {
			t.Fatalf("Message = %q", e.Message)
		}
	})
	t.Run("returns the parsed object on success", func(t *testing.T) {
		e, obj := judgeResponse(200, nil, []byte(`{"code":0,"data":{"n":1}}`))
		if e != nil || obj["data"] == nil {
			t.Fatalf("got %v %v", e, obj)
		}
	})
}
