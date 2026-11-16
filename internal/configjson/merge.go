package configjson

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// Merge applies the changes from base to draft to current. Independent object
// fields and ID-addressed records can coexist; an overlapping scalar edit or
// delete-versus-edit is reported before anything is written. Values never enter
// the error text, since configuration fields can contain credentials.
func Merge(base, draft, current []byte) ([]byte, error) {
	values := make([]any, 3)
	for i, raw := range [][]byte{base, draft, current} {
		value, err := ParseObject(raw)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	merged, err := mergeValue(values[0], values[1], values[2], "")
	if err != nil {
		return nil, err
	}
	return json.Marshal(merged)
}

type missingValue struct{}

func mergeValue(base, draft, current any, path string) (any, error) {
	if reflect.DeepEqual(base, draft) {
		return current, nil
	}
	if reflect.DeepEqual(base, current) || reflect.DeepEqual(draft, current) {
		return draft, nil
	}
	b, bok := base.(map[string]any)
	d, dok := draft.(map[string]any)
	c, cok := current.(map[string]any)
	if !bok && (base == nil || base == (missingValue{})) && dok && cok {
		b, bok = map[string]any{}, true
	}
	if bok && dok && cok {
		keys := map[string]bool{}
		for _, object := range []map[string]any{b, d, c} {
			for key := range object {
				keys[key] = true
			}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		out := map[string]any{}
		get := func(object map[string]any, key string) any {
			if value, ok := object[key]; ok {
				return value
			}
			return missingValue{}
		}
		for _, key := range ordered {
			value, err := mergeValue(get(b, key), get(d, key), get(c, key), path+"/"+key)
			if err != nil {
				return nil, err
			}
			if _, missing := value.(missingValue); !missing {
				out[key] = value
			}
		}
		return out, nil
	}
	// Provider/key arrays carry stable IDs. Do not pair records by position:
	// removing or inserting one record must never edit its former neighbour.
	ba, bok := base.([]any)
	da, dok := draft.([]any)
	ca, cok := current.([]any)
	if bok && dok && cok {
		bm, bIDs := indexedRecords(ba)
		dm, dIDs := indexedRecords(da)
		cm, cIDs := indexedRecords(ca)
		if bm != nil && dm != nil && cm != nil {
			merged, err := mergeValue(bm, dm, cm, path)
			if err != nil {
				return nil, err
			}
			// Use the user's order if edited, otherwise the current order, then
			// append independently added records in their original order.
			order, extra := cIDs, dIDs
			if !reflect.DeepEqual(bIDs, dIDs) {
				order, extra = dIDs, cIDs
			}
			records := merged.(map[string]any)
			out := []any{}
			for _, id := range append(order, extra...) {
				if value, ok := records[id]; ok {
					out = append(out, value)
					delete(records, id)
				}
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("此项已在其他操作中修改，当前草稿已保留：%s", path)
}

func indexedRecords(values []any) (map[string]any, []string) {
	out, ids := map[string]any{}, []string{}
	for _, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, nil
		}
		id, ok := object["id"].(string)
		if !ok || id == "" || out[id] != nil {
			return nil, nil
		}
		out[id] = value
		ids = append(ids, id)
	}
	return out, ids
}
