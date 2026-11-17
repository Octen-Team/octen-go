package spec

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// Message suffixes for fields that only the SDK has; known_diffs.json may excuse exactly these.
const (
	MsgSDKOnly        = "in SDK but not in spec"
	MsgSDKVariantOnly = "variant in SDK but not in spec"
)

// Diff compares SDK tree a with spec tree b; paths start with "." (".a[]{company}.b").
// Requests compare names, kinds, enums and required; responses compare names and
// nullability, where a spec-nullable field needs an SDK type that can hold null.
func Diff(a, b *Object, side Side) []string {
	var out []string
	diffObject(side, a, b, "", &out)
	return out
}

func add(out *[]string, path, format string, args ...any) {
	*out = append(*out, path+": "+fmt.Sprintf(format, args...))
}

func diffObject(side Side, a, b *Object, path string, out *[]string) {
	if a.FreeForm || b.FreeForm {
		if a.FreeForm != b.FreeForm {
			add(out, path, "free-form mismatch (sdk %v, spec %v)", a.FreeForm, b.FreeForm)
		}
		return
	}
	for _, key := range sortedKeys(a.Properties) {
		p := path + "." + key
		specChild, ok := b.Properties[key]
		if !ok {
			add(out, p, MsgSDKOnly)
			continue
		}
		if side == Request {
			if sdkReq, specReq := has(a.Required, key), has(b.Required, key); sdkReq != specReq {
				add(out, p, "required mismatch (sdk %v, spec %v)", sdkReq, specReq)
			}
		}
		diffSchema(side, a.Properties[key], specChild, p, out)
	}
	for _, key := range sortedKeys(b.Properties) {
		if _, ok := a.Properties[key]; !ok {
			add(out, path+"."+key, "in spec but not in SDK")
		}
	}
}

func isScalar(k Kind) bool {
	return k == KindString || k == KindInteger || k == KindNumber || k == KindBoolean
}

func diffSchema(side Side, a, b *Schema, path string, out *[]string) {
	if side == Response && b.Nullable && !a.Nullable {
		add(out, path, "spec allows null but the SDK type cannot hold null")
	}
	switch {
	case a.Kind == KindObject && b.Kind == KindObject:
		diffObject(side, a.Object, b.Object, path, out)
	case a.Kind == KindArray && b.Kind == KindArray:
		diffSchema(side, a.Items, b.Items, path+"[]", out)
	case a.Kind == KindUnion && b.Kind == KindUnion:
		if a.Tag != b.Tag {
			add(out, path, "union tag mismatch (sdk %s, spec %s)", a.Tag, b.Tag)
		}
		for _, v := range sortedKeys(a.Variants) {
			p := path + "{" + v + "}"
			if specVariant, ok := b.Variants[v]; ok {
				diffObject(side, a.Variants[v], specVariant, p, out)
			} else {
				add(out, p, MsgSDKVariantOnly)
			}
		}
		for _, v := range sortedKeys(b.Variants) {
			if _, ok := a.Variants[v]; !ok {
				add(out, path+"{"+v+"}", "variant in spec but not in SDK")
			}
		}
	case isScalar(b.Kind) && (isScalar(a.Kind) || a.Kind == KindRaw):
		if side == Response {
			return
		}
		if a.Kind != b.Kind {
			add(out, path, "type mismatch (sdk %s, spec %s)", a.Kind, b.Kind)
			return
		}
		if sa, sb := sortedCopy(a.Enum), sortedCopy(b.Enum); !reflect.DeepEqual(sa, sb) {
			add(out, path, "enum mismatch (sdk %v, spec %v)", sa, sb)
		}
	default:
		add(out, path, "type mismatch (sdk %s, spec %s)", a.Kind, b.Kind)
	}
}

func has(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func sortedCopy(xs []string) []string {
	if xs == nil {
		return nil
	}
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}

// KnownDiff is one documented SDK-only field from known_diffs.json.
type KnownDiff struct {
	Path   string
	Reason string
}

var knownPath = regexp.MustCompile(`^[A-Za-z0-9_]+\.\S+$`)

// ParseKnownDiffs parses known_diffs.json: an array of {path, reason} with unique paths.
func ParseKnownDiffs(data []byte) ([]KnownDiff, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("known_diffs: top level must be an array: %w", err)
	}
	seen := map[string]bool{}
	out := make([]KnownDiff, 0, len(raw))
	for i, r := range raw {
		var e map[string]any
		if err := json.Unmarshal(r, &e); err != nil || e == nil {
			return nil, fmt.Errorf("known_diffs[%d]: entry must be an object", i)
		}
		path, _ := e["path"].(string)
		if !knownPath.MatchString(path) {
			return nil, fmt.Errorf(`known_diffs[%d]: path must look like "<SchemaName>.<dotted.path>"`, i)
		}
		reason, _ := e["reason"].(string)
		if strings.TrimSpace(reason) == "" {
			return nil, fmt.Errorf("known_diffs[%d] (%s): reason is required", i, path)
		}
		if seen[path] {
			return nil, fmt.Errorf("known_diffs: duplicate path %s", path)
		}
		seen[path] = true
		out = append(out, KnownDiff{Path: path, Reason: reason})
	}
	return out, nil
}

// ApplyKnownDiffs prefixes diffs with root, drops SDK-only fields listed in known, and reports
// entries for root that no longer match one, so the list can only shrink.
func ApplyKnownDiffs(root string, diffs []string, known []KnownDiff) []string {
	listed := map[string]bool{}
	for _, k := range known {
		if strings.HasPrefix(k.Path, root+".") {
			listed[k.Path] = true
		}
	}
	used := map[string]bool{}
	var out []string
	for _, d := range diffs {
		q := root + d
		path, msg, _ := strings.Cut(q, ": ")
		if listed[path] && (msg == MsgSDKOnly || msg == MsgSDKVariantOnly) {
			used[path] = true
			continue
		}
		out = append(out, q)
	}
	for _, k := range known {
		if listed[k.Path] && !used[k.Path] {
			out = append(out, k.Path+": listed in known_diffs.json but not an SDK-only field; remove the entry")
		}
	}
	return out
}
