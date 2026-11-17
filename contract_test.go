package octen

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Octen-Team/octen-go/internal/spec"
)

type contractCase struct {
	path    string
	reqName string
	req     reflect.Type
	resName string
	res     reflect.Type
}

var contractCases = []contractCase{
	{"/search", "SearchRequest", reflect.TypeOf(SearchParams{}), "SearchResponse", reflect.TypeOf(SearchResponse{})},
	{"/broad-search", "BroadSearchRequest", reflect.TypeOf(BroadSearchParams{}), "BroadSearchResponse", reflect.TypeOf(BroadSearchResponse{})},
	{"/news-search", "NewsSearchRequest", reflect.TypeOf(NewsSearchParams{}), "NewsSearchResponse", reflect.TypeOf(NewsSearchResponse{})},
	{"/business-search", "BusinessSearchRequest", reflect.TypeOf(BusinessSearchParams{}), "BusinessSearchResponse", reflect.TypeOf(BusinessSearchResponse{})},
	{"/pulse-search", "PulseSearchRequest", reflect.TypeOf(PulseSearchParams{}), "PulseSearchResponse", reflect.TypeOf(PulseSearchResponse{})},
	{"/place-search", "PlaceSearchRequest", reflect.TypeOf(PlaceSearchParams{}), "PlaceSearchResponse", reflect.TypeOf(PlaceSearchResponse{})},
}

// goUnion describes a Go type that decodes a discriminated OpenAPI oneOf.
type goUnion struct {
	tag      string
	variants map[string]reflect.Type
}

var goUnions = map[reflect.Type]goUnion{
	reflect.TypeOf(BusinessEntity{}): {tag: "type", variants: map[string]reflect.Type{
		"company": reflect.TypeOf(CompanyEntity{}),
		"person":  reflect.TypeOf(PersonEntity{}),
	}},
}

var rawMessageType = reflect.TypeOf(json.RawMessage(nil))

func loadContractDoc(t *testing.T) *spec.Doc {
	t.Helper()
	doc, err := spec.Load("testdata/openapi.json")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	return doc
}

func loadKnownDiffs(t *testing.T) []spec.KnownDiff {
	t.Helper()
	data, err := os.ReadFile("testdata/known_diffs.json")
	if err != nil {
		t.Fatal(err)
	}
	known, err := spec.ParseKnownDiffs(data)
	if err != nil {
		t.Fatal(err)
	}
	return known
}

func TestContract(t *testing.T) {
	doc := loadContractDoc(t)
	known := loadKnownDiffs(t)
	check := func(t *testing.T, name string, typ reflect.Type, side spec.Side) {
		flat, err := spec.Flatten(doc, name, side)
		if err != nil {
			t.Fatal(err)
		}
		d := &describer{side: side}
		sdk := d.object(typ, name, nil)
		problems := append(d.problems, spec.ApplyKnownDiffs(name, spec.Diff(sdk, flat, side), known)...)
		if side == spec.Response {
			problems = append(problems, checkEnvelope(doc, name, typ)...)
		}
		for _, p := range problems {
			t.Error(p)
		}
	}
	for _, c := range contractCases {
		t.Run(c.reqName, func(t *testing.T) { check(t, c.reqName, c.req, spec.Request) })
		t.Run(c.resName, func(t *testing.T) { check(t, c.resName, c.res, spec.Response) })
	}
}

// TestEndpointPaths ties each compared schema pair to the path its operation is served on.
func TestEndpointPaths(t *testing.T) {
	doc := loadContractDoc(t)
	for _, c := range contractCases {
		req, res, err := doc.PostRefs(c.path)
		if err != nil {
			t.Errorf("%s: %v", c.path, err)
			continue
		}
		if req != c.reqName || res != c.resName {
			t.Errorf("%s: snapshot refs %s / %s, test compares %s / %s", c.path, req, res, c.reqName, c.resName)
		}
	}
}

// A known_diffs entry whose root is never compared would never be checked, so it could go stale silently.
func TestKnownDiffsRootsAreCompared(t *testing.T) {
	compared := map[string]bool{}
	for _, c := range contractCases {
		compared[c.reqName], compared[c.resName] = true, true
	}
	known := loadKnownDiffs(t)
	if len(known) == 0 {
		t.Fatal("known_diffs.json is empty; the stale-entry check would be vacuous")
	}
	for _, k := range known {
		if root := strings.SplitN(k.Path, ".", 2)[0]; !compared[root] {
			t.Errorf("%s: root schema %s is not compared by TestContract", k.Path, root)
		}
	}
}

