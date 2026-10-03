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
	if !utf8.Valid(data) {
		return errors.New("配置不是有效的 UTF-8 文本")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("配置内容必须为 JSON 对象")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := walkJSON(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("JSON 包含多余内容")
	}
	return nil
}
func walkJSON(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("配置嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return errors.New("JSON 包含重复字段")
			}
			keys[name] = true
			if err = walkJSON(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := walkJSON(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("JSON 结构无效")
	}
	_, err = decoder.Token()
	return err
}
