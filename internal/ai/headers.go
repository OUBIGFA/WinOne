package ai

import (
	"errors"
	"strings"
)

// Keep the exact header spelling for Magpie, while rejecting control characters
// before they enter an HTTP request. Header values may contain credentials and
// are intentionally never included in validation errors.
func validateProviderHeaders(headers map[string]string) error {
	if len(headers) > 64 {
		return errors.New("自定义请求头过多")
	}
	total := 0
	seen := map[string]bool{}
	for name, value := range headers {
		if name == "" || strings.TrimSpace(name) != name || len(name) > 256 || seen[strings.ToLower(name)] {
			return errors.New("请求头名称为空、重复或无效")
		}
		seen[strings.ToLower(name)] = true
		for _, ch := range name {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", ch)) {
				return errors.New("请求头名称包含无效字符")
			}
		}
		for _, ch := range value {
			if ch < 32 && ch != '\t' || ch == 127 {
				return errors.New("请求头值必须为单行文字；JSON 请先检查格式")
			}
		}
		total += len(name) + len(value)
	}
	if total > 64<<10 {
		return errors.New("自定义请求头过大")
	}
	return nil
}