// The union registry must agree with BusinessEntity.UnmarshalJSON, or the contract compares the wrong variants.
func TestUnionRegistryMatchesDecoder(t *testing.T) {
	for typ, u := range goUnions {
		var variantFields []string
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.Ptr && f.Type.Elem().Kind() == reflect.Struct {
				variantFields = append(variantFields, f.Name)
			}
		}
		if len(variantFields) != len(u.variants) {
			t.Errorf("%s has variant fields %v, registry lists %d variants", typ, variantFields, len(u.variants))
		}
		for tagValue, vt := range u.variants {
			v := reflect.New(typ)
			body := fmt.Sprintf(`{%q:%q}`, u.tag, tagValue)
			if err := json.Unmarshal([]byte(body), v.Interface()); err != nil {
				t.Fatalf("%s: decode %s: %v", typ, body, err)
			}
			for _, name := range variantFields {
				f := v.Elem().FieldByName(name)
				if set, want := !f.IsNil(), f.Type().Elem() == vt; set != want {
					t.Errorf("%s: decoding %s set %s = %v, want %v", typ, body, name, set, want)
				}
			}
		}
	}
}

// describer turns a Go type into the flattened form, enforcing the SDK's field-shape rules on the way.
type describer struct {
	side     spec.Side
	problems []string
}

func (d *describer) fail(path, format string, args ...any) {
	d.problems = append(d.problems, path+": "+fmt.Sprintf(format, args...))
}

func (d *describer) object(t reflect.Type, path string, stack []reflect.Type) *spec.Object {
	for _, s := range stack {
		if s == t {
			d.fail(path, "type cycle through %s", t)
			return &spec.Object{Properties: map[string]*spec.Schema{}}
		}
	}
	stack = append(stack[:len(stack):len(stack)], t)
	obj := &spec.Object{Properties: map[string]*spec.Schema{}}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag, hasTag := f.Tag.Lookup("json")
		name, opts, _ := strings.Cut(tag, ",")
		if !hasTag || name == "" || name == "-" || f.Anonymous {
			d.fail(path+"."+f.Name, "exported field needs an explicit json name")
			continue
		}
		p := path + "." + name
		if _, dup := obj.Properties[name]; dup {
			d.fail(p, "json name used twice")
		}
		if want := goFieldName(name); f.Name != want {
			d.fail(p, "Go field is %s, want %s", f.Name, want)
		}
		omitempty := hasOption(opts, "omitempty")
		if d.side == spec.Request {
			k := f.Type.Kind()
			optionalShape := k == reflect.Ptr || k == reflect.Slice
			switch {
			case optionalShape && !omitempty:
				d.fail(p, "optional request field needs omitempty")
			case !optionalShape && omitempty:
				d.fail(p, "optional request field must be a pointer or a slice")
			case !optionalShape:
				obj.Required = append(obj.Required, name)
			}
		}
		enum, hasEnum := f.Tag.Lookup("enum")
		obj.Properties[name] = d.schema(f.Type, p, enum, hasEnum, stack)
	}
	sort.Strings(obj.Required)
	return obj
}

func (d *describer) schema(t reflect.Type, path, enum string, hasEnum bool, stack []reflect.Type) *spec.Schema {
	if t == rawMessageType {
		d.noEnum(path, hasEnum)
		return &spec.Schema{Kind: spec.KindRaw, Nullable: true}
	}
	nullable := false
	if t.Kind() == reflect.Ptr {
		nullable, t = true, t.Elem()
	}
	if u, ok := goUnions[t]; ok {
		d.noEnum(path, hasEnum)
		s := &spec.Schema{Kind: spec.KindUnion, Nullable: nullable, Tag: u.tag, Variants: map[string]*spec.Object{}}
		for v, vt := range u.variants {
			s.Variants[v] = d.object(vt, path+"{"+v+"}", stack)
		}
		return s
	}
	s := &spec.Schema{Nullable: nullable}
	switch t.Kind() {
	case reflect.String:
		s.Kind = spec.KindString
		if hasEnum {
			if d.side == spec.Response {
				d.fail(path, "response fields must not carry an enum tag")
			} else {
				s.Enum = strings.Split(enum, ",")
			}
		}
		return s
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		s.Kind = spec.KindInteger
	case reflect.Float32, reflect.Float64:
		s.Kind = spec.KindNumber
	case reflect.Bool:
		s.Kind = spec.KindBoolean
	case reflect.Slice:
		// An enum tag on a slice lists the allowed item values.
		return &spec.Schema{Kind: spec.KindArray, Nullable: true, Items: d.schema(t.Elem(), path+"[]", enum, hasEnum, stack)}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			d.fail(path, "map keys must be strings")
		}
		s.Kind, s.Nullable, s.Object = spec.KindObject, true, &spec.Object{FreeForm: true}
	case reflect.Struct:
		s.Kind, s.Object = spec.KindObject, d.object(t, path, stack)
	default:
		d.fail(path, "unsupported Go type %s", t)
		s.Kind = spec.KindRaw
	}
	if hasEnum {
		d.fail(path, "enum tag on a %s field", t)
	}
	return s
}

