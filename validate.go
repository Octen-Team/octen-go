package octen

import (
	"math"
	"reflect"
	"strconv"
	"strings"
)

// validateParams rejects a blank query and any NaN or ±Inf number before a request is sent:
// encoding/json refuses non-finite floats with an error that names neither field nor value.
func validateParams(v any) error {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return blankQuery()
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return &ValidationError{Field: "params", Message: "params must be a request struct."}
	}
	q, ok := jsonField(rv, "query")
	if !ok || q.Kind() != reflect.String || strings.TrimSpace(q.String()) == "" {
		return blankQuery()
	}
	return checkFinite(rv, "")
}

func blankQuery() error {
	return &ValidationError{Field: "query", Message: "query is required and must be a non-empty string."}
}

// jsonField finds the exported struct field serialised under name.
func jsonField(rv reflect.Value, name string) (reflect.Value, bool) {
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); f.IsExported() && jsonName(f) == name {
			return rv.Field(i), true
		}
	}
	return reflect.Value{}, false
}

func jsonName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "" {
		return f.Name
	}
	return name
}

func checkFinite(rv reflect.Value, path string) error {
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		if f := rv.Float(); math.IsNaN(f) || math.IsInf(f, 0) {
			return &ValidationError{Field: path, Message: path + " must be a finite number, got " + strconv.FormatFloat(f, 'g', -1, 64) + "."}
		}
	case reflect.Ptr, reflect.Interface:
		if !rv.IsNil() {
			return checkFinite(rv.Elem(), path)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if err := checkFinite(rv.Index(i), path+"["+strconv.Itoa(i)+"]"); err != nil {
				return err
			}
		}
	case reflect.Map:
		iter := rv.MapRange()
		for iter.Next() {
			if err := checkFinite(iter.Value(), join(path, iter.Key().String())); err != nil {
				return err
			}
		}
	case reflect.Struct:
		t := rv.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() || f.Tag.Get("json") == "-" {
				continue
			}
			if err := checkFinite(rv.Field(i), join(path, jsonName(f))); err != nil {
				return err
			}
		}
	}
	return nil
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
