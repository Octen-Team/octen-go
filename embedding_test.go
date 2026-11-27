package octen

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// Fields and limits: testdata/openapi.json, EmbeddingRequest / EmbeddingResponse
// (https://docs.octen.ai/api-reference/embedding).
func TestEmbeddingSendsOnlyDocumentedFields(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	cases := []struct {
		name, want string
		params     EmbeddingParams
	}{
		{"every field", `{"input":["a"],"model":"octen-embedding-0.6b","dimension":8,"input_type":"query"}`,
			EmbeddingParams{Input: []string{"a"}, Model: String("octen-embedding-0.6b"), Dimension: Int(8), InputType: String("query")}},
		{"unset fields omitted", `{"input":["a","b"]}`, EmbeddingParams{Input: []string{"a", "b"}}},
		// The server answers a dimension above the model maximum with a 400; it is not checked here
		{"dimension not checked locally", `{"input":["a"],"dimension":99999}`, EmbeddingParams{Input: []string{"a"}, Dimension: Int(99999)}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := c.Embedding.Create(context.Background(), tc.params)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			got := rec.requests()
			if len(got) != i+1 {
				t.Fatalf("server saw %d requests, want %d", len(got), i+1)
			}
			if r := got[i]; r.method != http.MethodPost || r.path != "/embedding" || r.body != tc.want {
				t.Fatalf("request = %s %s %s\nwant      POST /embedding %s", r.method, r.path, r.body, tc.want)
			}
			if resp.RequestID != "rid/embedding" || string(resp.RawJSON()) != rawBodyFor("/embedding") {
				t.Fatalf("RequestID = %q, RawJSON = %q", resp.RequestID, resp.RawJSON())
			}
		})
	}
}

func TestEmbeddingValidation(t *testing.T) {
	texts := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "t" + strconv.Itoa(i)
		}
		return out
	}
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	for _, tc := range []struct {
		name, field string
		input       []string
	}{
		{"empty input", "input", nil},
		{"more than 1000 items", "input", texts(1001)},
		// The server accepts a whitespace-only item and bills it (checked 2026-11-26)
		{"blank item", "input[2]", []string{"a", "b", "  "}},
		{"empty item", "input[0]", []string{""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Embedding.Create(context.Background(), EmbeddingParams{Input: tc.input})
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("err = %v, want *ValidationError on %s", err, tc.field)
			}
		})
	}
	if n := len(rec.requests()); n != 0 {
		t.Fatalf("server saw %d requests, want none", n)
	}
	if _, err := c.Embedding.Create(context.Background(), EmbeddingParams{Input: texts(1000)}); err != nil {
		t.Fatalf("1000 items: %v", err)
	}
}

func TestEmbeddingPerCallTimeoutOverridesDefault(t *testing.T) {
	setKey(t)
	for _, tc := range []struct {
		name string
		req  []RequestOption
		want time.Duration
	}{
		{"default", nil, defaultTimeout},
		{"per request", []RequestOption{WithRequestTimeout(90 * time.Second)}, 90 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cd capturedDeadline
			c, err := NewClient(WithHTTPClient(cd.client()))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Embedding.Create(context.Background(), EmbeddingParams{Input: []string{"a"}}, tc.req...); err != nil {
				t.Fatal(err)
			}
			if !cd.ok || cd.left > tc.want || cd.left < tc.want-2*time.Second {
				t.Fatalf("time left on attempt context = %v (deadline set: %v), want just under %v", cd.left, cd.ok, tc.want)
			}
		})
	}
}

// The 200 example of POST /embedding in testdata/openapi.json.
func TestEmbeddingDecodesResponse(t *testing.T) {
	body := `{"code":0,"msg":"success","request_id":"req_abc123def456","data":{"results":[` +
		`{"index":0,"embedding":[0.0123,-0.0456,0.0789]},{"index":1,"embedding":[-0.0011,0.0234,0.0567]}],` +
		`"model":"octen-embedding-0.6b"},"meta":{"usage":{"input_tokens":128},"latency":167,` +
		`"warning":"Input was truncated for one or more items."}}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	c := newTestClient(t, s.URL)
	resp, err := c.Embedding.Create(context.Background(), EmbeddingParams{Input: []string{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Data == nil || len(resp.Data.Results) != 2 || resp.Data.Results[1].Index != 1 || resp.Data.Results[1].Embedding[2] != 0.0567 {
		t.Fatalf("data = %+v", resp.Data)
	}
	if resp.Data.Model != "octen-embedding-0.6b" {
		t.Fatalf("model = %q", resp.Data.Model)
	}
	if resp.Meta == nil || resp.Meta.Usage == nil || resp.Meta.Usage.InputTokens != 128 || resp.Meta.Latency != 167 ||
		resp.Meta.Warning == nil || *resp.Meta.Warning != "Input was truncated for one or more items." {
		t.Fatalf("meta = %+v", resp.Meta)
	}
}
