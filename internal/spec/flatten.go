// Package spec flattens the OpenAPI snapshot into the minimal tree the contract test compares.
// It is test infrastructure and must not import the octen package.
package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Side selects the comparison rules: requests are strict, responses allow the registered unions.
type Side int

const (
	Request Side = iota
	Response
)

func (s Side) String() string {
	if s == Response {
		return "response"
	}
	return "request"
}

// Kind is the structural kind of a flattened node.
type Kind string

const (
	KindString  Kind = "string"
	KindInteger Kind = "integer"
	KindNumber  Kind = "number"
	KindBoolean Kind = "boolean"
	KindObject  Kind = "object"
	KindArray   Kind = "array"
	KindUnion   Kind = "union"
	// KindRaw only appears on the SDK side (json.RawMessage) and matches any scalar.
	KindRaw Kind = "raw"
)

// Schema is one flattened node.
type Schema struct {
	Kind     Kind
	Nullable bool
	Enum     []string           // string scalars only
	Object   *Object            // KindObject
	Items    *Schema            // KindArray
	Tag      string             // KindUnion: discriminator property
	Variants map[string]*Object // KindUnion: by discriminator value
}

// Object is a flattened object with its properties and sorted required names.
type Object struct {
	Properties map[string]*Schema
	Required   []string
	// FreeForm is an object without declared properties (a Go map on the SDK side).
	FreeForm bool
}

// Doc is a parsed OpenAPI document.
type Doc struct {
	schemas map[string]any
	paths   map[string]any
}

const refPrefix = "#/components/schemas/"

// New unions must be registered here so a new oneOf in the spec cannot be accepted silently.
var responseUnions = map[string]bool{"BusinessEntity": true}

var structural = set("$ref", "allOf", "type", "nullable", "enum", "properties", "required", "items")

var unionKeywords = set("oneOf", "discriminator")

// Annotations and value-range constraints are not compared.
var ignored = set("description", "example", "examples", "default", "format", "title", "deprecated", "externalDocs",
	"maxLength", "minLength", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
	"maxItems", "minItems", "uniqueItems", "pattern", "multipleOf", "readOnly", "writeOnly")

