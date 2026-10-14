//go:build windows

package distribution

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Completed transactions are immutable history. Do not rename their root:
// WebView2 or a diagnostic reader can still have a cache directory open there.
// Search all numbered siblings before allocating, so a removed history entry
// cannot hide a later unfinished transaction that must be resumed.
func installerWorkDirectory(r InstallerUpdate, base string) (string, error) {
	entries, err := os.ReadDir(filepath.Dir(base))
	if err != nil {
		return "", err
	}
	name := filepath.Base(base)
	occupied := map[int]string{}
	for _, entry := range entries {
		index := 0
		if !strings.EqualFold(entry.Name(), name) {
			if !strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(name)+"-") {
				continue
			}
			suffix := entry.Name()[len(name)+1:]
			index, err = strconv.Atoi(suffix)
			if err != nil || index < 1 || strconv.Itoa(index) != suffix {
				continue // Old random-suffixed backups are already completed history.
			}
		}
		occupied[index] = filepath.Join(filepath.Dir(base), entry.Name())
	}
	indices := make([]int, 0, len(occupied))
	for index := range occupied {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	owner := installerUpdateOwner{r.Installation, filepath.Clean(r.Profile), r.RegistryKey, filepath.Clean(r.ShortcutPath)}
	active := ""
	for _, index := range indices {
		work := occupied[index]
		info, err := os.Lstat(work)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("恢复目录不是普通文件夹：%s", work)
		}
		var saved installerUpdateOwner
		if err = readUpgradeMetadata(work, "owner.json", &saved); err != nil {
			return "", fmt.Errorf("无法核实恢复目录 %s：%w", work, err)
		}
		if saved != owner {
			return "", fmt.Errorf("恢复目录属于另一安装操作：%s", work)
		}
		journal, err := OpenUpgradeJournal(filepath.Join(work, "journal.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue // Interrupted preparation never reached any live writes.
		}
		if err != nil {
			return "", fmt.Errorf("无法读取恢复记录 %s：%w", work, err)
		}
		phase := journal.Snapshot().Phase
		if phase == "committed" || phase == "rolled-back" {
			continue
		}
		if active != "" {
			return "", errors.New("发现多个未完成的覆盖安装事务，请保留恢复目录")
		}
		active = work
	}
	if active != "" {
		return active, nil
	}
	if _, err = os.Lstat(filepath.Join(r.Installation, ".winone-pending")); !errors.Is(err, os.ErrNotExist) {
		return "", errors.Join(errors.New("安装仍有未完成标记，但未找到对应恢复事务"), err)
	}
	for index := 0; index <= len(occupied); index++ {
		if _, exists := occupied[index]; !exists {
			if index == 0 {
				return base, nil
			}
			return fmt.Sprintf("%s-%d", base, index), nil
		}
	}
	return "", errors.New("无法分配覆盖安装恢复目录")
}
