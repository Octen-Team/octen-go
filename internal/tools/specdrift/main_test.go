package main

import (
	"bytes"
	"encoding/json"
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

// mutated writes a copy of the snapshot with one extra property on the named schema.
func mutated(t *testing.T, schema string) string {
	t.Helper()
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	props := doc["components"].(map[string]any)["schemas"].(map[string]any)[schema].(map[string]any)["properties"].(map[string]any)
	props["brand_new_field"] = map[string]any{"type": "string"}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "live.json")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIdenticalSpecsExitZero(t *testing.T) {
	code, out, errOut := runWith(t, snapshotPath, snapshotPath)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "12 request/response schemas") {
		t.Errorf("stdout = %q, want the count of 12 compared schemas", out)
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

// Schemas outside the six endpoints change often and must not raise the alarm.
func TestChangeOutsideTheSixEndpointsIsIgnored(t *testing.T) {
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
