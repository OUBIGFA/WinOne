package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
	"github.com/schollz/croc/v11/src/utils"
)

// FileShare publishes regular files. Directory preparation directly reuses
// croc's cancellable ZIP implementation, including its exclusion/path checks.
// Small selections get an immutable local copy; large regular files stay at
// their original location. Archives always need disk space for the ZIP.
func (t *task) prepareShare(ctx context.Context) (any, error) {
	var total int64
	for _, path := range t.spec.Paths {
		if err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("不支持符号链接: %s", p)
			}
			if !d.IsDir() {
				s, e := d.Info()
				if e != nil {
					return e
				}
				if !s.Mode().IsRegular() {
					return fmt.Errorf("不是普通文件: %s", p)
				}
				total += s.Size()
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	// croc's exclusion and gitignore switches describe one published selection.
	// Apply them here so the LAN pool and the code transfer stay equal.
	ignoredPaths, filterErr := t.gitIgnoredPaths(ctx)
	if filterErr != nil {
		return nil, filterErr
	}
	excluded := func(abs string, name string) bool {
		if ignoredPaths[abs] {
			return true
		}
		for _, pattern := range t.spec.Options.Exclude {
			if matched, matchErr := filepath.Match(pattern, name); matchErr == nil && matched {
				return true
			}
		}
		for _, filter := range t.spec.Options.ExcludeFile {
			if strings.EqualFold(filter, name) {
				return true
			}
		}
		return false
	}
	paths := []string{}
	codePaths := []string{}
	for i, path := range t.spec.Paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		absolute, absoluteErr := filepath.Abs(path)
		if absoluteErr != nil {
			return nil, absoluteErr
		}
		if excluded(absolute, source.Name()) {
			continue
		}
		codePath := path
		t.mu.Lock()
		t.state.Detail = path
		t.mu.Unlock()
		if !source.IsDir() && total > 300*1024*1024 {
			paths = append(paths, path)
			codePaths = append(codePaths, codePath)
			continue
		}
		directory := filepath.Join(t.spec.DataDir, fmt.Sprint(i))
		if err = os.MkdirAll(directory, 0700); err != nil {
			return nil, err
		}
		dest := filepath.Join(directory, filepath.Base(path))
		if source.IsDir() {
			dest += ".zip"
			err = utils.ZipDirectoryContext(ctx, dest, path, ignoredPaths, t.spec.Options.Exclude)
			if t.spec.Options.ZipFolder {
				codePath = dest
			}
		} else {
			var input, output *os.File
			input, err = os.Open(path)
			if err != nil {
				return nil, err
			}
			output, err = os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err == nil {
				_, err = io.Copy(output, &contextReader{ctx: ctx, Reader: input})
				err = errors.Join(err, output.Close())
			}
			after, statErr := input.Stat()
			err = errors.Join(err, statErr, input.Close())
			if err == nil && (after.Size() != source.Size() || !after.ModTime().Equal(source.ModTime())) {
				err = errors.New("复制期间源文件发生变化，请重试")
			}
		}
		if err != nil {
			return nil, err
		}
		paths = append(paths, dest)
		codePaths = append(codePaths, codePath)
	}
	return map[string]any{"paths": paths, "codePaths": codePaths, "cached": total <= 300*1024*1024, "bytes": total}, nil
}

// Cloud services accept regular files; reuse the same cancellable archive
// preparation as the LAN pool instead of a second ZIP implementation.
func (t *task) prepareStoredPaths(ctx context.Context) ([]string, error) {
	if t.spec.Text != "" {
		if len(t.spec.Paths) != 0 {
			return nil, errors.New("请选择文件或文本，不要混合发送")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(t.spec.DataDir, "WinOne-text.txt")
		if err := os.WriteFile(path, []byte(t.spec.Text), 0600); err != nil {
			return nil, err
		}
		return []string{path}, nil
	}
	result, err := t.prepareShare(ctx)
	if err != nil {
		return nil, err
	}
	paths := result.(map[string]any)["paths"].([]string)
	if len(paths) == 0 {
		return nil, errors.New("没有可上传的文件")
	}
	return paths, nil
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

// Mirror croc's gitignore handling: the working directory's .gitignore filters
// the selection itself; a selected folder's .gitignore filters its subtree.
func (t *task) gitIgnoredPaths(ctx context.Context) (map[string]bool, error) {
	ignored := map[string]bool{}
	if !t.spec.Options.GitIgnore {
		return ignored, nil
	}
	if working, err := os.Stat(".gitignore"); err == nil {
		if rules, err := ignore.CompileIgnoreFile(working.Name()); err == nil {
			for _, path := range t.spec.Paths {
				if !rules.MatchesPath(path) {
					continue
				}
				abs, err := filepath.Abs(path)
				if err != nil {
					return nil, err
				}
				ignored[abs] = true
			}
		}
	}
	for _, path := range t.spec.Paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		rulesPath := filepath.Join(path, ".gitignore")
		if _, err = os.Stat(rulesPath); err != nil {
			continue
		}
		rules, err := ignore.CompileIgnoreFile(rulesPath)
		if err != nil {
			return nil, err
		}
		if err = markIgnored(ctx, path, rules, ignored); err != nil {
			return nil, err
		}
	}
	return ignored, nil
}

func markIgnored(ctx context.Context, dir string, rules *ignore.GitIgnore, ignored map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dir, entry.Name())
		if rules.MatchesPath(entry.Name()) {
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			ignored[abs] = true
			continue
		}
		if entry.IsDir() {
			if err = markIgnored(ctx, path, rules, ignored); err != nil {
				return err
			}
		}
	}
	return nil
}
