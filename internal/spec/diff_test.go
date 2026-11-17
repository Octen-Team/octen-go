package spec

import (
	"reflect"
	"strings"
	"testing"
)

const reqSchemas = `{
	"Opts":{"type":"object","properties":{
		"count":{"type":"integer"},
		"mode":{"type":"string","enum":["a","b"]},
		"tags":{"type":"array","items":{"type":"object","required":["k"],"properties":{"k":{"type":"string"}}}}}},
	"Req":{"type":"object","required":["query"],"properties":{"query":{"type":"string"},"opts":{"$ref":"#/components/schemas/Opts"}}}
}`

func str(enum ...string) *Schema { return &Schema{Kind: KindString, Enum: enum} }

func obj(required []string, props map[string]*Schema) *Object {
	return &Object{Properties: props, Required: required}
}

func objSchema(o *Object) *Schema { return &Schema{Kind: KindObject, Object: o} }

// goodReq mirrors reqSchemas the way the SDK side would describe it.
func goodReq() *Object {
	return obj([]string{"query"}, map[string]*Schema{
		"query": str(),
		"opts": objSchema(obj(nil, map[string]*Schema{
			"count": {Kind: KindInteger},
			"mode":  str("b", "a"),
			"tags":  {Kind: KindArray, Items: objSchema(obj([]string{"k"}, map[string]*Schema{"k": str()}))},
		})),
	})
}

// wantOne asserts exactly one difference that contains every fragment.
func wantOne(t *testing.T, got []string, fragments ...string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("got %d differences %q, want exactly one", len(got), got)
	}
	for _, f := range fragments {
		if !strings.Contains(got[0], f) {
			t.Fatalf("difference %q does not contain %q", got[0], f)
		}
	}
}

func TestDiffRequest(t *testing.T) {
	spec := mustFlatten(t, parse(t, reqSchemas), "Req", Request)
	if got := Diff(goodReq(), spec, Request); len(got) != 0 {
		t.Fatalf("identical structure: %q", got)
	}
	opts := func(o *Object) *Object { return o.Properties["opts"].Object }

	a := goodReq()
	opts(a).Properties["topic"] = str()
	wantOne(t, Diff(a, spec, Request), ".opts.topic", MsgSDKOnly)

	a = goodReq()
	delete(opts(a).Properties, "count")
	wantOne(t, Diff(a, spec, Request), ".opts.count", "in spec but not in SDK")

	a = goodReq()
	opts(a).Properties["count"] = &Schema{Kind: KindNumber}
	wantOne(t, Diff(a, spec, Request), ".opts.count", "type mismatch")

	a = goodReq()
	opts(a).Properties["mode"] = str("a", "c")
	wantOne(t, Diff(a, spec, Request), ".opts.mode", "enum mismatch")

	a = goodReq()
	opts(a).Properties["mode"] = str()
	wantOne(t, Diff(a, spec, Request), ".opts.mode", "enum mismatch")

	a = goodReq()
	a.Required = nil
	wantOne(t, Diff(a, spec, Request), ".query", "required mismatch")

	a = goodReq()
	opts(a).Properties["tags"].Items.Object.Required = nil
	wantOne(t, Diff(a, spec, Request), ".opts.tags[].k", "required mismatch")

	a = goodReq()
	opts(a).Properties["tags"].Items.Object.Properties["v"] = str()
	wantOne(t, Diff(a, spec, Request), ".opts.tags[].v", MsgSDKOnly)

	a = goodReq()
	opts(a).Properties["count"] = &Schema{Kind: KindRaw, Nullable: true}
	wantOne(t, Diff(a, spec, Request), ".opts.count", "type mismatch")

	a = goodReq()
	opts(a).Properties["tags"] = str()
	wantOne(t, Diff(a, spec, Request), ".opts.tags", "type mismatch")
}

