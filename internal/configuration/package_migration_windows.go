//go:build windows

package configuration

import (
	"context"
	"errors"
	"os"
	"winone/internal/recycle"
)

// RecyclePackageMigration is called only for a completed package transaction's
// retained cleanup record. A pending transaction must keep its verified backup.
func (s *HostStore) RecyclePackageMigration(ctx context.Context, operation, digest string) error {
	path, err := s.packageMigrationPath(operation)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	check := func() error { _, err := s.LoadPackageMigration(operation, digest); return err }
	if err = check(); err != nil {
		return err
	}
	receipt, err := recycle.CheckedFile(ctx, path, check)
	if err != nil {
		return err
	}
	if !receipt.Recycled {
		return errors.New("模块迁移备份回收未确认")
	}
	return nil
}
