package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const snapshotPath = "../../../testdata/openapi.json"

func runWith(t *testing.T, url, snapshot string) (int, string, string) {
	t.Helper()
	env := map[string]string{"SPEC_DRIFT_URL": url, "SPEC_DRIFT_SNAPSHOT": snapshot}
	var out, errOut bytes.Buffer
	code := run(func(k string) string { return env[k] }, &out, &errOut)
	return code, out.String(), errOut.String()
}

// edited writes a copy of the snapshot after change has edited the named schema's properties.
func edited(t *testing.T, schema string, change func(props map[string]any)) string {
	t.Helper()
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	change(doc["components"].(map[string]any)["schemas"].(map[string]any)[schema].(map[string]any)["properties"].(map[string]any))
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return writeFile(t, string(out))
}

// mutated writes a copy of the snapshot with one extra property on the named schema.
func mutated(t *testing.T, schema string) string {
	t.Helper()
	return edited(t, schema, func(props map[string]any) { props["brand_new_field"] = map[string]any{"type": "string"} })
}

func TestIdenticalSpecsExitZero(t *testing.T) {
	code, out, errOut := runWith(t, snapshotPath, snapshotPath)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "26 request/response schemas") {
		t.Errorf("stdout = %q, want the count of 26 compared schemas", out)
	}
}

// The endpoints added after the first release are watched like the original six.
func TestChangeInAddedEndpointIsDrift(t *testing.T) {
	for schema, want := range map[string]string{
		"ImageSearchRequest":  "/image-search request",
		"VideoSearchResponse": "/video-search response",
		"DesignImageOptions":  "/design-search request",
		"DesignImageResult":   "/design-search response",
		"SearchResponse":      "/search-premier response",
		"ExtractRequest":      "/extract request",
		"ExtractResult":       "/extract response",
		"EmbeddingRequest":    "/embedding request",
		"EmbeddingResult":     "/embedding response",
		"VLEmbeddingContent":  "/vl-embedding request",
		"VLEmbeddingUsage":    "/vl-embedding response",
	} {
		code, _, errOut := runWith(t, mutated(t, schema), snapshotPath)
		if code != 1 || !strings.Contains(errOut, want) || !strings.Contains(errOut, "brand_new_field") {
			t.Errorf("%s: exit %d, want 1 naming %q; stderr: %s", schema, code, want, errOut)
		}
	}
}

func TestChangedSchemaExitsOneAndNamesTheField(t *testing.T) {
	code, _, errOut := runWith(t, mutated(t, "SearchRequest"), snapshotPath)
	if code != 1 {
		t.Fatalf("exit %d, want 1; stderr: %s", code, errOut)
	}
	if want := "/search request"; !strings.Contains(errOut, want) || !strings.Contains(errOut, "brand_new_field") {
		t.Errorf("stderr should name %q and the new field, got: %s", want, errOut)
	}
}

func TestUnusableInputExitsTwo(t *testing.T) {
	garbage := filepath.Join(t.TempDir(), "garbage.json")
	if err := os.WriteFile(garbage, []byte("<html>not json</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string][2]string{
		"garbage live spec":     {garbage, snapshotPath},
		"missing live file":     {filepath.Join(t.TempDir(), "absent.json"), snapshotPath},
		"garbage snapshot":      {snapshotPath, garbage},
		"empty-object live doc": {writeFile(t, "{}"), snapshotPath},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if code, _, errOut := runWith(t, c[0], c[1]); code != 2 {
				t.Errorf("exit %d, want 2; stderr: %s", code, errOut)
			}
		})
	}
}

// Schemas outside the watched endpoints change often and must not raise the alarm.
func TestChangeOutsideTheWatchedEndpointsIsIgnored(t *testing.T) {
	if code, _, errOut := runWith(t, mutated(t, "AnswerRequest"), snapshotPath); code != 0 {
		t.Fatalf("exit %d, want 0; stderr: %s", code, errOut)
	}
}

