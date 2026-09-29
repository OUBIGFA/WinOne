//go:build !windows

package worker

import "os"

func replaceJournal(from, to string) error { return os.Rename(from, to) }