const resSchemas = `{
	"Company":{"type":"object","required":["type"],"properties":{"type":{"type":"string","enum":["company"]},"name":{"type":"string"}}},
	"Person":{"type":"object","required":["type"],"properties":{"type":{"type":"string","enum":["person"]},"title":{"type":"string"}}},
	"BusinessEntity":{"oneOf":[{"$ref":"#/components/schemas/Company"},{"$ref":"#/components/schemas/Person"}],
		"discriminator":{"propertyName":"type","mapping":{"company":"#/components/schemas/Company","person":"#/components/schemas/Person"}}},
	"Res":{"type":"object","required":["request_id"],"properties":{
		"code":{"type":"integer"},
		"request_id":{"type":"string"},
		"data":{"type":"object","properties":{
			"warning":{"type":"string","nullable":true},
			"latency":{"type":"number"},
			"count":{"type":"integer"},
			"attrs":{"type":"object"},
			"results":{"type":"array","items":{"type":"object","properties":{"title":{"type":"string"}}}},
			"entities":{"type":"array","items":{"$ref":"#/components/schemas/BusinessEntity"}}}}}}
}`

func goodRes() *Object {
	return obj(nil, map[string]*Schema{
		"code":       {Kind: KindRaw, Nullable: true},
		"request_id": {Kind: KindString},
		"data": {Kind: KindObject, Nullable: true, Object: obj(nil, map[string]*Schema{
			"warning": {Kind: KindString, Nullable: true},
			"latency": {Kind: KindNumber},
			"count":   {Kind: KindInteger},
			"attrs":   {Kind: KindObject, Nullable: true, Object: &Object{FreeForm: true}},
			"results": {Kind: KindArray, Nullable: true, Items: objSchema(obj(nil, map[string]*Schema{"title": str()}))},
			"entities": {Kind: KindArray, Nullable: true, Items: &Schema{Kind: KindUnion, Tag: "type", Variants: map[string]*Object{
				"company": obj(nil, map[string]*Schema{"type": str(), "name": str()}),
				"person":  obj(nil, map[string]*Schema{"type": str(), "title": str()}),
			}}},
		})},
	})
}

func TestDiffResponse(t *testing.T) {
	spec := mustFlatten(t, parse(t, resSchemas), "Res", Response)
	if got := Diff(goodRes(), spec, Response); len(got) != 0 {
		t.Fatalf("matching structure: %q", got)
	}
	data := func(o *Object) *Object { return o.Properties["data"].Object }

	a := goodRes()
	delete(data(a).Properties["results"].Items.Object.Properties, "title")
	wantOne(t, Diff(a, spec, Response), ".data.results[].title", "in spec but not in SDK")

	a = goodRes()
	a.Properties["msg"] = str()
	wantOne(t, Diff(a, spec, Response), ".msg", MsgSDKOnly)

	// A wrong scalar kind makes Go decoding fail, so responses compare kinds too.
	a = goodRes()
	data(a).Properties["latency"] = &Schema{Kind: KindInteger}
	wantOne(t, Diff(a, spec, Response), ".data.latency", "type mismatch (sdk integer, spec number)")

	a = goodRes()
	data(a).Properties["count"] = str()
	wantOne(t, Diff(a, spec, Response), ".data.count", "type mismatch (sdk string, spec integer)")

	a = goodRes()
	data(a).Properties["results"].Items.Object.Properties["title"] = &Schema{Kind: KindBoolean}
	wantOne(t, Diff(a, spec, Response), ".data.results[].title", "type mismatch (sdk boolean, spec string)")

	// A float64 decodes any JSON integer, so an SDK number on a spec integer is fine.
	a = goodRes()
	data(a).Properties["count"] = &Schema{Kind: KindNumber}
	if got := Diff(a, spec, Response); len(got) != 0 {
		t.Errorf("number on integer: %q", got)
	}

	a = goodRes()
	data(a).Properties["warning"] = str()
	wantOne(t, Diff(a, spec, Response), ".data.warning", "null")

	a = goodRes()
	delete(data(a).Properties["entities"].Items.Variants["person"].Properties, "title")
	wantOne(t, Diff(a, spec, Response), ".data.entities[]{person}.title", "in spec but not in SDK")

	a = goodRes()
	delete(data(a).Properties["entities"].Items.Variants, "person")
	wantOne(t, Diff(a, spec, Response), ".data.entities[]{person}", "variant in spec but not in SDK")

	a = goodRes()
	data(a).Properties["entities"].Items.Variants["robot"] = obj(nil, map[string]*Schema{"type": str()})
	wantOne(t, Diff(a, spec, Response), ".data.entities[]{robot}", MsgSDKVariantOnly)

	a = goodRes()
	data(a).Properties["entities"].Items.Tag = "kind"
	wantOne(t, Diff(a, spec, Response), ".data.entities[]", "tag mismatch")

	a = goodRes()
	data(a).Properties["results"] = str()
	wantOne(t, Diff(a, spec, Response), ".data.results", "type mismatch")

	a = goodRes()
	data(a).Properties["attrs"] = objSchema(obj(nil, map[string]*Schema{"wifi": {Kind: KindBoolean}}))
	wantOne(t, Diff(a, spec, Response), ".data.attrs", "free-form")

	// Raw matches scalars only, never a structured spec node.
	a = goodRes()
	a.Properties["data"] = &Schema{Kind: KindRaw, Nullable: true}
	wantOne(t, Diff(a, spec, Response), ".data", "type mismatch")
}

