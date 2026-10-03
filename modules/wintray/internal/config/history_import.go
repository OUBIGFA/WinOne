package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DecodeHistory validates imported records before any destination is changed.
func DecodeHistory(data []byte) (map[string][]time.Time, error) {
	var state startupHistoryData
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Version != 1 || state.Apps == nil {
		return nil, errors.New("不支持的运行历史格式")
	}
	result := map[string][]time.Time{}
	for path, stamps := range state.Apps {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("运行历史包含非绝对路径")
		}
		key := strings.ToLower(filepath.Clean(path))
		for _, stamp := range stamps {
			if stamp.IsZero() {
				return nil, errors.New("运行历史包含无效时间")
			}
			result[key] = append(result[key], stamp)
		}
	}
	return result, nil
}

// MergeImport preserves actual launch records even if a later settings save fails.
// It is additive and idempotent: retrying an import cannot refund or duplicate a launch.
func (h *StartupHistory) MergeImport(data []byte) (err error) {
	imported, err := DecodeHistory(data)
	if err != nil {
		return err
	}
	release, err := h.lock()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	current := map[string][]time.Time{}
	existing, err := os.ReadFile(h.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		current, err = DecodeHistory(existing)
		if err != nil {
			return err
		}
	}
	for path, stamps := range imported {
		// Preserve multiplicity within each source; identical reservations in two copies
		// represent the same launches, so retain the larger count for each timestamp.
		counts := map[time.Time]int{}
		old := map[time.Time]int{}
		for _, stamp := range current[path] {
			old[stamp.UTC()]++
		}
		for _, stamp := range stamps {
			counts[stamp.UTC()]++
		}
		for stamp, n := range counts {
			for i := old[stamp]; i < n; i++ {
				current[path] = append(current[path], stamp)
			}
		}
		sort.Slice(current[path], func(i, j int) bool { return current[path][i].Before(current[path][j]) })
	}
	return h.save(startupHistoryData{Version: 1, Apps: current})
}
