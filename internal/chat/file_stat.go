package chat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// FileStat is a lightweight existence/type result, not a preview capability.
// Building file chips and recognizing inline paths must not hash/read 10 MB of
// every file or allocate HTTP preview tokens that no viewer will consume.
type FileStat struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
	Modified  int64  `json:"modified"`
}
type FileStatResult struct {
	Requested string    `json:"requested"`
	File      *FileStat `json:"file,omitempty"`
	Missing   bool      `json:"missing,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func (s *Store) StatFile(id, path string) (FileStat, error) {
	c, err := s.Conversation(id)
	if err != nil {
		return FileStat{}, err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.Directory, path)
	}
	path = filepath.Clean(path)
	// Directories are allowed only through an alias inside the saved workspace.
	// Following a junction does not grant the renderer its external target root.
	if within(c.Directory, path) {
		if info, err := os.Stat(path); err != nil {
			return FileStat{}, err
		} else if info.IsDir() {
			return FileStat{Path: path, Name: filepath.Base(path), Directory: true, Modified: info.ModTime().UnixMilli()}, nil
		}
	}
	resolved, err := s.ResolvePreviewFile(id, path)
	if err != nil {
		return FileStat{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return FileStat{}, err
	}
	return FileStat{Path: path, Name: filepath.Base(path), Size: info.Size(), Modified: info.ModTime().UnixMilli()}, nil
}
func (s *Store) StatFiles(id string, paths []string) ([]FileStatResult, error) {
	if _, err := s.Conversation(id); err != nil {
		return nil, err
	}
	if len(paths) > 128 {
		return nil, errors.New("一次最多检查 128 个文件位置")
	}
	size := 0
	for _, path := range paths {
		size += len(path)
	}
	if size > 256<<10 {
		return nil, errors.New("文件位置列表过长")
	}
	result := make([]FileStatResult, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" || seen[path] {
			continue
		}
		seen[path] = true
		value, err := s.StatFile(id, path)
		entry := FileStatResult{Requested: path}
		if err == nil {
			entry.File = &value
		} else if errors.Is(err, os.ErrNotExist) {
			entry.Missing = true
		} else {
			entry.Error = err.Error()
		}
		result = append(result, entry)
	}
	return result, nil
}
