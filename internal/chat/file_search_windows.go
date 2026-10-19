//go:build windows

package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"
)

type FileSearchMatch struct {
	LineNumber int               `json:"lineNumber"`
	LineText   string            `json:"lineText"`
	Submatches []FileSearchRange `json:"submatches"`
}
type FileSearchRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}
type FileSearchResult struct {
	RelPath string            `json:"relPath"`
	Matches []FileSearchMatch `json:"matches"`
}
type FileSearchPage struct {
	Results      []FileSearchResult `json:"results"`
	TotalMatches int                `json:"totalMatches"`
	TotalFiles   int                `json:"totalFiles"`
	Truncated    bool               `json:"truncated"`
}
type FileIndex struct {
	Files     []string `json:"files"`
	Truncated bool     `json:"truncated"`
}

// rg reads only the already-resolved saved workspace. No shell expansion or
// executable commands come from the search text, and .gitignore stays active.
func fileSearchCommand(ctx context.Context, executable, directory string, args ...string) *exec.Cmd {
	// Production supplies its bundled executable. Never depend on an agent's
	// temporary PATH, and never let RIPGREP_CONFIG_PATH alter the scoped query.
	command := exec.CommandContext(ctx, executable, append([]string{"--no-config"}, args...)...)
	command.Dir = directory
	command.Env = gitReadEnvironment(os.Environ())
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsNoWindow}
	return command
}
func ReadFileIndex(ctx context.Context, executable, directory string) (FileIndex, error) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	command := fileSearchCommand(child, executable, directory, "--files", "--hidden", "-g", "!.git", "-0")
	pipe, err := command.StdoutPipe()
	if err != nil {
		return FileIndex{}, err
	}
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	if err = command.Start(); err != nil {
		return FileIndex{}, err
	}
	result := FileIndex{Files: []string{}}
	reader := bufio.NewReader(pipe)
	for {
		path, readErr := reader.ReadString(0)
		if len(path) > 0 {
			result.Files = append(result.Files, strings.ReplaceAll(strings.TrimSuffix(path, "\x00"), "\\", "/"))
		}
		if len(result.Files) >= 100000 {
			result.Truncated = true
			cancel()
			break
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			cancel()
			_ = command.Wait()
			return result, readErr
		}
	}
	err = command.Wait()
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if result.Truncated {
		return result, nil
	}
	if err != nil && command.ProcessState.ExitCode() != 1 {
		return result, errors.New("文件索引失败：" + diagnostic.String())
	}
	return result, nil
}

func SearchWorkspaceFiles(ctx context.Context, executable, directory, query string, caseSensitive bool, maxMatches int) (FileSearchPage, error) {
	result := FileSearchPage{Results: []FileSearchResult{}}
	if strings.TrimSpace(query) == "" {
		return result, nil
	}
	if len(query) > 4096 {
		return result, errors.New("搜索文字过长")
	}
	if maxMatches <= 0 || maxMatches > 2000 {
		maxMatches = 500
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	args := []string{"--json", "--hidden", "--fixed-strings", "--line-number", "--encoding", "utf-8", "-g", "!.git"}
	if !caseSensitive {
		args = append(args, "--ignore-case")
	}
	args = append(args, "--", query, ".")
	command := fileSearchCommand(child, executable, directory, args...)
	pipe, err := command.StdoutPipe()
	if err != nil {
		return result, err
	}
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	if err = command.Start(); err != nil {
		return result, err
	}
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	byPath := map[string]int{}
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				Line       int               `json:"line_number"`
				Submatches []FileSearchRange `json:"submatches"`
			} `json:"data"`
		}
		if err = json.Unmarshal(scanner.Bytes(), &event); err != nil {
			cancel()
			_ = command.Wait()
			return result, err
		}
		if event.Type != "match" {
			continue
		}
		path := strings.TrimPrefix(strings.ReplaceAll(event.Data.Path.Text, "\\", "/"), "./")
		index, exists := byPath[path]
		if !exists {
			index = len(result.Results)
			byPath[path] = index
			result.Results = append(result.Results, FileSearchResult{RelPath: path, Matches: []FileSearchMatch{}})
		}
		text := strings.TrimSuffix(strings.TrimSuffix(event.Data.Lines.Text, "\n"), "\r")
		ranges := []FileSearchRange{}
		for _, span := range event.Data.Submatches {
			if span.Start < 0 || span.End < span.Start || span.End > len(text) {
				continue
			}
			ranges = append(ranges, FileSearchRange{Start: len(utf16.Encode([]rune(text[:span.Start]))), End: len(utf16.Encode([]rune(text[:span.End])))})
		}
		result.Results[index].Matches = append(result.Results[index].Matches, FileSearchMatch{LineNumber: event.Data.Line, LineText: text, Submatches: ranges})
		result.TotalMatches += len(ranges)
		if result.TotalMatches >= maxMatches {
			result.Truncated = true
			cancel()
			break
		}
	}
	scanErr := scanner.Err()
	err = command.Wait()
	result.TotalFiles = len(result.Results)
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if result.Truncated {
		return result, nil
	}
	if scanErr != nil {
		return result, scanErr
	}
	if err != nil && command.ProcessState.ExitCode() != 1 {
		return result, errors.New("搜索失败：" + diagnostic.String())
	}
	return result, nil
}
