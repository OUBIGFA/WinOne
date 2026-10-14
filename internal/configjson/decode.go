// Package configjson validates bounded-depth configuration objects.
package configjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// DecodeObject rejects unknown fields, duplicate keys and trailing content.
func DecodeObject(data []byte, target any) error {
	if err := ValidateObject(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// ValidateObject requires a UTF-8 JSON object with unique keys and bounded depth.
func ValidateObject(data []byte) error {
	_, err := readObject(data, false)
	return err
}

// ParseObject shares the same duplicate-key, UTF-8 and depth checks while
// retaining the parsed values. Schema validation can consume this tree without
// parsing untrusted JSON a second time. Numbers retain their exact spelling.
func ParseObject(data []byte) (map[string]any, error) {
	value, err := readObject(data, true)
	if err != nil {
		return nil, err
	}
	return value.(map[string]any), nil
}

func readObject(data []byte, collect bool) (any, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("配置不是有效的 UTF-8 文本")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("配置内容必须为 JSON 对象")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	value, err := walkJSON(decoder, 0, collect)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("JSON 包含多余内容")
	}
	return value, nil
}
func walkJSON(decoder *json.Decoder, depth int, collect bool) (any, error) {
	if depth > 32 {
		return nil, errors.New("配置嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		if collect {
			return token, nil
		}
		return nil, nil
	}
	var result any
	switch delimiter {
	case '{':
		var keys map[string]bool
		var object map[string]any
		if collect {
			object = map[string]any{}
			result = object
		} else {
			keys = map[string]bool{}
		}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			seen := keys[name]
			if collect {
				_, seen = object[name]
			}
			if !ok || seen {
				return nil, errors.New("JSON 包含重复字段")
			}
			value, err := walkJSON(decoder, depth+1, collect)
			if err != nil {
				return nil, err
			}
			if collect {
				object[name] = value
			} else {
				keys[name] = true
			}
		}
	case '[':
		var values []any
		if collect {
			values = []any{}
		}
		for decoder.More() {
			value, err := walkJSON(decoder, depth+1, collect)
			if err != nil {
				return nil, err
			}
			if collect {
				values = append(values, value)
			}
		}
		if collect {
			result = values
		}
	default:
		return nil, errors.New("JSON 结构无效")
	}
	_, err = decoder.Token()
	return result, err
}
