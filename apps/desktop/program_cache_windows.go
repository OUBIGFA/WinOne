//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"winone/internal/settings"
)

type programCacheEntry struct {
	Version  int
	Size     int64
	Modified int64
	Metadata ProgramMetadata
}

var programCacheMu sync.Mutex

// Disk cache survives UI exit. File size and write time invalidate extracted
// resources; cache failures are reported to diagnostics without losing a read.
func cachedProgramInfo(directory, path string, read func(string) (ProgramMetadata, error)) (ProgramMetadata, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ProgramMetadata{}, err
	}
	stat, err := os.Stat(absolute)
	if err != nil {
		return ProgramMetadata{}, err
	}
	if stat.IsDir() {
		return ProgramMetadata{}, fmt.Errorf("not a program file")
	}
	key := fmt.Sprintf("%x.json", sha256.Sum256([]byte(strings.ToLower(filepath.Clean(absolute)))))
	file := filepath.Join(directory, key)
	programCacheMu.Lock()
	defer programCacheMu.Unlock()
	if data, err := os.ReadFile(file); err == nil && len(data) <= 1<<20 {
		var entry programCacheEntry
		if json.Unmarshal(data, &entry) == nil && entry.Version == 1 && entry.Size == stat.Size() && entry.Modified == stat.ModTime().UnixNano() && len(entry.Metadata.Icon) > 0 {
			return entry.Metadata, nil
		}
	}
	value, err := read(absolute)
	if err != nil {
		return value, err
	}
	if len(value.Icon) == 0 {
		return value, nil
	}
	entry := programCacheEntry{1, stat.Size(), stat.ModTime().UnixNano(), value}
	data, err := json.Marshal(entry)
	if err == nil {
		err = os.MkdirAll(directory, 0700)
	}
	if err == nil {
		err = settings.WriteAtomic(file, data)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Program icon cache:", err)
	}
	return value, nil
}
