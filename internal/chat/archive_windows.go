//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"time"
)

func (s *Service) ExportArchive(ctx context.Context, options ArchiveOptions) (outcome ArchiveOutcome, err error) {
	if len(options.Password) > 16384 {
		return outcome, errors.New("导出密码过长")
	}
	if err = ValidateArchiveTarget(options.TargetPath); err != nil {
		return outcome, err
	}
	// At most one 256 MiB export runs at a time. It has no model credentials and
	// cannot interrupt an active agent; closing it only stops its own export job.
	select {
	case s.exports <- struct{}{}:
		defer func() { <-s.exports }()
	case <-ctx.Done():
		return outcome, ctx.Err()
	}
	conversation, err := s.store.Conversation(options.ID)
	if err != nil {
		return outcome, err
	}
	draft, err := s.store.ReadDraft(options.ID)
	if err != nil {
		return outcome, err
	}
	runner, err := StartRunner(ctx, s.options.Bun, s.options.Worker, s.store.root, nil)
	if err != nil {
		return outcome, err
	}
	s.mu.Lock()
	if s.paused {
		s.mu.Unlock()
		ending, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Close(ending)
		return outcome, errors.New("Chat 正在退出")
	}
	s.archiveRunner = runner
	s.mu.Unlock()
	defer func() {
		ending, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closeErr := runner.Close(ending)
		s.mu.Lock()
		if s.archiveRunner == runner {
			s.archiveRunner = nil
		}
		s.mu.Unlock()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	params := map[string]any{"conversation": conversation, "draft": draft, "catalogPath": s.store.path, "targetPath": options.TargetPath, "excludeMedia": options.ExcludeMedia}
	if options.Password != "" {
		params["password"] = options.Password
	}
	raw, err := runner.Call(ctx, "export", params)
	if err != nil {
		return outcome, err
	}
	if err = json.Unmarshal(raw, &outcome); err != nil {
		return outcome, err
	}
	if outcome.Status == "oversize" {
		if outcome.LimitBytes != 256<<20 || outcome.TotalBytes <= outcome.LimitBytes {
			return outcome, errors.New("任务导出大小结果无效")
		}
		return outcome, nil
	}
	if outcome.Status != "ok" || outcome.FilePath != options.TargetPath || !slices.Contains([]string{"full", "partial", "db-only"}, outcome.Fidelity) || outcome.FileBytes < 12 || outcome.MessageCount < 0 || outcome.MediaMissing < 0 {
		return outcome, errors.New("无法确认任务已完整导出")
	}
	file, err := os.Open(options.TargetPath)
	if err != nil {
		return outcome, err
	}
	defer file.Close()
	header := make([]byte, 8)
	n, err := file.Read(header)
	if err != nil || n != 8 || string(header) != "XDTSHARE" {
		return outcome, errors.New("导出的任务文件头无效")
	}
	info, err := file.Stat()
	if err != nil || info.Size() != outcome.FileBytes {
		return outcome, errors.New("导出的任务文件大小不一致")
	}
	return outcome, nil
}