var scalarKinds = map[string]Kind{"string": KindString, "integer": KindInteger, "number": KindNumber, "boolean": KindBoolean}

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// Load reads and parses an OpenAPI JSON file.
func Load(path string) (*Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse parses an OpenAPI JSON document.
func Parse(data []byte) (*Doc, error) {
	var raw struct {
		Paths      map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("spec: parse: %w", err)
	}
	return &Doc{schemas: raw.Components.Schemas, paths: raw.Paths}, nil
}

// PostRefs returns the schema names of the JSON request body and 200 response of POST path.
func (d *Doc) PostRefs(path string) (request, response string, err error) {
	op, _ := dig(d.paths, path, "post").(map[string]any)
	if op == nil {
		return "", "", fmt.Errorf("spec: no POST %s", path)
	}
	request, err = refName(dig(op, "requestBody", "content", "application/json", "schema", "$ref"))
	if err != nil {
		return "", "", fmt.Errorf("spec: POST %s request: %w", path, err)
	}
	response, err = refName(dig(op, "responses", "200", "content", "application/json", "schema", "$ref"))
	if err != nil {
		return "", "", fmt.Errorf("spec: POST %s 200 response: %w", path, err)
	}
	return request, response, nil
}

// PropertyRef returns the schema name that property of schema points to with a direct $ref.
func (d *Doc) PropertyRef(schema, property string) (string, error) {
	name, err := refName(dig(d.schemas, schema, "properties", property, "$ref"))
	if err != nil {
		return "", fmt.Errorf("spec: %s.%s: %w", schema, property, err)
	}
	return name, nil
}

// ItemBounds describes the minItems and maxItems of an array property declared directly on schema.
func (d *Doc) ItemBounds(schema, property string) (string, error) {
	p, ok := dig(d.schemas, schema, "properties", property).(map[string]any)
	if !ok || p["type"] != "array" {
		return "", fmt.Errorf("spec: %s.%s is not a directly declared array", schema, property)
	}
	bound := func(k string) string {
		if v, ok := p[k]; ok {
			return fmt.Sprint(v)
		}
		return "unset"
	}
	return "minItems=" + bound("minItems") + " maxItems=" + bound("maxItems"), nil
}

// PropertyKeywords describes the listed keywords of a property declared directly on schema, or of
// its items when items is true, as "key=value" pairs with "unset" for an absent keyword.
func (d *Doc) PropertyKeywords(schema, property string, items bool, keys ...string) (string, error) {
	p, ok := dig(d.schemas, schema, "properties", property).(map[string]any)
	if !ok {
		return "", fmt.Errorf("spec: %s.%s is not a directly declared property", schema, property)
	}
	if items {
		if p, ok = p["items"].(map[string]any); !ok {
			return "", fmt.Errorf("spec: %s.%s has no inline items", schema, property)
		}
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		v, ok := p[k]
		if !ok {
			v = "unset"
		}
		parts[i] = k + "=" + fmt.Sprint(v)
	}
	return strings.Join(parts, " "), nil
}

// ItemsRef returns the schema name that the items of an array property of schema point to with a direct $ref.
func (d *Doc) ItemsRef(schema, property string) (string, error) {
	name, err := refName(dig(d.schemas, schema, "properties", property, "items", "$ref"))
	if err != nil {
		return "", fmt.Errorf("spec: %s.%s items: %w", schema, property, err)
	}
	return name, nil
}

// SchemaDescription returns the description of a named schema, "unset" when it has none.
func (d *Doc) SchemaDescription(name string) (string, error) {
	s, ok := dig(d.schemas, name).(map[string]any)
	if !ok {
		return "", fmt.Errorf("spec: schema %s not found", name)
	}
	if text, ok := s["description"].(string); ok {
		return text, nil
	}
	return "unset", nil
}

// PropertyDescription returns the description of a property declared directly on schema, "unset" when it has none.
func (d *Doc) PropertyDescription(schema, property string) (string, error) {
	p, ok := dig(d.schemas, schema, "properties", property).(map[string]any)
	if !ok {
		return "", fmt.Errorf("spec: %s.%s is not a directly declared property", schema, property)
	}
	if s, ok := p["description"].(string); ok {
		return s, nil
	}
	return "unset", nil
}

func dig(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func refName(ref any) (string, error) {
	s, ok := ref.(string)
	if !ok || !strings.HasPrefix(s, refPrefix) {
		return "", fmt.Errorf("unsupported $ref %v", ref)
	}
	return strings.TrimPrefix(s, refPrefix), nil
}

type flattenError struct{ msg string }

type ctx struct {
	doc  *Doc
	side Side
}

func fail(where, format string, args ...any) {
	panic(flattenError{fmt.Sprintf(format, args...) + " at " + where})
}

// Flatten expands $ref and allOf of the named component schema into an Object.
func Flatten(doc *Doc, schemaName string, side Side) (obj *Object, err error) {
	defer func() {
		if r := recover(); r != nil {
			fe, ok := r.(flattenError)
			if !ok {
				panic(r)
			}
			obj, err = nil, fmt.Errorf("spec: flatten %s: %s", schemaName, fe.msg)
		}
	}()
	c := &ctx{doc: doc, side: side}
	s := c.build(c.merge(c.lookup(schemaName, schemaName), schemaName, []string{schemaName}, schemaName), schemaName)
	if s.Kind != KindObject {
		fail(schemaName, "root schema is not an object")
	}
	if s.Object.FreeForm {
		fail(schemaName, "root schema is a free-form object")
	}
	return s.Object, nil
}

// def is a raw sub-schema plus the $ref chain that reached it, for cycle detection.
type def struct {
	node  any
	stack []string
}

type unionDef struct {
	name  string
	node  map[string]any
	stack []string
}

// merged is a node after folding $ref and allOf, before its children are built.
type merged struct {
	typ      string
	nullable bool
	enum     []any
	props    map[string][]def
	required map[string]bool
	items    []def
	union    *unionDef
}

func (c *ctx) lookup(name, where string) any {
	s, ok := c.doc.schemas[name]
	if !ok {
		fail(where, "$ref target %q not found", name)
	}
	return s
}

func (c *ctx) merge(node any, where string, stack []string, viaRef string) *merged {
	n, ok := node.(map[string]any)
	if !ok {
		fail(where, "schema is not an object")
	}
	var unsupported []string
	for _, key := range sortedKeys(n) {
		if structural[key] || ignored[key] || strings.HasPrefix(key, "x-") {
			continue
		}
		if unionKeywords[key] && c.side == Response && responseUnions[viaRef] {
			continue
		}
		// An object with "additionalProperties": true and no properties is free-form, like one without either.
		if key == "additionalProperties" && n[key] == true && n["type"] == "object" && n["properties"] == nil {
			continue
		}
		unsupported = append(unsupported, strconv.Quote(key))
	}
	if len(unsupported) > 0 {
		fail(where, "unsupported keyword %s in %s schema", strings.Join(unsupported, ", "), c.side)
	}
	if _, ok := n["discriminator"]; ok && n["oneOf"] == nil {
		fail(where, `"discriminator" without "oneOf"`)
	}

	var parts []*merged
	if ref, ok := n["$ref"]; ok {
		name, err := refName(ref)
		if err != nil {
			fail(where, "%v", err)
		}
		for _, s := range stack {
			if s == name {
				fail(where, "$ref cycle %s -> %s", strings.Join(stack, " -> "), name)
			}
		}
		next := append(stack[:len(stack):len(stack)], name)
		parts = append(parts, c.merge(c.lookup(name, where), where+"("+name+")", next, name))
	}
	if all, ok := n["allOf"]; ok {
		members, ok := all.([]any)
		if !ok {
			fail(where, `"allOf" is not an array`)
		}
		for i, m := range members {
			parts = append(parts, c.merge(m, fmt.Sprintf("%s.allOf[%d]", where, i), stack, ""))
		}
	}

	own := &merged{nullable: n["nullable"] == true, required: map[string]bool{}}
	if t, ok := n["type"]; ok {
		own.typ = parseType(t, own, where)
	}
	if e, ok := n["enum"]; ok {
		if own.enum, ok = e.([]any); !ok {
			fail(where, `"enum" is not an array`)
		}
	}
	if p, ok := n["properties"]; ok {
		props, ok := p.(map[string]any)
		if !ok {
			fail(where, `"properties" is not an object`)
		}
		own.props = map[string][]def{}
		for k, v := range props {
			own.props[k] = []def{{node: v, stack: stack}}
		}
	}
	if r, ok := n["required"]; ok {
		list, ok := r.([]any)
		if !ok {
			fail(where, `"required" is not a string array`)
		}
		for _, x := range list {
			s, ok := x.(string)
			if !ok {
				fail(where, `"required" is not a string array`)
			}
			own.required[s] = true
		}
	}
	if it, ok := n["items"]; ok {
		own.items = []def{{node: it, stack: stack}}
	}
	if _, ok := n["oneOf"]; ok {
		own.union = &unionDef{name: viaRef, node: n, stack: stack}
	}
	parts = append(parts, own)

	acc := parts[0]
	for _, p := range parts[1:] {
		acc = combine(acc, p, where)
	}
	return acc
}

// parseType accepts a type name or the OpenAPI 3.1 form [T, "null"].
func parseType(t any, m *merged, where string) string {
	if list, ok := t.([]any); ok {
		var nonNull []any
		for _, x := range list {
			if x != "null" {
				nonNull = append(nonNull, x)
			}
		}
		if len(nonNull) != 1 || len(nonNull) == len(list) {
			fail(where, "unsupported type %v", t)
		}
		m.nullable = true
		t = nonNull[0]
	}
	s, ok := t.(string)
	if !ok {
		fail(where, "unsupported type %v", t)
	}
	return s
}

func combine(a, b *merged, where string) *merged {
	if a.union != nil || b.union != nil {
		u, other := a, b
		if a.union == nil {
			u, other = b, a
		}
		if other.typ != "" || other.props != nil || other.items != nil || other.enum != nil || len(other.required) > 0 || other.union != nil {
			fail(where, `cannot combine "oneOf" with other structure`)
		}
		out := *u
		out.nullable = a.nullable || b.nullable
		return &out
	}
	if a.typ != "" && b.typ != "" && a.typ != b.typ {
		fail(where, "conflicting types %q and %q", a.typ, b.typ)
	}
	if a.enum != nil && b.enum != nil && !reflect.DeepEqual(a.enum, b.enum) {
		fail(where, "conflicting enums")
	}
	out := &merged{nullable: a.nullable || b.nullable, required: map[string]bool{}, typ: a.typ, enum: a.enum}
	if out.typ == "" {
		out.typ = b.typ
	}
	if out.enum == nil {
		out.enum = b.enum
	}
	for _, m := range []*merged{a, b} {
		for r := range m.required {
			out.required[r] = true
		}
		out.items = append(out.items, m.items...)
		if m.props != nil && out.props == nil {
			out.props = map[string][]def{}
		}
		for k, defs := range m.props {
			out.props[k] = append(out.props[k], defs...)
		}
	}
	return out
}

// buildOne builds a property or items defined by several allOf members, which must agree.
func (c *ctx) buildOne(defs []def, where string) *Schema {
	var first *Schema
	for i, d := range defs {
		s := c.build(c.merge(d.node, where, d.stack, ""), where)
		if i == 0 {
			first = s
		} else if !reflect.DeepEqual(s, first) {
			fail(where, "conflicting definitions across allOf")
		}
	}
	return first
}

func (c *ctx) build(m *merged, where string) *Schema {
	if m.union != nil {
		return c.buildUnion(m.union, m.nullable, where)
	}
	typ := m.typ
	if typ == "" && m.props != nil {
		typ = "object"
	}
	if typ == "" {
		fail(where, `missing "type"`)
	}
	switch typ {
	case "object":
		if m.items != nil || m.enum != nil {
			fail(where, `object with "items" or "enum"`)
		}
		if m.props == nil && c.side == Request {
			fail(where, `request object without "properties" (free-form)`)
		}
		obj := &Object{Properties: map[string]*Schema{}, FreeForm: m.props == nil}
		for _, k := range sortedKeys(m.props) {
			obj.Properties[k] = c.buildOne(m.props[k], where+"."+k)
		}
		for r := range m.required {
			if obj.Properties[r] == nil {
				fail(where, "required %q is not a property", r)
			}
			obj.Required = append(obj.Required, r)
		}
		sort.Strings(obj.Required)
		return &Schema{Kind: KindObject, Nullable: m.nullable, Object: obj}
	case "array":
		if m.props != nil || len(m.required) > 0 {
			fail(where, `"array" with "properties" or "required"`)
		}
		if m.items == nil {
			fail(where, `array without "items"`)
		}
		if m.enum != nil {
			fail(where, `array with "enum"`)
		}
		return &Schema{Kind: KindArray, Nullable: m.nullable, Items: c.buildOne(m.items, where+"[]")}
	}
	kind, ok := scalarKinds[typ]
	if !ok {
		fail(where, "unsupported type %q", typ)
	}
	if m.props != nil || len(m.required) > 0 || m.items != nil {
		fail(where, `%q with "properties", "required" or "items"`, typ)
	}
	s := &Schema{Kind: kind, Nullable: m.nullable}
	if m.enum != nil {
		if kind != KindString {
			fail(where, "only string enums are supported")
		}
		for _, e := range m.enum {
			v, ok := e.(string)
			if !ok {
				fail(where, "only string enums are supported")
			}
			s.Enum = append(s.Enum, v)
		}
	}
	return s
}

func (c *ctx) buildUnion(u *unionDef, nullable bool, where string) *Schema {
	oneOf, ok := u.node["oneOf"].([]any)
	if !ok {
		fail(where, `"oneOf" is not an array`)
	}
	disc, _ := u.node["discriminator"].(map[string]any)
	tag, _ := disc["propertyName"].(string)
	mapping, _ := disc["mapping"].(map[string]any)
	if tag == "" || mapping == nil {
		fail(where, `"oneOf" in %s needs a discriminator with propertyName and mapping`, u.name)
	}
	var members, mapped []string
	for _, m := range oneOf {
		mm, ok := m.(map[string]any)
		if !ok || len(mm) != 1 {
			fail(where, `"oneOf" members must be bare $ref`)
		}
		name, err := refName(mm["$ref"])
		if err != nil {
			fail(where, "%v", err)
		}
		members = append(members, name)
	}
	variants := map[string]*Object{}
	for _, tagValue := range sortedKeys(mapping) {
		name, err := refName(mapping[tagValue])
		if err != nil {
			fail(where, "%v", err)
		}
		mapped = append(mapped, name)
		vWhere := where + "{" + tagValue + "}"
		s := c.buildOne([]def{{node: map[string]any{"$ref": refPrefix + name}, stack: u.stack}}, vWhere)
		if s.Kind != KindObject || s.Object.Properties[tag] == nil {
			fail(vWhere, "variant lacks tag property %q", tag)
		}
		variants[tagValue] = s.Object
	}
	sort.Strings(members)
	sort.Strings(mapped)
	if !reflect.DeepEqual(members, mapped) {
		fail(where, `discriminator mapping does not match "oneOf"`)
	}
	return &Schema{Kind: KindUnion, Nullable: nullable, Tag: tag, Variants: variants}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