func TestParseKnownDiffs(t *testing.T) {
	got, err := ParseKnownDiffs([]byte(`[{"path":"A.b.c","reason":"why"}]`))
	if err != nil || !reflect.DeepEqual(got, []KnownDiff{{Path: "A.b.c", Reason: "why"}}) {
		t.Fatalf("ParseKnownDiffs = %+v, %v", got, err)
	}
	bad := map[string]string{
		`[{"path":"A.b","reason":" "}]`:                             "reason",
		`[{"path":"A.b"}]`:                                          "reason",
		`[{"path":"nodot","reason":"r"}]`:                           "path",
		`[{"path":"A.b","reason":"r"},{"path":"A.b","reason":"r"}]`: "duplicate",
		`{}`:                        "array",
		`[1]`:                       "object",
		`[{"path":5,"reason":"r"}]`: "path",
	}
	for in, want := range bad {
		if _, err := ParseKnownDiffs([]byte(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseKnownDiffs(%s) error = %v, want it to mention %q", in, err, want)
		}
	}
}

func TestApplyKnownDiffs(t *testing.T) {
	known := []KnownDiff{{Path: "Req.opts.max_reviews", Reason: "r"}, {Path: "Other.x", Reason: "r"}}
	diffs := []string{".opts.max_reviews: " + MsgSDKOnly, ".opts.count: in spec but not in SDK"}
	got := ApplyKnownDiffs("Req", diffs, known)
	if !reflect.DeepEqual(got, []string{"Req.opts.count: in spec but not in SDK"}) {
		t.Errorf("ApplyKnownDiffs = %q", got)
	}
	// An entry for this root that no longer matches an SDK-only field is itself a failure.
	got = ApplyKnownDiffs("Req", nil, known)
	wantOne(t, got, "Req.opts.max_reviews", "known_diffs")
	// Known entries never excuse fields missing from the SDK.
	got = ApplyKnownDiffs("Req", []string{".opts.max_reviews: in spec but not in SDK"}, known)
	if len(got) != 2 {
		t.Errorf("ApplyKnownDiffs = %q, want the missing field and the stale entry", got)
	}
	// Variant-level entries are honoured too.
	got = ApplyKnownDiffs("Res", []string{".e[]{robot}: " + MsgSDKVariantOnly}, []KnownDiff{{Path: "Res.e[]{robot}", Reason: "r"}})
	if len(got) != 0 {
		t.Errorf("variant entry not applied: %q", got)
	}
}
