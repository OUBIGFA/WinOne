package configuration

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"winone/internal/catalog"
)

// PackageMigrationOperations lists only bounded control-file names. It does not
// read configuration values, create a directory, or treat a name as ownership.
func PackageMigrationOperations(ctx context.Context, profile string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(profile) {
		return nil, errors.New("migration inspection requires an absolute profile")
	}
	directory := filepath.Join(profile, "package-migrations")
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("模块迁移目录类型无效")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var result []string
	count := 0
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		entries, readErr := file.ReadDir(64)
		count += len(entries)
		if count > 512 {
			return nil, errors.New("迁移备份目录项目过多，请先检查")
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, "migration-") || !strings.HasSuffix(name, ".json") {
				continue
			}
			operation := strings.TrimSuffix(strings.TrimPrefix(name, "migration-"), ".json")
			if catalog.ValidID(operation) && len(operation) <= 128 {
				result = append(result, operation)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	sort.Strings(result)
	return result, nil
}
