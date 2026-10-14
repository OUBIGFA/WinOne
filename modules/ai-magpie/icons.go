package main

import (
	"context"
	"errors"
	"github.com/yetone/magpie/internal/provider"
)

type iconRequest struct {
	Data []byte `json:"data,omitempty"`
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
}

func iconOperation(ctx context.Context, in request) (string, error) {
	if in.Icon == nil {
		return "", errors.New("请选择头像")
	}
	switch in.Operation {
	case "icons.store":
		return provider.StoreIcon(in.Icon.Data)
	case "icons.fetch":
		return provider.FetchIcon(ctx, in.Icon.URL)
	case "icons.website":
		return provider.FaviconFor(ctx, in.Icon.URL, in.Icon.Name)
	default:
		return "", errors.New("不支持的头像操作")
	}
}
