package core

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
)

// Preserved decodes a JSON object into T and keeps every field T does not
// declare, so a page that edits part of a Host document saves the rest as
// it was. The React window kept such fields by spreading the object it
// read; Go structs would drop them.
//
// The fields T declares (by json tag, or Go name) are written from T, and
// removed when T omits them (omitempty), so clearing a value in the UI
// clears it in the document. Nested documents that also carry unknown
// fields use Preserved themselves.
type Preserved[T any] struct {
	Value T
	rest  map[string]json.RawMessage
}

// UnmarshalJSON reads the declared fields into Value and keeps the others.
func (p *Preserved[T]) UnmarshalJSON(data []byte) error {
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		all = nil
	} else if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	for name := range declaredFields(reflect.TypeFor[T]()) {
		delete(all, name)
	}
	p.Value, p.rest = value, all
	return nil
}

// MarshalJSON writes Value's fields over the preserved ones.
func (p Preserved[T]) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(p.Value)
	if err != nil || len(p.rest) == 0 {
		return data, err
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(data, &known); err != nil {
		// T is not an object; there is nothing to merge into.
		return data, nil
	}
	merged := make(map[string]json.RawMessage, len(known)+len(p.rest))
	for name, raw := range p.rest {
		merged[name] = raw
	}
	for name, raw := range known {
		merged[name] = raw
	}
	return json.Marshal(merged)
}

// declaredFields lists the JSON names a struct type declares, including
// those of embedded structs, as encoding/json resolves them.
func declaredFields(t reflect.Type) map[string]struct{} {
	names := map[string]struct{}{}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return names
	}
	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if field.Anonymous && name == "" {
			for embedded := range declaredFields(field.Type) {
				names[embedded] = struct{}{}
			}
			continue
		}
		if !field.IsExported() {
			continue
		}
		if name == "" {
			name = field.Name
		}
		names[name] = struct{}{}
	}
	return names
}