func (d *describer) noEnum(path string, hasEnum bool) {
	if hasEnum {
		d.fail(path, "enum tag on a field that is not a string")
	}
}

func hasOption(opts, want string) bool {
	for _, o := range strings.Split(opts, ",") {
		if o == want {
			return true
		}
	}
	return false
}

// goNames are the json name parts that are not simply capitalised in Go field names.
var goNames = map[string]string{"id": "ID", "url": "URL", "hq": "HQ", "cik": "CIK", "linkedin": "LinkedIn"}

func goFieldName(jsonName string) string {
	var b strings.Builder
	for _, part := range strings.Split(jsonName, "_") {
		if n, ok := goNames[part]; ok {
			b.WriteString(n)
		} else if part != "" {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}

// checkEnvelope enforces the fixed envelope every response type shares.
func checkEnvelope(doc *spec.Doc, name string, typ reflect.Type) []string {
	var out []string
	want := []struct {
		field, tag string
		typ        reflect.Type
		ref        string
	}{
		{"Code", "code,omitempty", rawMessageType, ""},
		{"Msg", "msg,omitempty", reflect.TypeOf((*string)(nil)), ""},
		{"RequestID", "request_id", reflect.TypeOf(""), ""},
		{"Data", "data,omitempty", nil, "data"},
		{"Meta", "meta,omitempty", nil, "meta"},
	}
	for _, w := range want {
		f, ok := typ.FieldByName(w.field)
		if !ok {
			out = append(out, fmt.Sprintf("%s: envelope field %s missing", name, w.field))
			continue
		}
		if got := f.Tag.Get("json"); got != w.tag {
			out = append(out, fmt.Sprintf("%s.%s: json tag %q, want %q", name, w.field, got, w.tag))
		}
		if w.ref == "" {
			if f.Type != w.typ {
				out = append(out, fmt.Sprintf("%s.%s: type %s, want %s", name, w.field, f.Type, w.typ))
			}
			continue
		}
		ref, err := doc.PropertyRef(name, w.ref)
		if err != nil {
			out = append(out, fmt.Sprintf("%s.%s: %v", name, w.field, err))
			continue
		}
		if f.Type.Kind() != reflect.Ptr || f.Type.Elem().Name() != ref {
			out = append(out, fmt.Sprintf("%s.%s: type %s, want *%s", name, w.field, f.Type, ref))
		}
	}
	if f, ok := typ.FieldByName("raw"); !ok || f.Type != reflect.TypeOf([]byte(nil)) {
		out = append(out, fmt.Sprintf("%s: unexported raw []byte field missing", name))
	}
	m, ok := reflect.PointerTo(typ).MethodByName("RawJSON")
	if !ok || m.Type.NumIn() != 1 || m.Type.NumOut() != 1 || m.Type.Out(0) != reflect.TypeOf([]byte(nil)) {
		out = append(out, fmt.Sprintf("%s: RawJSON() []byte missing", name))
	}
	return out
}

// The describer's shape rules are guards; prove each one reports.
func TestDescriberShapeRules(t *testing.T) {
	type badReq struct {
		Query     string  `json:"query,omitempty"`
		Count     *int    `json:"count"`
		Mode      *int    `json:"mode,omitempty" enum:"a,b"`
		WrongName *string `json:"right_name,omitempty"`
		NoTag     *string
		Tags      []string `json:"tags,omitempty" enum:"x,y"`
	}
	type badRes struct {
		Status string `json:"status,omitempty" enum:"open"`
	}
	cases := []struct {
		typ  reflect.Type
		side spec.Side
		want []string
	}{
		{reflect.TypeOf(badReq{}), spec.Request, []string{
			"R.query: optional request field must be a pointer or a slice",
			"R.count: optional request field needs omitempty",
			"R.mode: enum tag on a int field",
			"R.right_name: Go field is WrongName, want RightName",
			"R.NoTag: exported field needs an explicit json name",
		}},
		{reflect.TypeOf(badRes{}), spec.Response, []string{"R.status: response fields must not carry an enum tag"}},
	}
	for _, c := range cases {
		d := &describer{side: c.side}
		obj := d.object(c.typ, "R", nil)
		sort.Strings(d.problems)
		sort.Strings(c.want)
		if !reflect.DeepEqual(d.problems, c.want) {
			t.Errorf("%s problems:\n got %q\nwant %q", c.typ, d.problems, c.want)
		}
		if c.side == spec.Request {
			if got := obj.Properties["tags"].Items.Enum; !reflect.DeepEqual(got, []string{"x", "y"}) {
				t.Errorf("slice enum tag went to items as %v", got)
			}
			if len(obj.Required) != 0 {
				t.Errorf("required = %v, want none", obj.Required)
			}
		}
	}
	if got := goFieldName("linkedin_url"); got != "LinkedInURL" {
		t.Errorf("goFieldName(linkedin_url) = %s", got)
	}
}