// Diff only flags a nullable spec field the SDK cannot hold, so a field that stops being
// nullable in the live spec is found by the reverse comparison.
func TestFieldNoLongerNullableIsDrift(t *testing.T) {
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	title := doc["components"].(map[string]any)["schemas"].(map[string]any)["PulseSearchResult"].(map[string]any)["properties"].(map[string]any)["title"].(map[string]any)
	if title["nullable"] != true {
		t.Fatal("snapshot no longer has a nullable PulseSearchResult.title; pick another field")
	}
	delete(title, "nullable")
	out, _ := json.Marshal(doc)
	code, _, errOut := runWith(t, writeFile(t, string(out)), snapshotPath)
	if code != 1 || !strings.Contains(errOut, "title") {
		t.Fatalf("exit %d, want 1 naming title; stderr: %s", code, errOut)
	}
}

func TestPanicExitsTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(func(string) string { panic("boom") }, &out, &errOut); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDeletedResponseFieldIsDrift(t *testing.T) {
	live := edited(t, "PulseSearchResult", func(props map[string]any) { delete(props, "favicon") })
	code, _, errOut := runWith(t, live, snapshotPath)
	if code != 1 || !strings.Contains(errOut, "/pulse-search response") || !strings.Contains(errOut, "favicon") {
		t.Fatalf("exit %d, want 1 naming the response field favicon; stderr: %s", code, errOut)
	}
}

func TestChangedEnumValueIsDrift(t *testing.T) {
	live := edited(t, "WebSearchOptions", func(props map[string]any) {
		props["safesearch"].(map[string]any)["enum"] = []any{"off", "moderate"}
	})
	code, _, errOut := runWith(t, live, snapshotPath)
	if code != 1 || !strings.Contains(errOut, "safesearch") {
		t.Fatalf("exit %d, want 1 naming safesearch; stderr: %s", code, errOut)
	}
}

func TestHTTPFetch(t *testing.T) {
	snapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi.json" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<html>not found</html>"))
			return
		}
		_, _ = w.Write(snapshot)
	}))
	defer s.Close()
	if code, _, errOut := runWith(t, s.URL+"/missing", snapshotPath); code != 2 || !strings.Contains(errOut, "HTTP 404") {
		t.Errorf("404: exit %d, want 2; stderr: %s", code, errOut)
	}
	if code, out, errOut := runWith(t, s.URL+"/openapi.json", snapshotPath); code != 0 || !strings.Contains(out, "No drift") {
		t.Errorf("identical snapshot over HTTP: exit %d, want 0; stderr: %s", code, errOut)
	}
}

// The SDK enforces a single entry in inputs locally, so a change of its item bounds must be drift
// although the field comparison ignores minItems and maxItems.
func TestInputsItemCountChangeIsDrift(t *testing.T) {
	cases := []struct {
		schema, want string
		change       func(inputs map[string]any)
	}{
		{"ImageSearchRequest", "/image-search request.inputs", func(in map[string]any) { in["maxItems"] = 2.0 }},
		{"ImageSearchRequest", "/image-search request.inputs", func(in map[string]any) { delete(in, "minItems") }},
		{"VideoSearchRequest", "/video-search request.inputs", func(in map[string]any) { delete(in, "maxItems") }},
		{"VideoSearchRequest", "/video-search request.inputs", func(in map[string]any) { in["minItems"] = 1.0 }},
	}
	for _, c := range cases {
		live := edited(t, c.schema, func(props map[string]any) { c.change(props["inputs"].(map[string]any)) })
		code, _, errOut := runWith(t, live, snapshotPath)
		if code != 1 || !strings.Contains(errOut, c.want) || !strings.Contains(errOut, "maxItems") {
			t.Errorf("%s: exit %d, want 1 naming %q with the item bounds; stderr: %s", c.schema, code, c.want, errOut)
		}
	}
}

