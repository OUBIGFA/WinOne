//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"winone/internal/chat"
	"winone/internal/configjson"
	"winone/internal/recycle"
)

// Review always resolves a saved conversation, never a renderer-supplied CWD.
func (c *chatControl) reviewAction(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	var input struct {
		SessionID string              `json:"sessionId"`
		TurnID    string              `json:"turnId"`
		Scope     string              `json:"scope"`
		Base      string              `json:"base"`
		Path      string              `json:"path"`
		Mutation  chat.ReviewMutation `json:"mutation"`
	}
	if err := configjson.DecodeObject(raw, &input); err != nil {
		return nil, err
	}
	conversation, err := c.store.Conversation(input.SessionID)
	if err != nil {
		return nil, err
	}
	// A conversation card must open its original turn, even inside a Git project.
	if method == "review.turn.apply" {
		var snapshot chat.ReviewSnapshot
		err := c.engine.WithIdleConversation(ctx, input.SessionID, func() error {
			var err error
			snapshot, err = c.store.ApplyTurnReview(ctx, input.SessionID, input.TurnID, input.Mutation.Revision, input.Mutation.Action, func(path string, check func() error) error {
				_, err := recycle.CheckedFile(ctx, path, check)
				return err
			})
			return err
		})
		return snapshot, err
	}
	if input.TurnID != "" || method == "review.turn" {
		snapshot, patches, err := c.store.TurnReview(input.SessionID, input.TurnID)
		if err != nil {
			return nil, err
		}
		switch method {
		case "review.turn":
			return map[string]any{"snapshot": snapshot, "patches": patches}, nil
		case "review.snapshot":
			return snapshot, nil
		case "review.diff":
			if patch, ok := patches[input.Path]; ok {
				return patch, nil
			}
			return nil, errors.New("此文件不在当前工作轮次中，请刷新")
		default:
			return nil, errors.New("此操作不适用于本轮文件改动")
		}
	}
	var state chat.GitState
	// Independent chat folders do not inherit a repository from an ancestor.
	// Otherwise a workspace created under a source tree would expose unrelated
	// parent-project changes. Explicit projects retain Git's normal discovery.
	_, ownGit := os.Stat(filepath.Join(conversation.Directory, ".git"))
	if conversation.ProjectID != "" || ownGit == nil {
		state, err = chat.ReadGitState(ctx, conversation.Directory)
		if err != nil {
			return nil, err
		}
	} else if !errors.Is(ownGit, os.ErrNotExist) {
		return nil, ownGit
	}
	if !state.Repository {
		switch method {
		case "review.branches":
			return []string{}, nil
		case "review.commits":
			return []chat.ReviewCommit{}, nil
		case "review.mutate":
			return nil, errors.New("当前文件夹未使用 Git，无法暂存、提交或推送")
		case "review.snapshot", "review.diff":
			snapshot, patches, err := c.store.SessionReview(input.SessionID)
			if err != nil {
				return nil, err
			}
			if method == "review.snapshot" {
				return snapshot, nil
			}
			patch, ok := patches[input.Path]
			if !ok {
				return nil, errors.New("此文件没有已记录的修改，请刷新")
			}
			return patch, nil
		}
	}
	switch method {
	case "review.snapshot":
		return chat.ReadReview(ctx, conversation.Directory, input.Scope, input.Base)
	case "review.diff":
		return chat.ReadReviewDiff(ctx, conversation.Directory, input.Scope, input.Base, input.Path)
	case "review.branches":
		return chat.ReviewBranches(ctx, conversation.Directory)
	case "review.commits":
		return chat.ReviewCommits(ctx, conversation.Directory)
	case "review.mutate":
		return nil, chat.MutateReview(ctx, conversation.Directory, input.Mutation)
	}
	return nil, errors.New("未知审查操作")
}
