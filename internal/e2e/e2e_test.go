// Package e2e builds a separate module that consumes the SDK through a replace directive,
// so the public API surface and module metadata are exercised the way a user sees them.
package e2e

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type seen struct {
	path, key, agent string
}

func fakeAPI(t *testing.T) (*httptest.Server, func() []seen) {
	t.Helper()
	var mu sync.Mutex
	var got []seen
	bodies := map[string]string{
		"/search":          `{"code":0,"msg":"ok","request_id":"rid-search","data":{"query":"q","results":[{"title":"t","url":"https://example.com/a"}]}}`,
		"/broad-search":    `{"code":0,"msg":"ok","request_id":"rid-broad-search","data":{"query":"q","queries":["q1"],"search_results":[]}}`,
		"/news-search":     `{"code":0,"msg":"ok","request_id":"rid-news-search","data":{"query":"q","results":[{"title":"n","url":"https://example.com/n"}]}}`,
		"/business-search": `{"code":0,"msg":"ok","request_id":"rid-business-search","data":{"query":"q","results":[{"title":"b","url":"https://example.com/b"}]}}`,
		"/pulse-search":    `{"code":0,"msg":"ok","request_id":"rid-pulse-search","data":{"query":"q","results":[]},"meta":{"usage":{"billed_mode":"lite"}}}`,
		"/place-search":    `{"code":0,"msg":"ok","request_id":"rid-place-search","data":{"query":"q","places":[{"name":"p"}]}}`,
		"/search-premier":  `{"code":0,"msg":"ok","request_id":"rid-search-premier","data":{"query":"q","results":[{"url":"https://example.com/p"}]},"meta":{"result_type":"dynamic"}}`,
		"/image-search":    `{"code":0,"msg":"","request_id":"rid-image-search","data":{"results":[{"url":"https://example.com/i.jpg","width":10}]}}`,
		"/video-search":    `{"code":0,"msg":"","request_id":"rid-video-search","data":{"results":[{"url":"https://example.com/v","duration_seconds":78}]}}`,
		"/design-search":   `{"code":0,"msg":"success","request_id":"rid-design-search","data":{"images":[{"type":"slide","group":{"group_id":"g","position":1,"total":2}}],"results":[]}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, seen{r.URL.Path, r.Header.Get("x-api-key"), r.Header.Get("User-Agent")})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err := checkBody(r.URL.Path, body); err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"code":422,"msg":"fake API: `+err.Error()+`","request_id":"rid-shape"}`)
			return
		}
		if strings.Contains(string(body), `"query":"bad"`) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"code":400,"msg":"bad query","request_id":"rid-bad"}`)
			return
		}
		b, ok := bodies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, b)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seen {
		mu.Lock()
		defer mu.Unlock()
		return append([]seen(nil), got...)
	}
}

type fakeInput struct {
	Type string  `json:"type"`
	URL  *string `json:"url"`
	Data *string `json:"data"`
}

// checkInput accepts a text input with data and no url, and, when images are allowed, an image
// input with exactly one non-empty url or data.
func checkInput(in fakeInput, images bool) error {
	nonEmpty := func(s *string) bool { return s != nil && *s != "" }
	switch {
	case in.Type == "text" && nonEmpty(in.Data) && in.URL == nil:
		return nil
	case in.Type == "image" && images && (in.URL != nil) != (in.Data != nil) && (nonEmpty(in.URL) || nonEmpty(in.Data)):
		return nil
	}
	return errors.New("invalid input")
}

// checkBody rejects a body without the input its path requires: one valid entry in inputs for
// image and video search, a query or images.group_id for design search, and a query elsewhere.
func checkBody(path string, body []byte) error {
	var b struct {
		Query  *string     `json:"query"`
		Inputs []fakeInput `json:"inputs"`
		Images *struct {
			GroupID string `json:"group_id"`
		} `json:"images"`
	}
	if err := json.Unmarshal(body, &b); err != nil {
		return errors.New("body is not a JSON object")
	}
	hasQuery := b.Query != nil && *b.Query != ""
	switch path {
	case "/image-search", "/video-search":
		if len(b.Inputs) != 1 || b.Query != nil {
			return errors.New("want exactly one inputs entry and no query")
		}
		if err := checkInput(b.Inputs[0], path == "/image-search"); err != nil {
			return err
		}
	case "/design-search":
		if b.Inputs != nil || (!hasQuery && (b.Images == nil || b.Images.GroupID == "")) {
			return errors.New("want a query or images.group_id")
		}
	default:
		if !hasQuery || b.Inputs != nil {
			return errors.New("want a query")
		}
	}
	return nil
}

// goCmd runs the local toolchain only, offline: the replace directive means nothing is downloaded.
func goCmd(t *testing.T, dir string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append([]string{"GOTOOLCHAIN=local", "GOFLAGS=-mod=mod", "GOPROXY=off"}, env...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := goCmd(t, ".", nil, "env", "GOMOD")
	if err != nil {
		t.Fatalf("go env GOMOD: %v\n%s", err, out)
	}
	return filepath.Dir(strings.TrimSpace(out))
}

// newConsumer lays out a standalone module whose only link to the SDK is a replace directive.
func newConsumer(t *testing.T, mainSrc string) string {
	t.Helper()
	dir := t.TempDir()
	gomod := "module example.com/consumer\n\ngo 1.22\n\n" +
		"require github.com/Octen-Team/octen-go v0.0.0\n\n" +
		"replace github.com/Octen-Team/octen-go => " + repoRoot(t) + "\n"
	for name, content := range map[string]string{"go.mod": gomod, "main.go": mainSrc} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestConsumerModule(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a separate module")
	}
	src, err := os.ReadFile(filepath.Join("consumer", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	dir := newConsumer(t, string(src))

	if out, err := goCmd(t, dir, nil, "build", "-o", "consumer.bin", "."); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if out, err := goCmd(t, dir, nil, "vet", "./..."); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "go.sum")); len(b) != 0 {
		t.Fatalf("go.sum is not empty, the SDK must have no dependencies:\n%s", b)
	}

	srv, requests := fakeAPI(t)
	cmd := exec.Command(filepath.Join(dir, "consumer.bin"))
	cmd.Env = append(os.Environ(), "OCTEN_E2E_URL="+srv.URL)
	out, err := cmd.CombinedOutput()
	t.Logf("consumer output:\n%s", out)
	if err != nil {
		t.Fatalf("consumer failed: %v", err)
	}
	for _, want := range []string{
		"OK search rid-search", "OK broad-search rid-broad-search", "OK news-search rid-news-search",
		"OK business-search rid-business-search", "OK pulse-search rid-pulse-search",
		"OK place-search rid-place-search", "OK search-premier rid-search-premier", "OK image-search rid-image-search",
		"OK video-search rid-video-search", "OK design-search rid-design-search", "OK error-path 400 rid-bad", "OK validation query",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output lacks %q", want)
		}
	}

	// Ten successes, then the 400; the blank query must not reach the server.
	wantPaths := []string{"/search", "/broad-search", "/news-search", "/business-search", "/pulse-search", "/place-search",
		"/search-premier", "/image-search", "/video-search", "/design-search", "/search"}
	got := requests()
	if len(got) != len(wantPaths) {
		t.Fatalf("server saw %d requests, want %d: %+v", len(got), len(wantPaths), got)
	}
	for i, r := range got {
		if r.path != wantPaths[i] || r.key != "e2e-key" || r.agent != "octen-go-sdk/0.2.0" {
			t.Errorf("request %d = %+v, want path %s with the SDK key and user agent", i, r, wantPaths[i])
		}
	}
}

// A misspelled field must be a build error that names the field: this is the compile-time
// guarantee that replaces the runtime unknown-key check of dynamic SDKs.
func TestConsumerFieldTypoFailsToCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a separate module")
	}
	dir := newConsumer(t, `package main