// The SDK checks 1 to 1000 Embedding inputs and 1 to 20 VL contents locally; the VL bound sits on
// the nested VLEmbeddingInput.contents.
func TestEmbeddingItemCountChangeIsDrift(t *testing.T) {
	cases := []struct {
		schema, property, want string
		change                 func(p map[string]any)
	}{
		{"EmbeddingRequest", "input", "/embedding request.input: item bounds", func(p map[string]any) { p["maxItems"] = 2048.0 }},
		{"EmbeddingRequest", "input", "/embedding request.input: item bounds", func(p map[string]any) { p["minItems"] = 2.0 }},
		{"VLEmbeddingInput", "contents", "/vl-embedding request.input.contents: item bounds", func(p map[string]any) { p["maxItems"] = 30.0 }},
		{"VLEmbeddingInput", "contents", "/vl-embedding request.input.contents: item bounds", func(p map[string]any) { p["minItems"] = 2.0 }},
	}
	for _, c := range cases {
		live := edited(t, c.schema, func(props map[string]any) { c.change(props[c.property].(map[string]any)) })
		code, _, errOut := runWith(t, live, snapshotPath)
		if code != 1 || !strings.Contains(errOut, c.want) {
			t.Errorf("%s.%s: exit %d, want 1 naming %q; stderr: %s", c.schema, c.property, code, c.want, errOut)
		}
	}
}

// The SDK rejects a VL content item with zero or several of text, image and video; the spec states
// that rule only in the VLEmbeddingContent description.
func TestVLContentRuleChangeIsDrift(t *testing.T) {
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	content := doc["components"].(map[string]any)["schemas"].(map[string]any)["VLEmbeddingContent"].(map[string]any)
	content["description"] = "A single content element. Each object may combine text, image and video."
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runWith(t, writeFile(t, string(out)), snapshotPath)
	if want := "/vl-embedding request.input.contents[]: description"; code != 1 || !strings.Contains(errOut, want) {
		t.Fatalf("exit %d, want 1 naming %q; stderr: %s", code, want, errOut)
	}
}

// Flatten ignores descriptions, lengths and value ranges, yet the SDK relies on these Extract values:
// the urls limits it checks locally (stated in the description) and the per-URL timeout its default derives from.
func TestExtractLocalCheckChangeIsDrift(t *testing.T) {
	prop := func(props map[string]any, name string) map[string]any { return props[name].(map[string]any) }
	cases := []struct {
		name, want string
		change     func(props map[string]any)
	}{
		{"urls maxItems added", "/extract request.urls: item bounds", func(p map[string]any) { prop(p, "urls")["maxItems"] = 20.0 }},
		{"urls description", "/extract request.urls: description", func(p map[string]any) {
			prop(p, "urls")["description"] = "List of URLs to extract content from. Maximum URLs per request: 50."
		}},
		{"urls item maxLength added", "/extract request.urls[]: item length bounds", func(p map[string]any) {
			prop(p, "urls")["items"].(map[string]any)["maxLength"] = 2048.0
		}},
		{"urls item minLength added", "/extract request.urls[]: item length bounds", func(p map[string]any) {
			prop(p, "urls")["items"].(map[string]any)["minLength"] = 1.0
		}},
		{"timeout maximum", "/extract request.timeout: default and range", func(p map[string]any) { prop(p, "timeout")["maximum"] = 120.0 }},
		{"timeout minimum removed", "/extract request.timeout: default and range", func(p map[string]any) { delete(prop(p, "timeout"), "minimum") }},
		{"timeout default", "/extract request.timeout: default and range", func(p map[string]any) { prop(p, "timeout")["default"] = 60.0 }},
	}
	for _, c := range cases {
		live := edited(t, "ExtractRequest", c.change)
		code, _, errOut := runWith(t, live, snapshotPath)
		if code != 1 || !strings.Contains(errOut, c.want) {
			t.Errorf("%s: exit %d, want 1 naming %q; stderr: %s", c.name, code, c.want, errOut)
		}
	}
	// A property the check reads that disappears cannot be compared, which needs a human look.
	live := edited(t, "ExtractRequest", func(p map[string]any) { delete(p, "timeout") })
	if code, _, errOut := runWith(t, live, snapshotPath); code != 2 {
		t.Errorf("timeout removed: exit %d, want 2; stderr: %s", code, errOut)
	}
}
