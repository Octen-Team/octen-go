package octen

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Fields and limits: testdata/openapi.json, VLEmbeddingRequest / VLEmbeddingInput / VLEmbeddingContent /
// VLEmbeddingResponse (https://docs.octen.ai/api-reference/vl-embedding).
func TestVLEmbeddingSendsNestedContents(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	cases := []struct {
		name, want string
		params     VLEmbeddingParams
	}{
		{"every field", `{"model":"octen-vl-embedding-large","input":{"contents":[{"text":"tent"},` +
			`{"image":"https://example.com/a.jpg"},{"video":"https://example.com/v.mp4"}]},` +
			`"enable_fusion":true,"dimension":2048,"fps":0.3,"instruct":"Represent the product"}`,
			VLEmbeddingParams{
				Model: "octen-vl-embedding-large",
				Input: VLEmbeddingInput{Contents: []VLEmbeddingContent{
					TextContent("tent"), ImageContent("https://example.com/a.jpg"), VideoContent("https://example.com/v.mp4"),
				}},
				EnableFusion: Bool(true), Dimension: Int(2048), FPS: Float64(0.3), Instruct: String("Represent the product"),
			}},
		{"unset fields omitted", `{"model":"octen-vl-embedding","input":{"contents":[{"text":"a"}]}}`,
			VLEmbeddingParams{Model: "octen-vl-embedding", Input: VLEmbeddingInput{Contents: []VLEmbeddingContent{TextContent("a")}}}},
		{"enable_fusion false is sent", `{"model":"octen-vl-embedding","input":{"contents":[{"text":"a"}]},"enable_fusion":false}`,
			VLEmbeddingParams{Model: "octen-vl-embedding", Input: VLEmbeddingInput{Contents: []VLEmbeddingContent{TextContent("a")}}, EnableFusion: Bool(false)}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := c.VLEmbedding.Create(context.Background(), tc.params)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			got := rec.requests()
			if r := got[i]; r.method != http.MethodPost || r.path != "/vl-embedding" || r.body != tc.want {
				t.Fatalf("request = %s %s %s\nwant      POST /vl-embedding %s", r.method, r.path, r.body, tc.want)
			}
			if resp.RequestID != "rid/vl-embedding" || string(resp.RawJSON()) != rawBodyFor("/vl-embedding") {
				t.Fatalf("RequestID = %q, RawJSON = %q", resp.RequestID, resp.RawJSON())
			}
		})
	}
}

// The server answers these with a 400 (checked 2026-11-26), so they are not checked here.
func TestVLEmbeddingDoesNotCheckCountsOrRangesLocally(t *testing.T) {
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	six := make([]VLEmbeddingContent, 6)
	for i := range six {
		six[i] = ImageContent("https://example.com/a.jpg")
	}
	p := VLEmbeddingParams{Model: "octen-vl-embedding", Input: VLEmbeddingInput{Contents: six}, FPS: Float64(1.5), Dimension: Int(99999)}
	if _, err := c.VLEmbedding.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.requests()); n != 1 {
		t.Fatalf("server saw %d requests, want 1", n)
	}
}

func TestVLEmbeddingValidation(t *testing.T) {
	contents := func(n int) []VLEmbeddingContent {
		out := make([]VLEmbeddingContent, n)
		for i := range out {
			out[i] = TextContent("a")
		}
		return out
	}
	both := VLEmbeddingContent{Text: String("a"), Image: String("https://example.com/a.jpg")}
	rec := newRecorder(t)
	c := newTestClient(t, rec.URL)
	for _, tc := range []struct {
		name, field string
		contents    []VLEmbeddingContent
	}{
		{"empty contents", "input.contents", nil},
		{"more than 20 contents", "input.contents", contents(21)},
		// The API reference says each item holds exactly one of text, image or video
		{"no modality", "input.contents[0]", []VLEmbeddingContent{{}}},
		{"two modalities", "input.contents[1]", []VLEmbeddingContent{TextContent("a"), both}},
		{"blank value", "input.contents[0]", []VLEmbeddingContent{TextContent("  ")}},
		{"empty image", "input.contents[0]", []VLEmbeddingContent{ImageContent("")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := VLEmbeddingParams{Model: "octen-vl-embedding", Input: VLEmbeddingInput{Contents: tc.contents}}
			_, err := c.VLEmbedding.Create(context.Background(), p)
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("err = %v, want *ValidationError on %s", err, tc.field)
			}
		})
	}
	// model is required by the API reference; its value is left to the server
	for _, model := range []string{"", "  "} {
		p := VLEmbeddingParams{Model: model, Input: VLEmbeddingInput{Contents: contents(1)}}
		_, err := c.VLEmbedding.Create(context.Background(), p)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != "model" {
			t.Fatalf("model %q: err = %v, want *ValidationError on model", model, err)
		}
	}
	if n := len(rec.requests()); n != 0 {
		t.Fatalf("server saw %d requests, want none", n)
	}
	p := VLEmbeddingParams{Model: "not-a-model", Input: VLEmbeddingInput{Contents: contents(20)}}
	if _, err := c.VLEmbedding.Create(context.Background(), p); err != nil {
		t.Fatalf("20 contents: %v", err)
	}
}

// The fusion and independent 200 examples of POST /vl-embedding in testdata/openapi.json.
func TestVLEmbeddingDecodesFusionAndIndependent(t *testing.T) {
	const usage = `"meta":{"usage":{"input_tokens":6814,"text_tokens":18,"image_tokens":6796,"image_count":2,"duration":22},"warning":""}`
	result := func(i, kind string) string {
		return `{"index":` + i + `,"embedding":[0.0156,-0.0298,0.0411],"type":"` + kind + `"}`
	}
	bodies := map[string]string{
		"fusion": `{"code":0,"msg":"success","request_id":"a7b8c9d0","data":{"results":[` + result("0", "fusion") +
			`],"model":"octen-vl-embedding-large"},` + usage + `}`,
		"independent": `{"code":0,"request_id":"a7b8c9d0","data":{"results":[` + result("0", "vl") + `,` + result("1", "vl") + `,` +
			result("2", "vl") + `,` + result("3", "vl") + `],"model":"octen-vl-embedding-large"},` + usage + `}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(s.Close)
			c := newTestClient(t, s.URL)
			p := VLEmbeddingParams{Model: "octen-vl-embedding", Input: VLEmbeddingInput{Contents: []VLEmbeddingContent{TextContent("a")}}}
			resp, err := c.VLEmbedding.Create(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Meta == nil || resp.Meta.Usage == nil || resp.Meta.Usage.ImageCount != 2 || resp.Meta.Usage.Duration != 22 ||
				resp.Meta.Warning == nil || *resp.Meta.Warning != "" {
				t.Fatalf("meta = %+v", resp.Meta)
			}
			want := map[string][]string{"fusion": {"fusion"}, "independent": {"vl", "vl", "vl", "vl"}}[name]
			if resp.Data == nil || len(resp.Data.Results) != len(want) {
				t.Fatalf("data = %+v", resp.Data)
			}
			for i, r := range resp.Data.Results {
				if r.Type != want[i] || r.Index != i || len(r.Embedding) != 3 {
					t.Fatalf("results[%d] = %+v", i, r)
				}
			}
		})
	}
}