import octen "github.com/Octen-Team/octen-go"

func main() {
	_ = octen.SearchParams{Query: "q", Cuont: octen.Int(1)}
}
`)
	out, err := goCmd(t, dir, nil, "build", "-o", os.DevNull, ".")
	if err == nil {
		t.Fatal("go build succeeded, want a compile error")
	}
	var named string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Cuont") {
			named = line
			break
		}
	}
	if named == "" {
		t.Fatalf("compile error does not mention Cuont:\n%s", out)
	}
	t.Logf("compile error: %s", named)
}

// The fake API answers only bodies of the shape each endpoint requires, so a call that sends the
// wrong body to a known path fails the consumer run.
func TestFakeAPIChecksBodyShape(t *testing.T) {
	cases := []struct {
		path, body string
		ok         bool
	}{
		{"/search", `{"query":"q"}`, true},
		{"/search", `{}`, false},
		{"/search-premier", `{"query":""}`, false},
		{"/place-search", `{"query":"q","latitude":1}`, true},
		{"/image-search", `{"inputs":[{"type":"text","data":"q"}]}`, true},
		{"/image-search", `{}`, false},
		{"/image-search", `{"inputs":[]}`, false},
		{"/image-search", `{"query":"q"}`, false},
		{"/image-search", `{"inputs":[{"type":"image","url":"https://example.com/a.jpg"}]}`, true},
		{"/image-search", `{"inputs":[{"type":"image","data":"aGVsbG8="}]}`, true},
		{"/image-search", `{"inputs":[{"type":"image","url":"https://example.com/a.jpg","data":"aGVsbG8="}]}`, false},
		{"/image-search", `{"inputs":[{"type":"image","url":""}]}`, false},
		{"/image-search", `{"inputs":[{"type":"image"}]}`, false},
		{"/image-search", `{"inputs":[{"type":"text","data":"q","url":"https://example.com/a.jpg"}]}`, false},
		{"/image-search", `{"inputs":[{"type":"text","data":""}]}`, false},
		{"/image-search", `{"inputs":[{"type":"audio","data":"q"}]}`, false},
		{"/video-search", `{"inputs":[{"type":"text","data":"q"}]}`, true},
		{"/video-search", `{"inputs":[{"type":"text","data":"a"},{"type":"text","data":"b"}]}`, false},
		{"/video-search", `{"inputs":[{"type":"image","url":"https://example.com/a.jpg"}]}`, false},
		{"/video-search", `{"inputs":[{"type":"text","data":""}]}`, false},
		{"/design-search", `{"query":"q"}`, true},
		{"/design-search", `{"images":{"group_id":"g"}}`, true},
		{"/design-search", `{"inputs":[{"type":"text","data":"q"}]}`, false},
		{"/design-search", `{"images":{"group_id":""}}`, false},
		{"/search", `not json`, false},
		{"/search", `{"query":"q","inputs":[{"type":"text","data":"q"}]}`, false},
	}
	for _, c := range cases {
		if err := checkBody(c.path, []byte(c.body)); (err == nil) != c.ok {
			t.Errorf("checkBody(%s, %s) = %v, want ok %v", c.path, c.body, err, c.ok)
		}
	}
}
