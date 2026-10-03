package captureprefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

func DecodeFileNameTemplate(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	_, err := FileName(value, time.Now(), 1)
	return value, err
}

var reservedName = regexp.MustCompile(`^(CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[1-9¹²³])$`)

// Templates produce one filename, never a path. A missing sequence token
// still receives a suffix on collision so literal custom names remain safe.
func FileName(template string, at time.Time, sequence int) (string, error) {
	fail := func() (string, error) {
		return "", errors.New("文件名模板无效，请检查占位符、长度和特殊字符")
	}
	useDefault := template == ""
	if useDefault {
		template = "WinOne-{timestamp}-OK"
	}
	if len(template) > 160 || !utf8.ValidString(template) || sequence < 1 {
		return fail()
	}
	name := strings.NewReplacer("{timestamp}", at.Format("20060102150405"), "{date}", at.Format("20060102"), "{time}", at.Format("150405"), "{sequence}", fmt.Sprintf("%02d", sequence)).Replace(template)
	if strings.ContainsAny(name, "{}<>:\"/\\|?*") {
		return fail()
	}
	for _, r := range name {
		if r < 32 {
			return fail()
		}
	}
	if strings.HasSuffix(strings.ToLower(name), ".png") {
		name = name[:len(name)-4]
	}
	if name == "" || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fail()
	}
	if reservedName.MatchString(strings.ToUpper(strings.TrimSpace(strings.Split(name, ".")[0]))) {
		return fail()
	}
	if sequence > 1 && !strings.Contains(template, "{sequence}") {
		suffix := sequence
		if useDefault {
			suffix--
		}
		name += fmt.Sprintf("-%02d", suffix)
	}
	if len(name) > 240 {
		return fail()
	}
	return name + ".png", nil
}
