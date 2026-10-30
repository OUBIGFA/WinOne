package interconnect

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var cloudShareCode = regexp.MustCompile(`^(?:[a-zA-Z0-9]{6}|[a-zA-Z0-9]{8})$`)

// Open only GoFile's documented download pages, never arbitrary shell targets,
// API URLs, lookalike hosts, credentials or fragments containing account tokens.
func cloudDownloadLink(value string) (string, error) {
	link, err := url.Parse(strings.TrimSpace(value))
	if err != nil || link.Scheme != "https" || !strings.EqualFold(link.Host, "gofile.io") || link.User != nil || link.RawQuery != "" || link.Fragment != "" || !strings.HasPrefix(link.Path, "/d/") || !cloudShareCode.MatchString(strings.TrimPrefix(link.Path, "/d/")) {
		return "", errors.New("请输入 https://gofile.io/d/… 格式的 GoFile 下载链接")
	}
	link.Host = "gofile.io"
	return link.String(), nil
}
