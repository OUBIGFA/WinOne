package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
	paths := []string{}
	for i, path := range t.spec.Paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		t.mu.Lock()
		t.state.Detail = path
		t.mu.Unlock()
		if !source.IsDir() && total > 300*1024*1024 {
			paths = append(paths, path)
			continue
		}
		directory := filepath.Join(t.spec.DataDir, fmt.Sprint(i))
		if err = os.MkdirAll(directory, 0700); err != nil {
			return nil, err
		}
		dest := filepath.Join(directory, filepath.Base(path))
		if source.IsDir() {
			dest += ".zip"
			err = utils.ZipDirectoryContext(ctx, dest, path, nil, nil)
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
	}
	return map[string]any{"paths": paths, "cached": total <= 300*1024*1024, "bytes": total}, nil
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
