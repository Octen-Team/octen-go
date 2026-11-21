package spec

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

const snapshotPath = "../../testdata/openapi.json"

func loadSnapshot(t *testing.T) *Doc {
	t.Helper()
	doc, err := Load(snapshotPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return doc
}

func parse(t *testing.T, schemas string) *Doc {
	t.Helper()
	doc, err := Parse([]byte(`{"components":{"schemas":` + schemas + `}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc
}

func mustFlatten(t *testing.T, doc *Doc, name string, side Side) *Object {
	t.Helper()
	obj, err := Flatten(doc, name, side)
	if err != nil {
		t.Fatalf("Flatten(%s): %v", name, err)
	}
	return obj
}

func wantErr(t *testing.T, doc *Doc, name string, side Side, substr string) {
	t.Helper()
	_, err := Flatten(doc, name, side)
	if err == nil {
		t.Fatalf("Flatten(%s) succeeded, want error containing %q", name, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("Flatten(%s) error = %q, want it to contain %q", name, err, substr)
	}
}

func keys(m map[string]*Schema) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestFlattenAllOfTopLevelAndProperty(t *testing.T) {
	doc := loadSnapshot(t)
	s := mustFlatten(t, doc, "SearchRequest", Request)
	if s.Properties["include_domains"] == nil {
		t.Errorf("SearchRequest lacks include_domains from top-level allOf WebSearchOptions; got %v", keys(s.Properties))
	}
	if !reflect.DeepEqual(s.Required, []string{"query"}) {
		t.Errorf("SearchRequest required = %v, want [query]", s.Required)
	}
	b := mustFlatten(t, doc, "BroadSearchRequest", Request)
	opts := b.Properties["search_options"]
	if opts == nil || opts.Kind != KindObject || opts.Object.Properties["count"] == nil {
		t.Fatalf("BroadSearchRequest.search_options did not expand its property-level allOf: %+v", opts)
	}
}

func TestFlattenRejectsUnknownKeywordsInRequests(t *testing.T) {
	doc := parse(t, `{"X":{"type":"object","properties":{"a":{"oneOf":[{"type":"string"},{"type":"integer"}]}}}}`)
	wantErr(t, doc, "X", Request, "oneOf")
	for _, kw := range []string{"anyOf", "not", "additionalProperties", "const", "patternProperties"} {
		doc := parse(t, `{"X":{"type":"object","properties":{"a":{"type":"string","`+kw+`":{}}}}}`)
		wantErr(t, doc, "X", Request, kw)
	}
	// The real BusinessEntity union is a response-only construct.
	wantErr(t, loadSnapshot(t), "BusinessSearchResponse", Request, "oneOf")
}

func TestRequiredMergedAcrossAllOf(t *testing.T) {
	doc := parse(t, `{
		"A":{"type":"object","required":["a"],"properties":{"a":{"type":"string"}}},
		"X":{"type":"object","required":["b"],"properties":{"b":{"type":"integer"}},"allOf":[{"$ref":"#/components/schemas/A"}]}
	}`)
	x := mustFlatten(t, doc, "X", Request)
	if !reflect.DeepEqual(x.Required, []string{"a", "b"}) {
		t.Errorf("required = %v, want [a b]", x.Required)
	}
	if got := keys(x.Properties); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("properties = %v, want [a b]", got)
	}
}

func TestFlattenResponseRejectsUnlistedUnions(t *testing.T) {
	doc := parse(t, `{
		"A":{"type":"object","properties":{"type":{"type":"string"}}},
		"X":{"type":"object","properties":{"u":{"oneOf":[{"$ref":"#/components/schemas/A"}],
			"discriminator":{"propertyName":"type","mapping":{"a":"#/components/schemas/A"}}}}}
	}`)
	wantErr(t, doc, "X", Response, "oneOf")
	wantErr(t, parse(t, `{"X":{"type":"object","additionalProperties":true}}`), "X", Response, "free-form")
	wantErr(t, parse(t, `{"X":{"type":"object","additionalProperties":{"type":"string"}}}`), "X", Response, "additionalProperties")
	wantErr(t, parse(t, `{"X":{"type":"object","properties":{"a":{"anyOf":[]}}}}`), "X", Response, "anyOf")
}

func TestFlattenBusinessEntityUnion(t *testing.T) {
	f := mustFlatten(t, loadSnapshot(t), "BusinessSearchResponse", Response)
	entities := f.Properties["data"].Object.Properties["entities"]
	if entities == nil || entities.Kind != KindArray || entities.Items.Kind != KindUnion {
		t.Fatalf("entities = %+v, want array of union", entities)
	}
	u := entities.Items
	if u.Tag != "type" {
		t.Errorf("tag = %q, want type", u.Tag)
	}
	if u.Variants["company"] == nil || u.Variants["company"].Properties["key_people"] == nil {
		t.Errorf("company variant lacks key_people")
	}
	if p := u.Variants["person"]; p == nil || !contains(p.Required, "type") {
		t.Errorf("person variant does not require type")
	}
	if len(u.Variants) != 2 {
		t.Errorf("variants = %d, want 2", len(u.Variants))
	}
}

func TestFlattenNullable(t *testing.T) {
	f := mustFlatten(t, loadSnapshot(t), "SearchResponse", Response)
	w := f.Properties["meta"].Object.Properties["warning"]
	if w.Kind != KindString || !w.Nullable {
		t.Errorf("SearchResponse.meta.warning = %+v, want nullable string", w)
	}
	x := mustFlatten(t, parse(t, `{"X":{"type":"object","properties":{"a":{"type":["string","null"]}}}}`), "X", Request)
	if a := x.Properties["a"]; a.Kind != KindString || !a.Nullable {
		t.Errorf("3.1 type array = %+v, want nullable string", a)
	}
	wantErr(t, parse(t, `{"X":{"type":"object","properties":{"a":{"type":["string","integer"]}}}}`), "X", Request, "unsupported type")
}

func TestFlattenKeepsTypesAndEnumsIgnoresAnnotations(t *testing.T) {
	f := mustFlatten(t, loadSnapshot(t), "SearchRequest", Request)
	want := &Schema{Kind: KindString, Enum: []string{"day", "week", "month", "year", "d", "w", "m", "y"}}
	if got := f.Properties["time_range"]; !reflect.DeepEqual(got, want) {
		t.Errorf("time_range = %+v, want %+v", got, want)
	}
	if got := f.Properties["count"]; !reflect.DeepEqual(got, &Schema{Kind: KindInteger}) {
		t.Errorf("count = %+v, want plain integer", got)
	}
	if l := f.Properties["language"]; l.Kind != KindArray || l.Items.Kind != KindString || len(l.Items.Enum) == 0 {
		t.Errorf("language = %+v, want array of string enum", l)
	}
	doc := parse(t, `{"X":{"type":"object","title":"t","description":"d","x-internal":true,"properties":{
		"a":{"type":"integer","format":"int64","minimum":1,"maximum":9,"default":3,"example":4,"examples":[4],"deprecated":true,"externalDocs":{}}}}}`)
	if got := mustFlatten(t, doc, "X", Request).Properties["a"]; !reflect.DeepEqual(got, &Schema{Kind: KindInteger}) {
		t.Errorf("annotated integer = %+v, want plain integer", got)
	}
}

func TestFlattenStructuralErrors(t *testing.T) {
	cases := []struct {
		name, schemas, want string
		side                Side
	}{
		{"free-form request object", `{"X":{"type":"object","properties":{"a":{"type":"object"}}}}`, "properties", Request},
		{"missing ref target", `{"X":{"$ref":"#/components/schemas/Nope"}}`, "Nope", Request},
		{"direct cycle", `{"X":{"type":"object","properties":{"self":{"$ref":"#/components/schemas/X"}}}}`, "cycle", Request},
		{"indirect cycle", `{"B":{"type":"object","properties":{"c":{"$ref":"#/components/schemas/B"}}},
			"X":{"type":"object","properties":{"b":{"$ref":"#/components/schemas/B"}}}}`, "cycle", Request},
		{"conflicting allOf property", `{"A":{"type":"object","properties":{"a":{"type":"string"}}},
			"X":{"type":"object","properties":{"a":{"type":"integer"}},"allOf":[{"$ref":"#/components/schemas/A"}]}}`, "conflict", Request},
		{"required not a property", `{"X":{"type":"object","required":["z"],"properties":{"a":{"type":"string"}}}}`, "required", Request},
		{"non-string enum", `{"X":{"type":"object","properties":{"a":{"type":"integer","enum":[1,2]}}}}`, "enum", Request},
		{"array without items", `{"X":{"type":"object","properties":{"a":{"type":"array"}}}}`, "items", Request},
		{"missing type", `{"X":{"type":"object","properties":{"a":{"description":"?"}}}}`, "type", Response},
		{"external ref", `{"X":{"$ref":"other.json#/X"}}`, "$ref", Request},
		{"root not object", `{"X":{"type":"string"}}`, "not an object", Request},
		{"unknown root", `{}`, "X", Request},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { wantErr(t, parse(t, c.schemas), "X", c.side, c.want) })
	}
}

func TestFlattenSiblingReuseIsNotACycle(t *testing.T) {
	doc := parse(t, `{"B":{"type":"object","properties":{"v":{"type":"string"}}},
		"X":{"type":"object","properties":{"a":{"$ref":"#/components/schemas/B"},"b":{"$ref":"#/components/schemas/B"}}}}`)
	if got := keys(mustFlatten(t, doc, "X", Request).Properties); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("properties = %v, want [a b]", got)
	}
}

func TestFlattenResponseFreeFormObject(t *testing.T) {
	f := mustFlatten(t, loadSnapshot(t), "PlaceSearchResponse", Response)
	attrs := f.Properties["data"].Object.Properties["places"].Items.Object.Properties["attributes"]
	if attrs.Kind != KindObject || !attrs.Object.FreeForm {
		t.Errorf("Place.attributes = %+v, want free-form object", attrs)
	}
}

// "additionalProperties": true on an object without properties is the explicit free-form form.
func TestFlattenAdditionalPropertiesTrueIsFreeForm(t *testing.T) {
	f := mustFlatten(t, loadSnapshot(t), "DesignSearchResponse", Response)
	img := f.Properties["data"].Object.Properties["images"].Items.Object
	for _, s := range []*Schema{img.Properties["design_facts"], img.Properties["group"].Object.Properties["master"]} {
		if s.Kind != KindObject || !s.Object.FreeForm {
			t.Errorf("got %+v, want free-form object", s)
		}
	}
	for name, schemas := range map[string]string{
		"schema-valued":     `{"X":{"type":"object","properties":{"m":{"type":"object","additionalProperties":{"type":"string"}}}}}`,
		"false":             `{"X":{"type":"object","properties":{"m":{"type":"object","additionalProperties":false}}}}`,
		"beside properties": `{"X":{"type":"object","properties":{"m":{"type":"object","additionalProperties":true,"properties":{"a":{"type":"string"}}}}}}`,
		"on a string":       `{"X":{"type":"object","properties":{"m":{"type":"string","additionalProperties":true}}}}`,
		"without a type":    `{"X":{"type":"object","properties":{"m":{"additionalProperties":true}}}}`,
	} {
		t.Run(name, func(t *testing.T) { wantErr(t, parse(t, schemas), "X", Response, "additionalProperties") })
	}
	wantErr(t, parse(t, `{"X":{"type":"object","properties":{"m":{"type":"object","additionalProperties":true}}}}`), "X", Request, "free-form")
}

func TestEndpointSchemasFlatten(t *testing.T) {
	doc := loadSnapshot(t)
	for _, name := range []string{"Search", "BroadSearch", "NewsSearch", "BusinessSearch", "PulseSearch", "PlaceSearch", "ImageSearch", "VideoSearch", "DesignSearch"} {
		if _, err := Flatten(doc, name+"Request", Request); err != nil {
			t.Errorf("%sRequest: %v", name, err)
		}
		if _, err := Flatten(doc, name+"Response", Response); err != nil {
			t.Errorf("%sResponse: %v", name, err)
		}
	}
}

func TestDocRefs(t *testing.T) {
	doc := loadSnapshot(t)
	req, res, err := doc.PostRefs("/search")
	if err != nil || req != "SearchRequest" || res != "SearchResponse" {
		t.Errorf("PostRefs(/search) = %q, %q, %v", req, res, err)
	}
	if _, _, err := doc.PostRefs("/nope"); err == nil {
		t.Error("PostRefs(/nope) succeeded")
	}
	if got, err := doc.PropertyRef("BroadSearchResponse", "meta"); err != nil || got != "SearchMeta" {
		t.Errorf("PropertyRef(BroadSearchResponse.meta) = %q, %v", got, err)
	}
	if _, err := doc.PropertyRef("SearchResponse", "request_id"); err == nil {
		t.Error("PropertyRef on a non-$ref property succeeded")
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
