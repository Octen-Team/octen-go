package octen_test

import (
	"errors"
	"math"
	"net/http"
	"testing"
	"time"

	octen "github.com/Octen-Team/octen-go"
)

// Media: public CC0 files the API could fetch on 2026-11-26.
const (
	liveImageURL = "https://interactive-examples.mdn.mozilla.net/media/cc0-images/grapefruit-slice-332-332.jpg"
	liveVideoURL = "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"
)

func TestLiveEmbeddingBasic(t *testing.T) {
	c := liveClient(t)
	resp, err := c.Embedding.Create(liveContext(t), octen.EmbeddingParams{
		Input: []string{"What is vector search?"}, Model: octen.String("octen-embedding-0.6b"),
		Dimension: octen.Int(8), InputType: octen.String("query"),
	})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, resp.RequestID, resp.Data != nil)
	if len(resp.Data.Results) != 1 || len(resp.Data.Results[0].Embedding) != 8 || resp.Data.Model != "octen-embedding-0.6b" {
		t.Fatalf("data = %+v", resp.Data)
	}
	if resp.Meta == nil || resp.Meta.Usage == nil || resp.Meta.Usage.InputTokens <= 0 {
		t.Fatalf("meta = %+v", resp.Meta)
	}
}

func TestLiveEmbeddingBatchOrder(t *testing.T) {
	c := liveClient(t)
	texts := []string{"red apple", "blue ocean", "green forest"}
	params := func(input []string) octen.EmbeddingParams {
		return octen.EmbeddingParams{Input: input, Model: octen.String("octen-embedding-0.6b"), Dimension: octen.Int(8)}
	}
	batch, err := c.Embedding.Create(liveContext(t), params(texts))
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Data.Results) != len(texts) {
		t.Fatalf("results = %d, want %d", len(batch.Data.Results), len(texts))
	}
	// Control: each text embedded alone. Batched values differ by about 1e-4 (seen 2026-11-27), so the batch
	// vector at position i must be the one closest to text i alone, not equal to it.
	alone := make([][]float64, len(texts))
	for i, text := range texts {
		if got := batch.Data.Results[i].Index; got != i {
			t.Fatalf("results[%d].index = %d", i, got)
		}
		single, err := c.Embedding.Create(liveContext(t), params([]string{text}))
		if err != nil {
			t.Fatal(err)
		}
		alone[i] = single.Data.Results[0].Embedding
	}
	for i := range texts {
		b := batch.Data.Results[i].Embedding
		for j := range texts {
			sim := cosine(b, alone[j])
			t.Logf("cos(batch[%d], alone[%d]) = %.5f", i, j, sim)
			if (j == i && sim < 0.99) || (j != i && sim >= cosine(b, alone[i])) {
				t.Fatalf("batch[%d] is not closest to %q alone", i, texts[i])
			}
		}
	}
}

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for k := range a {
		dot, na, nb = dot+a[k]*b[k], na+a[k]*a[k], nb+b[k]*b[k]
	}
	return dot / math.Sqrt(na*nb)
}

func TestLiveEmbeddingDimensionOverMax(t *testing.T) {
	c := liveClient(t)
	_, err := c.Embedding.Create(liveContext(t), octen.EmbeddingParams{
		Input: []string{"a"}, Model: octen.String("octen-embedding-0.6b"), Dimension: octen.Int(4096),
	})
	if !errors.Is(err, octen.ErrBadRequest) {
		t.Fatalf("want ErrBadRequest, got %v", err)
	}
}

func vlTextAndImage() octen.VLEmbeddingInput {
	return octen.VLEmbeddingInput{Contents: []octen.VLEmbeddingContent{
		octen.TextContent("grapefruit"), octen.ImageContent(liveImageURL),
	}}
}

func TestLiveVLEmbeddingIndependent(t *testing.T) {
	c := liveClient(t)
	resp, err := c.VLEmbedding.Create(liveContext(t), octen.VLEmbeddingParams{
		Model: "octen-vl-embedding", Input: vlTextAndImage(), Dimension: octen.Int(8),
	})
	if err != nil {
		t.Fatal(err)
	}
	requireRequestID(t, resp.RequestID, resp.Data != nil)
	r := resp.Data.Results
	if len(r) != 2 || r[0].Type != "vl" || r[1].Type != "vl" || len(r[1].Embedding) != 8 {
		t.Fatalf("results = %+v", r)
	}
	if resp.Meta == nil || resp.Meta.Usage == nil || resp.Meta.Usage.ImageCount != 1 {
		t.Fatalf("meta = %+v", resp.Meta)
	}
}

func TestLiveVLEmbeddingFusion(t *testing.T) {
	c := liveClient(t)
	resp, err := c.VLEmbedding.Create(liveContext(t), octen.VLEmbeddingParams{
		Model: "octen-vl-embedding", Input: vlTextAndImage(), Dimension: octen.Int(8), EnableFusion: octen.Bool(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if r := resp.Data.Results; len(r) != 1 || r[0].Type != "fusion" {
		t.Fatalf("results = %+v", r)
	}
}

func TestLiveVLEmbeddingVideo(t *testing.T) {
	c := liveClient(t)
	started := time.Now()
	resp, err := c.VLEmbedding.Create(liveContext(t), octen.VLEmbeddingParams{
		Model: "octen-vl-embedding", Input: octen.VLEmbeddingInput{Contents: []octen.VLEmbeddingContent{octen.VideoContent(liveVideoURL)}},
		Dimension: octen.Int(8), FPS: octen.Float64(0.5),
	})
	t.Logf("took %v", time.Since(started).Round(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Data.Results) != 1 || resp.Meta == nil || resp.Meta.Usage == nil || resp.Meta.Usage.Duration <= 0 {
		t.Fatalf("data = %+v, meta = %+v", resp.Data, resp.Meta)
	}
}

// A 400 must not be retried: the request is sent exactly once.
func TestLiveVLEmbeddingUnreachableMedia(t *testing.T) {
	ct := &countingTransport{next: http.DefaultTransport}
	c := liveClient(t, octen.WithHTTPClient(&http.Client{Transport: ct}))
	_, err := c.VLEmbedding.Create(liveContext(t), octen.VLEmbeddingParams{
		Model: "octen-vl-embedding", Input: octen.VLEmbeddingInput{Contents: []octen.VLEmbeddingContent{octen.ImageContent("https://example.invalid/a.jpg")}},
	})
	if !errors.Is(err, octen.ErrBadRequest) {
		t.Fatalf("want ErrBadRequest, got %v", err)
	}
	if n := ct.calls.Load(); n != 1 {
		t.Fatalf("round trips = %d, want exactly 1", n)
	}
}
