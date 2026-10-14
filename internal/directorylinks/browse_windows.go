//go:build windows

package directorylinks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BrowseEntry describes an entry, not a registered link or an assertion that
// its contents can be read. Detailed verification remains an explicit scan.
type BrowseEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	ObjectType string `json:"objectType"`
	Kind       string `json:"kind,omitempty"`
	TargetPath string `json:"targetPath,omitempty"`
	Error      string `json:"error,omitempty"`
}

type DirectoryListing struct {
	Directory string        `json:"directory"`
	Parent    string        `json:"parent"`
	Entries   []BrowseEntry `json:"entries"`
	Total     int           `json:"total"`
	Offset    int           `json:"offset"`
	HasMore   bool          `json:"hasMore"`
}

// Browse reads one requested directory without registering history, traversing
// children, or creating anything. Paging bounds native metadata work while
// keeping every entry reachable through the UI's "load more" action.
func (m *Manager) Browse(ctx context.Context, directory string, offset int) (DirectoryListing, error) {
	result := DirectoryListing{Entries: []BrowseEntry{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	path, err := localPath(directory)
	if err != nil {
		return result, err
	}
	if offset < 0 {
		return result, errors.New("文件夹列表位置无效")
	}
	root, err := openState(path, true, false)
	if err != nil {
		return result, err
	}
	defer root.close()
	if !root.directory {
		return result, errors.New("请选择需要浏览的文件夹")
	}
	// Preserve the address the user opened; entering a junction is an explicit
	// navigation action and must not silently rewrite the selected source path.
	entries, err := os.ReadDir(path)
	if err != nil {
		return result, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	result.Directory, result.Parent, result.Total = path, filepath.Dir(path), len(entries)
	if strings.EqualFold(result.Parent, path) {
		result.Parent = ""
	}
	result.Offset = min(offset, len(entries))
	end := min(result.Offset+500, len(entries))
	result.HasMore = end < len(entries)
	for _, entry := range entries[result.Offset:end] {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		item := BrowseEntry{Name: entry.Name(), Path: filepath.Join(path, entry.Name()), ObjectType: "file"}
		if entry.IsDir() {
			item.ObjectType = "directory"
		}
		state, err := openState(item.Path, false, false)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.ObjectType = state.objectType()
			if state.reparse {
				tag, target, relative, readErr := readReparse(state)
				if readErr != nil {
					item.Error = readErr.Error()
				} else {
					item.Kind, item.TargetPath = "symlink", target
					if tag == tagJunction {
						item.Kind = "junction"
					}
					if relative {
						item.TargetPath = filepath.Join(path, target)
					}
				}
			} else if !state.directory && strings.EqualFold(filepath.Ext(item.Path), ".lnk") {
				item.Kind = "shortcut"
			} else if !state.directory && state.links > 1 {
				item.Kind = "hardlink"
			}
			state.close()
		}
		result.Entries = append(result.Entries, item)
	}
	return result, nil
}
