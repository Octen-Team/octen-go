// Command specdrift compares the SDK endpoints' schemas, and the item bounds of the inputs the
// SDK checks locally, in the live OpenAPI spec with testdata/openapi.json. Exit codes: 0 no
// drift, 1 drift, 2 cannot run. It never rewrites the snapshot, which is updated by hand after
// a drift is reviewed.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/Octen-Team/octen-go/internal/spec"
)

const (
	defaultURL      = "https://docs.octen.ai/api-reference/openapi.json"
	defaultSnapshot = "testdata/openapi.json"
	maxSpecBytes    = 64 << 20
)

var endpoints = []string{
	"/search", "/broad-search", "/news-search", "/business-search", "/pulse-search", "/place-search",
	"/search-premier", "/image-search", "/video-search", "/design-search",
}

// itemBounds lists the arrays whose item count the SDK enforces locally; Flatten ignores
// minItems and maxItems, so their bounds are compared separately.
var itemBounds = []struct{ path, property string }{
	{"/image-search", "inputs"},
	{"/video-search", "inputs"},
}

type flatSchema struct {
	name string
	side spec.Side
	obj  *spec.Object
}

func main() {
	os.Exit(run(os.Getenv, os.Stdout, os.Stderr))
}

// run returns the exit code. Any panic is reported as "cannot run" (2), never as drift (1).
func run(getenv func(string) string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "specdrift: unexpected failure: %v\n", r)
			code = 2
		}
	}()
	url := envOr(getenv, "SPEC_DRIFT_URL", defaultURL)
	snapshotPath := envOr(getenv, "SPEC_DRIFT_SNAPSHOT", defaultSnapshot)

	snapshot, snapBounds, err := flattenAll(func() ([]byte, error) { return os.ReadFile(snapshotPath) })
	if err != nil {
		fmt.Fprintf(stderr, "specdrift: cannot read the snapshot (%s): %v\n", snapshotPath, err)
		return 2
	}
	live, liveBounds, err := flattenAll(func() ([]byte, error) { return fetch(url) })
	if err != nil {
		fmt.Fprintf(stderr, "specdrift: cannot read the live spec (%s): %v\n", url, err)
		return 2
	}

	var diffs []string
	for i, snap := range snapshot {
		for _, d := range compare(snap, live[i]) {
			diffs = append(diffs, fmt.Sprintf("%s %s%s", snap.name, snap.side, d))
		}
	}
	for i, b := range itemBounds {
		if snapBounds[i] != liveBounds[i] {
			diffs = append(diffs, fmt.Sprintf("%s request.%s: item bounds changed (snapshot %s, live %s); the SDK checks them locally",
				b.path, b.property, snapBounds[i], liveBounds[i]))
		}
	}
	if len(diffs) == 0 {
		fmt.Fprintf(stdout, "No drift: %d request/response schemas match the snapshot.\n", len(snapshot))
		return 0
	}
	fmt.Fprintf(stderr, "Spec drift in %d place(s) (snapshot vs live):\n", len(diffs))
	for _, d := range diffs {
		fmt.Fprintf(stderr, "  %s\n", d)
	}
	fmt.Fprintln(stderr, "Review the change, update testdata/openapi.json by hand, then fix the types or testdata/known_diffs.json.")
	return 1
}

func envOr(getenv func(string) string, key, def string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return def
}

// fetch downloads an http(s) URL; anything else is read as a local file path.
func fetch(url string) ([]byte, error) {
	if !strings.HasPrefix(strings.ToLower(url), "http://") && !strings.HasPrefix(strings.ToLower(url), "https://") {
		return os.ReadFile(url)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSpecBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSpecBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxSpecBytes)
	}
	return data, nil
}

// flattenAll resolves the schema names from each endpoint, so a renamed schema is still followed.
func flattenAll(load func() ([]byte, error)) ([]flatSchema, []string, error) {
	data, err := load()
	if err != nil {
		return nil, nil, err
	}
	doc, err := spec.Parse(data)
	if err != nil {
		return nil, nil, err
	}
	var out []flatSchema
	for _, path := range endpoints {
		req, res, err := doc.PostRefs(path)
		if err != nil {
			return nil, nil, err
		}
		for _, s := range []struct {
			name string
			side spec.Side
		}{{req, spec.Request}, {res, spec.Response}} {
			obj, err := spec.Flatten(doc, s.name, s.side)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, flatSchema{path, s.side, obj})
		}
	}
	var bounds []string
	for _, b := range itemBounds {
		req, _, err := doc.PostRefs(b.path)
		if err != nil {
			return nil, nil, err
		}
		s, err := doc.ItemBounds(req, b.property)
		if err != nil {
			return nil, nil, err
		}
		bounds = append(bounds, s)
	}
	return out, bounds, nil
}

// compare reports how live differs from snapshot. Diff is not symmetric (it only flags a
// nullable spec field the SDK cannot hold), so the reverse direction and a deep comparison back it up.
func compare(snap, live flatSchema) []string {
	if reflect.DeepEqual(snap.obj, live.obj) {
		return nil
	}
	forward := strings.NewReplacer("SDK", "snapshot", "spec", "live spec", "sdk", "snapshot")
	if d := spec.Diff(snap.obj, live.obj, snap.side); len(d) > 0 {
		return relabel(d, forward)
	}
	reverse := strings.NewReplacer("SDK", "live spec", "spec", "snapshot", "sdk", "live spec")
	if d := spec.Diff(live.obj, snap.obj, snap.side); len(d) > 0 {
		return relabel(d, reverse)
	}
	return []string{": structure differs in a way the field comparison does not describe; compare the two specs"}
}

func relabel(diffs []string, r *strings.Replacer) []string {
	out := make([]string, len(diffs))
	for i, d := range diffs {
		path, msg, _ := strings.Cut(d, ": ")
		out[i] = path + ": " + r.Replace(msg)
	}
	return out
}
