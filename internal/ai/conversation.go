package ai

import (
	"errors"
	"strings"
)

// Message is plain conversation content; modules cannot inject protocol roles
// such as tools or replace the request's separately supplied system context.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func ValidateGeneration(input Generation) error {
	if len(input.System) > 16000 || len(input.ImagePNG) > 32<<20 {
		return errors.New("AI 输入过大")
	}
	if len(input.Messages) == 0 {
		if strings.TrimSpace(input.Text) == "" || len(input.Text) > 100000 {
			return errors.New("AI 输入为空或过大")
		}
		return nil
	}
	if input.Text != "" || len(input.ImagePNG) > 0 || len(input.Messages) > 64 {
		return errors.New("对话格式无效或过长，请开始新对话")
	}
	total := len(input.System)
	for _, message := range input.Messages {
		if (message.Role != "user" && message.Role != "assistant") || strings.TrimSpace(message.Content) == "" {
			return errors.New("对话内容无效")
		}
		total += len(message.Content)
	}
	if total > 200000 {
		return errors.New("对话过长，请开始新对话")
	}
	if input.Messages[len(input.Messages)-1].Role != "user" {
		return errors.New("请先输入问题")
	}
	return nil
}

func ValidTranslationTarget(value string) bool {
	switch value {
	case "auto", "zh-CN", "zh-TW", "en", "ja", "ko":
		return true
	}
	return false
}
