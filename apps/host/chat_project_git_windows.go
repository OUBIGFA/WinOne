//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"winone/internal/chat"
	"winone/internal/configjson"
	"winone/internal/host"
)

// projectGit resolves a registered project, never an arbitrary frontend path.
// The selected ID is echoed only for stale-response checks, not presentation.
func (c *chatControl) projectGit(request host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "chat_git", Message: err.Error()}}
	}
	var input struct {
		ProjectID string `json:"projectId"`
	}
	if err := configjson.DecodeObject(request.Params, &input); err != nil {
		return fail(err)
	}
	for _, project := range c.store.Snapshot().Projects {
		if project.ID != input.ProjectID {
			continue
		}
		state, err := chat.ReadGitState(c.ctx, project.Path)
		if err != nil {
			return fail(err)
		}
		result, err := json.Marshal(struct {
			ProjectID string `json:"projectId"`
			chat.GitState
		}{ProjectID: project.ID, GitState: state})
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: result}
	}
	return fail(errors.New("请选择已登记的项目"))
}
