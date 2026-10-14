//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"strings"
	"winone/internal/configjson"
)

func (c *chatControl) terminalAction(owner uint32, method string, raw json.RawMessage) (any, error) {
	var in struct {
		SessionID string `json:"sessionId"`
		TabID     string `json:"tabId"`
		Shell     string `json:"shell"`
		Data      string `json:"data"`
		Cols      int    `json:"cols"`
		Rows      int    `json:"rows"`
		Offset    uint64 `json:"offset"`
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
		return nil, c.terminals.Write(owner, in.TabID, []byte(in.Data))
	case "resize":
		return nil, c.terminals.Resize(owner, in.TabID, in.Cols, in.Rows)
	case "dispose":
		return nil, c.terminals.Dispose(owner, in.TabID)
	default:
		return nil, errors.New("不支持的终端操作")
	}
}
