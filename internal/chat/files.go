package chat

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type FileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
}
type FilePage struct {
	Directory string      `json:"directory"`
	Entries   []FileEntry `json:"entries"`
}

// Keep the workspace alias as the capability boundary. A directory link that
// the user placed inside it is browseable; its target is not a new root that
// callers may address directly.
func workspaceAlias(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if !within(root, path) {
		return "", errors.New("文件位置不在当前工作文件夹内")
	}
	return path, nil
}

// List through the alias so nested paths and the parent button retain the
// workspace's directory structure, including junctions and symbolic links.
func (s *Store) Files(id, directory string) (FilePage, error) {
	c, err := s.Conversation(id)
	if err != nil {
		return FilePage{}, err
	}
	if directory == "" {
		directory = c.Directory
	} else if !filepath.IsAbs(directory) {
		directory = filepath.Join(c.Directory, directory)
	}
	root, err := existingDirectory(c.Directory)
	if err != nil {
		return FilePage{}, err
	}
	alias, err := workspaceAlias(root, directory)
	if err != nil {
		return FilePage{}, err
	}
	if info, err := os.Stat(alias); err != nil {
		return FilePage{}, err
	} else if !info.IsDir() {
		return FilePage{}, errors.New("所选位置不是文件夹")
	}
	entries, err := os.ReadDir(alias)
	if err != nil {
		return FilePage{}, err
	}
	if len(entries) > 5000 {
		return FilePage{}, errors.New("文件夹项目过多，请选择较小的工作文件夹")
	}
	page := FilePage{Directory: alias, Entries: []FileEntry{}}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return FilePage{}, err
		}
		path := filepath.Join(alias, entry.Name())
		// Stat follows both symbolic links and Windows directory junctions.
		// Broken links stay visible; opening reports their filesystem error.
		if target, err := os.Stat(path); err == nil {
			info = target
		}
		page.Entries = append(page.Entries, FileEntry{Name: entry.Name(), Path: path, Directory: info.IsDir(), Size: info.Size()})
	}
	sort.SliceStable(page.Entries, func(i, j int) bool {
		a, b := page.Entries[i], page.Entries[j]
		if a.Directory != b.Directory {
			return a.Directory
		}
		return a.Name < b.Name
	})
	return page, nil
}
