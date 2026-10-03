//go:build windows

package logintask

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const administratorLaunchArguments = `--authorized-start "$(Arg0)"`

// Read actual action/principal fields, not only the description fingerprint.
// A stale or changed task must never become an arbitrary elevated launcher.
func validateAdministratorDefinition(raw []byte, sid, executable string) error {
	if len(raw) == 0 || len(raw) > 1<<20 {
		return errors.New("管理员启动任务格式无效")
	}
	text, err := schedulerXMLText(raw)
	if err != nil {
		return err
	}
	var value struct {
		XMLName  xml.Name
		Triggers struct {
			Items []struct{ XMLName xml.Name } `xml:",any"`
		}
		Principals struct {
			Items []struct {
				ID    string `xml:"id,attr"`
				User  string `xml:"UserId"`
				Group string `xml:"GroupId"`
				Logon string `xml:"LogonType"`
				Level string `xml:"RunLevel"`
			} `xml:"Principal"`
		}
		Settings struct {
			Enabled                 *bool
			AllowStartOnDemand      *bool
			MultipleInstancesPolicy string
		}
		Actions struct {
			Context string `xml:"Context,attr"`
			Items   []struct {
				XMLName                              xml.Name
				Command, Arguments, WorkingDirectory string
			} `xml:",any"`
		}
	}
	decoder := xml.NewDecoder(strings.NewReader(text))
	// schtasks exports Unicode text with its original encoding declaration.
	decoder.CharsetReader = func(name string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(name, "utf-16") {
			return input, nil
		}
		return nil, errors.New("不支持的任务编码")
	}
	if err = decoder.Decode(&value); err != nil {
		return err
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if chars, ok := token.(xml.CharData); ok && strings.TrimSpace(string(chars)) == "" {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		return errors.New("管理员启动任务包含多余内容")
	}
	fail := errors.New("管理员启动任务已变化，请重新授权")
	if value.XMLName.Local != "Task" || value.XMLName.Space != "http://schemas.microsoft.com/windows/2004/02/mit/task" || len(value.Triggers.Items) != 0 || len(value.Principals.Items) != 1 || len(value.Actions.Items) != 1 {
		return fail
	}
	principal, action := value.Principals.Items[0], value.Actions.Items[0]
	if principal.User != sid || principal.Group != "" || principal.Logon != "InteractiveToken" || principal.Level != "HighestAvailable" || principal.ID == "" || value.Actions.Context != principal.ID {
		return fail
	}
	// Task Scheduler may omit Enabled=true from its exported XML. Both Enabled
	// and AllowStartOnDemand default to true; explicit false still rejects use.
	if (value.Settings.Enabled != nil && !*value.Settings.Enabled) || (value.Settings.AllowStartOnDemand != nil && !*value.Settings.AllowStartOnDemand) || value.Settings.MultipleInstancesPolicy != "Parallel" {
		return fail
	}
	samePath := func(first, second string) bool {
		return strings.EqualFold(filepath.Clean(first), filepath.Clean(second))
	}
	if action.XMLName.Local != "Exec" || !samePath(action.Command, executable) || !samePath(action.WorkingDirectory, filepath.Dir(executable)) || action.Arguments != administratorLaunchArguments {
		return fail
	}
	return nil
}

func schedulerXMLText(raw []byte) (string, error) {
	if bytes.HasPrefix(raw, []byte{0xff, 0xfe}) || bytes.HasPrefix(raw, []byte{0xfe, 0xff}) || (len(raw) > 1 && raw[1] == 0) {
		var order binary.ByteOrder = binary.LittleEndian
		if bytes.HasPrefix(raw, []byte{0xfe, 0xff}) {
			order = binary.BigEndian
			raw = raw[2:]
		} else if bytes.HasPrefix(raw, []byte{0xff, 0xfe}) {
			raw = raw[2:]
		}
		if len(raw)%2 != 0 {
			return "", errors.New("任务文本编码不完整")
		}
		units := make([]uint16, len(raw)/2)
		for index := range units {
			units[index] = order.Uint16(raw[index*2:])
		}
		return string(utf16.Decode(units)), nil
	}
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	if utf8.Valid(raw) {
		return string(raw), nil
	}
	return oemText(raw), nil
}
