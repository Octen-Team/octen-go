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
	if !strings.Contains(out, "20 request/response schemas") {
		t.Errorf("stdout = %q, want the count of 20 compared schemas", out)
	}
}

// The four endpoints added after the first release are watched like the original six.
func TestChangeInAddedEndpointIsDrift(t *testing.T) {
	for schema, want := range map[string]string{
		"ImageSearchRequest":  "/image-search request",
		"VideoSearchResponse": "/video-search response",
		"DesignImageOptions":  "/design-search request",
		"DesignImageResult":   "/design-search response",
		"SearchResponse":      "/search-premier response",
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
	if code, _, errOut := runWith(t, mutated(t, "ExtractRequest"), snapshotPath); code != 0 {
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
