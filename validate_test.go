package octen

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func wantValidationError(t *testing.T, err error, field string) {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v (%T), want *ValidationError", err, err)
	}
	if ve.Field != field {
		t.Fatalf("Field = %q, want %q (message %q)", ve.Field, field, ve.Message)
	}
	if ve.Message == "" {
		t.Fatal("Message is empty")
	}
}

func TestBlankQueryRejected(t *testing.T) {
	for _, q := range []string{"", "  ", "\t", "\n \r\t"} {
		for _, params := range []any{
			&SearchParams{Query: q},
			&BroadSearchParams{Query: q},
			&NewsSearchParams{Query: q},
			&BusinessSearchParams{Query: q},
			&PulseSearchParams{Query: q},
			&PlaceSearchParams{Query: q},
			SearchParams{Query: q},
		} {
			err := validateParams(params)
			wantValidationError(t, err, "query")
			if !strings.Contains(err.Error(), "query") {
				t.Errorf("%T with query %q: message %q does not name query", params, q, err)
			}
		}
	}
	wantValidationError(t, validateParams((*SearchParams)(nil)), "query")
}

func TestValidParamsAccepted(t *testing.T) {
	for _, params := range []any{
		&SearchParams{Query: " q ", Count: Int(0), Language: []string{"en"}, Highlight: &HighlightOptions{Enable: Bool(true)}},
		&BroadSearchParams{Query: "q", SearchOptions: &WebSearchOptions{Count: Int(3)}},
		&PlaceSearchParams{Query: "q", Latitude: Float64(0), Longitude: Float64(-122.4), Places: &PlaceCardOptions{Count: Int(1)}},
		&PlaceSearchParams{Query: "q"},
	} {
		if err := validateParams(params); err != nil {
			t.Errorf("%T: %v", params, err)
		}
	}
}

// nestedFloats exercises paths the public request types do not have yet.
type nestedFloats struct {
	Query  string         `json:"query"`
	Inner  *nestedInner   `json:"inner,omitempty"`
	List   []float64      `json:"list,omitempty"`
	Items  []nestedInner  `json:"items,omitempty"`
	Ptrs   []*nestedInner `json:"ptrs,omitempty"`
	Small  *float32       `json:"small,omitempty"`
	hidden float64
}

type nestedInner struct {
	F float64 `json:"f,omitempty"`
}

func TestNonFiniteNumbersRejected(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		wantValidationError(t, validateParams(&PlaceSearchParams{Query: "q", Latitude: Float64(bad)}), "latitude")
		wantValidationError(t, validateParams(&PlaceSearchParams{Query: "q", Latitude: Float64(1), Longitude: Float64(bad)}), "longitude")
		wantValidationError(t, validateParams(&nestedFloats{Query: "q", Inner: &nestedInner{F: bad}}), "inner.f")
		wantValidationError(t, validateParams(&nestedFloats{Query: "q", List: []float64{1, bad}}), "list[1]")
		wantValidationError(t, validateParams(&nestedFloats{Query: "q", Items: []nestedInner{{F: bad}}}), "items[0].f")
		wantValidationError(t, validateParams(&nestedFloats{Query: "q", Ptrs: []*nestedInner{nil, {F: bad}}}), "ptrs[1].f")
		wantValidationError(t, validateParams(&nestedFloats{Query: "q", Small: Ptr(float32(bad))}), "small")
	}
	err := validateParams(&PlaceSearchParams{Query: "q", Latitude: Float64(math.Inf(1))})
	if !strings.Contains(err.Error(), "finite") {
		t.Errorf("message %q does not say finite", err)
	}
	// Unexported fields are never serialised, so they are not checked.
	if err := validateParams(&nestedFloats{Query: "q", hidden: math.NaN()}); err != nil {
		t.Errorf("unexported field checked: %v", err)
	}
	// A blank query is reported before any number.
	wantValidationError(t, validateParams(&PlaceSearchParams{Latitude: Float64(math.NaN())}), "query")
}

