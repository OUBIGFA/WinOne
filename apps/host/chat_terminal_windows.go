//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"winone/internal/configjson"
)

// Native VT input is a byte stream, whose chunks need not end on a UTF-8
// boundary. Keep the legacy string route while accepting exact native bytes.
func terminalInputData(text string, encoded *string) ([]byte, error) {
	if encoded == nil {
		return []byte(text), nil
	}
	if text != "" {
		return nil, errors.New("终端输入不能同时包含文字和二进制数据")
	}
	data, err := base64.StdEncoding.DecodeString(*encoded)
	if err != nil {
		return nil, errors.New("终端输入编码无效")
	}
	return data, nil
}

func (c *chatControl) terminalAction(owner uint32, method string, raw json.RawMessage) (any, error) {
	var in struct {
		SessionID  string  `json:"sessionId"`
		TabID      string  `json:"tabId"`
		Shell      string  `json:"shell"`
		Data       string  `json:"data"`
		DataBase64 *string `json:"dataBase64"`
		Cols       int     `json:"cols"`
		Rows       int     `json:"rows"`
		Offset     uint64  `json:"offset"`
	}
	if err := configjson.DecodeObject(raw, &in); err != nil {
		return nil, err
	}
	if c.terminals == nil {
		return nil, errors.New("终端服务不可用")
	}
	operation := strings.TrimPrefix(method, "terminal.")
	switch operation {
	case "create", "restart":
		conversation, err := c.store.Conversation(in.SessionID)
		if err != nil {
			return nil, err
		}
		if operation == "restart" {
			if err = c.terminals.Dispose(owner, in.TabID); err != nil {
				return nil, err
			}
		}
		return c.terminals.Create(owner, in.TabID, in.SessionID, conversation.Directory, in.Shell, in.Cols, in.Rows)
	case "poll":
		return c.terminals.Poll(owner, in.TabID, in.Offset)
	case "write":
		data, err := terminalInputData(in.Data, in.DataBase64)
		if err != nil {
			return nil, err
		}
		return nil, c.terminals.Write(owner, in.TabID, data)
	case "resize":
		return nil, c.terminals.Resize(owner, in.TabID, in.Cols, in.Rows)
	case "dispose":
		return nil, c.terminals.Dispose(owner, in.TabID)
	default:
		return nil, errors.New("不支持的终端操作")
	}
}
