package chat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type ArchiveOptions struct {
	ID           string `json:"id"`
	TargetPath   string `json:"targetPath"`
	Password     string `json:"password,omitempty"`
	ExcludeMedia bool   `json:"excludeMedia,omitempty"`
}

// ArchiveOutcome is a receipt, never the archive bytes. The worker reports
// missing media and transcripts rather than presenting a partial file as full.
type ArchiveOutcome struct {
	Status             string   `json:"status"`
	FilePath           string   `json:"filePath,omitempty"`
	Fidelity           string   `json:"fidelity,omitempty"`
	UnpackedBytes      int64    `json:"unpackedBytes,omitempty"`
	TotalBytes         int64    `json:"totalBytes"`
	MediaBytes         int64    `json:"mediaBytes"`
	LimitBytes         int64    `json:"limitBytes,omitempty"`
	FileBytes          int64    `json:"fileBytes,omitempty"`
	MessagesBytes      int64    `json:"messagesBytes,omitempty"`
	TranscriptBytes    int64    `json:"transcriptBytes,omitempty"`
	MediaMissing       int      `json:"mediaMissing"`
	MediaDropped       int      `json:"mediaDropped"`
	MediaExcluded      int      `json:"mediaExcluded"`
	MissingTranscripts []string `json:"missingTranscripts,omitempty"`
	OrcaWorkers        int      `json:"orcaWorkers"`
	MessageCount       int      `json:"messageCount"`
}

func ValidateArchiveTarget(target string) error {
	if !filepath.IsAbs(target) || strings.ContainsRune(target, 0) || !strings.EqualFold(filepath.Ext(target), ".cshare") {
		return errors.New("请选择 .cshare 文件保存位置")
	}
	parent, err := os.Stat(filepath.Dir(target))
	if err != nil || !parent.IsDir() {
		return errors.New("任务导出目录不存在")
	}
	existing, err := os.Lstat(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && (!existing.Mode().IsRegular() || existing.Mode()&os.ModeSymlink != 0) {
		return errors.New("任务导出位置必须是普通文件")
	}
	return nil
}