func TestNewEndpointInputsValidated(t *testing.T) {
	text := func(s string) ImageSearchInput { return ImageSearchInput{Type: "text", Data: String(s)} }
	cases := []struct {
		name   string
		params any
		field  string // empty when the params are valid
	}{
		{"image text", ImageSearchParams{Inputs: []ImageSearchInput{text("cats")}}, ""},
		{"image url", ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image", URL: String("https://example.com/a.jpg")}}}, ""},
		{"image base64 data", ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image", Data: String("aGVsbG8=")}}}, ""},
		{"image unknown type", ImageSearchParams{Inputs: []ImageSearchInput{{Type: "audio", Data: String("x")}}}, "inputs[0].type"},
		{"image empty type", ImageSearchParams{Inputs: []ImageSearchInput{{Data: String("cats")}}}, "inputs[0].type"},
		{"image nil inputs", ImageSearchParams{}, "inputs"},
		{"image empty inputs", ImageSearchParams{Inputs: []ImageSearchInput{}}, "inputs"},
		{"image two inputs", ImageSearchParams{Inputs: []ImageSearchInput{text("a"), text("b")}}, "inputs"},
		{"image text without data", ImageSearchParams{Inputs: []ImageSearchInput{{Type: "text"}}}, "inputs[0].data"},
		{"image text blank data", ImageSearchParams{Inputs: []ImageSearchInput{text(" \t")}}, "inputs[0].data"},
		{"image without url or data", ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image"}}}, "inputs[0]"},
		{"image blank url", ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image", URL: String(" ")}}}, "inputs[0]"},
		{"image blank data", ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image", Data: String("")}}}, "inputs[0]"},
		{"image url and data", ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image", URL: String("https://example.com/a.jpg"), Data: String("aGVsbG8=")}}}, "inputs[0]"},
		{"image url and empty data", ImageSearchParams{Inputs: []ImageSearchInput{{Type: "image", URL: String("https://example.com/a.jpg"), Data: String("")}}}, "inputs[0]"},
		{"video", VideoSearchParams{Inputs: []VideoSearchInput{{Type: "text", Data: String("knots")}}}, ""},
		{"video nil inputs", VideoSearchParams{}, "inputs"},
		{"video two inputs", VideoSearchParams{Inputs: []VideoSearchInput{{Type: "text", Data: String("a")}, {Type: "text", Data: String("b")}}}, "inputs"},
		{"video without data", VideoSearchParams{Inputs: []VideoSearchInput{{Type: "text"}}}, "inputs[0].data"},
		{"video blank data", VideoSearchParams{Inputs: []VideoSearchInput{{Type: "text", Data: String("\n")}}}, "inputs[0].data"},
		{"video image type", VideoSearchParams{Inputs: []VideoSearchInput{{Type: "image", Data: String("knots")}}}, "inputs[0].type"},
		{"video empty type", VideoSearchParams{Inputs: []VideoSearchInput{{Data: String("knots")}}}, "inputs[0].type"},
		{"design query", DesignSearchParams{Query: String("hero")}, ""},
		{"design group", DesignSearchParams{Images: &DesignImageOptions{GroupID: String("pg_1")}}, ""},
		{"design group with blank query", DesignSearchParams{Query: String(" "), Images: &DesignImageOptions{GroupID: String("pg_1")}}, ""},
		{"design query with blank group", DesignSearchParams{Query: String("hero"), Images: &DesignImageOptions{GroupID: String("")}}, ""},
		{"design nothing", DesignSearchParams{}, "query"},
		{"design blank query", DesignSearchParams{Query: String("  ")}, "query"},
		{"design images without group", DesignSearchParams{Images: &DesignImageOptions{Count: Int(2)}}, "query"},
		{"design blank group", DesignSearchParams{Images: &DesignImageOptions{GroupID: String(" ")}}, "query"},
		{"design pointer", &DesignSearchParams{Query: String("hero")}, ""},
		{"image pointer", &ImageSearchParams{}, "inputs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateParams(tc.params)
			if tc.field == "" {
				if err != nil {
					t.Fatalf("valid params rejected: %v", err)
				}
				return
			}
			wantValidationError(t, err, tc.field)
		})
	}
}
