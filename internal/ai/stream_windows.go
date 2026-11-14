//go:build windows

package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

// Delta carries only new provider text. Reasoning stays separate from answers
// and is never added to conversation history or diagnostic logs.
type Delta struct{ Text, Reasoning string }

func (t Translator) invokeStream(ctx context.Context, input any, progress func(Delta) error) (string, error) {
	process, err := t.start(ctx)
	if err != nil {
		return "", err
	}
	finish := func(failure error) (string, error) {
		process.closeJob()
		_ = process.finish()
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", failure
	}
	if err = json.NewEncoder(process.input).Encode(input); err != nil {
		return finish(err)
	}
	process.input.Close()
	text, err := readAIStream(ctx, process.output, progress)
	if err != nil {
		return finish(err)
	}
	err = process.finish()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", errors.New("AI 服务异常退出，请重试")
	}
	return text, nil
}

func readAIStream(ctx context.Context, input io.Reader, progress func(Delta) error) (string, error) {
	// Bound both a single frame and the entire pipe, including NDJSON overhead.
	scanner := bufio.NewScanner(io.LimitReader(input, 64<<20))
	scanner.Buffer(make([]byte, 4096), (1<<20)+1024)
	var text strings.Builder
	total := 0
	done := false
	for scanner.Scan() {
		var event struct {
			Limited           bool   `json:"limited"`
			Type              string `json:"type"`
			Delta             string `json:"delta"`
			Status            int    `json:"status"`
			RetryAfterSeconds int    `json:"retryAfterSeconds"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil || done {
			return "", (errors.New("AI 流式响应无效，请重试"))
		}
		total += len(event.Delta)
		if total > 1<<20 {
			return "", (errors.New("AI 响应过大"))
		}
		delta := Delta{}
		switch event.Type {
		case "text":
			text.WriteString(event.Delta)
			delta.Text = event.Delta
		case "reasoning":
			delta.Reasoning = event.Delta
		case "done":
			done = true
		case "error":
			if event.Status >= 400 && event.Status <= 599 && event.RetryAfterSeconds >= 0 && event.RetryAfterSeconds <= 86400 {
				return "", (&AttemptFailure{Status: event.Status, RetryAfter: time.Duration(event.RetryAfterSeconds) * time.Second, Limited: event.Limited})
			}
			return "", (errors.New("AI 输出中断，请检查服务后重试"))
		default:
			return "", (errors.New("AI 流式事件无效"))
		}
		if delta.Text != "" || delta.Reasoning != "" {
			if err := progress(delta); err != nil {
				return "", (err)
			}
		}
	}
	if scanner.Err() != nil || !done {
		return "", (errors.New("AI 输出中断，请重试"))
	}

	if strings.TrimSpace(text.String()) == "" {
		return "", errors.New("AI 未返回结果")
	}
	return text.String(), nil
}

func (t Translator) generateThroughRuntime(ctx context.Context, input map[string]any, progress func(Delta) error) (string, error) {
	input["stream"] = true
	response, err := t.RuntimeRequest(ctx, input)
	if err != nil {
		return "", err
	}
	defer response.Close()
	if progress == nil {
		progress = func(Delta) error { return nil }
	}
	text, err := readAIStream(ctx, response, progress)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return text, err
}
