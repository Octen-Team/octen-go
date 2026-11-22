// Command specdrift compares the SDK endpoints' schemas and the request values the SDK relies on
// (localChecks) in the live OpenAPI spec with testdata/openapi.json. Exit codes: 0 no drift, 1 drift,
// 2 cannot run. It never rewrites the snapshot, which is updated by hand after a drift is reviewed.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
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
	"/search-premier", "/image-search", "/video-search", "/design-search", "/extract",
}

// localCheck reads a request value that Flatten ignores (counts, lengths, ranges, descriptions)
// but the SDK relies on, so it is compared separately.
type localCheck struct {
	path, property, what, why string
	read                      func(doc *spec.Doc, schema string) (string, error)
}

const checkedLocally = "the SDK checks them locally"

var localChecks = []localCheck{
	{"/image-search", "inputs", "item bounds", checkedLocally, itemBounds("inputs")},
	{"/video-search", "inputs", "item bounds", checkedLocally, itemBounds("inputs")},
	{"/extract", "urls", "item bounds", checkedLocally, itemBounds("urls")},
	// The spec states the limits of urls (20 URLs, 2048 characters each) only in this text.
	{"/extract", "urls", "description", "it states the URL limits the SDK checks locally", func(d *spec.Doc, s string) (string, error) {
		text, err := d.PropertyDescription(s, "urls")
		return strconv.Quote(text), err
	}},
	{"/extract", "urls[]", "item length bounds", checkedLocally, func(d *spec.Doc, s string) (string, error) {
		return d.PropertyKeywords(s, "urls", true, "minLength", "maxLength")
	}},
	{"/extract", "timeout", "default and range", "the SDK derives the default HTTP timeout of Extract from them", func(d *spec.Doc, s string) (string, error) {
		return d.PropertyKeywords(s, "timeout", false, "default", "minimum", "maximum")
	}},
}

func itemBounds(property string) func(*spec.Doc, string) (string, error) {
	return func(d *spec.Doc, schema string) (string, error) { return d.ItemBounds(schema, property) }
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

	snapshot, snapValues, err := flattenAll(func() ([]byte, error) { return os.ReadFile(snapshotPath) })
	if err != nil {
		fmt.Fprintf(stderr, "specdrift: cannot read the snapshot (%s): %v\n", snapshotPath, err)
		return 2
	}
	live, liveValues, err := flattenAll(func() ([]byte, error) { return fetch(url) })
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
	for i, lc := range localChecks {
		if snapValues[i] != liveValues[i] {
			diffs = append(diffs, fmt.Sprintf("%s request.%s: %s changed (snapshot %s, live %s); %s",
				lc.path, lc.property, lc.what, snapValues[i], liveValues[i], lc.why))
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
	var values []string
	for _, lc := range localChecks {
		req, _, err := doc.PostRefs(lc.path)
		if err != nil {
			return nil, nil, err
		}
		v, err := lc.read(doc, req)
		if err != nil {
			return nil, nil, err
		}
		values = append(values, v)
	}
	return out, values, nil
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
