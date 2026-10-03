//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

type ModuleLogEntry struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Truncated bool   `json:"truncated"`
	Text      string `json:"text"`
}
type ModuleLogs struct {
	FolderAvailable bool             `json:"folderAvailable"`
	Entries         []ModuleLogEntry `json:"entries"`
}
type moduleLogFile struct{ name, file string }

// Only known log files are exposed; this API never accepts a filesystem path.
func moduleLogFiles(id string) ([]moduleLogFile, error) {
	switch id {
	case "host":
		return []moduleLogFile{{"process", "host.log"}}, nil
	case "wintray":
		return []moduleLogFile{{"activity", "wintray.log"}, {"process", "worker.log"}}, nil
	case "capture", "always-on-top":
		return []moduleLogFile{{"process", "worker.log"}}, nil
	default:
		return nil, errors.New("unknown diagnostics module")
	}
}

func moduleLogDirectory(id string) string {
	if id == "host" {
		return "logs"
	}
	return filepath.Join("modules", id)
}

// ReadModuleLogs reads existing data without contacting or starting a worker.
// Root prevents module/log symlinks from escaping the product data directory.
func (a *App) ReadModuleLogs(id string) (ModuleLogs, error) {
	files, err := moduleLogFiles(id)
	if err != nil {
		return ModuleLogs{}, err
	}
	result := ModuleLogs{Entries: make([]ModuleLogEntry, len(files))}
	for i, file := range files {
		result.Entries[i].Name = file.name
	}
	if a.dataDirectory == "" {
		return result, errors.New("diagnostics directory unavailable")
	}
	root, err := os.OpenRoot(a.dataDirectory)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer root.Close()
	folder, err := root.OpenRoot(moduleLogDirectory(id))
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer folder.Close()
	result.FolderAvailable = true
	for i, spec := range files {
		entry, err := readModuleLog(folder, spec)
		if err != nil {
			return result, err
		}
		result.Entries[i] = entry
	}
	return result, nil
}

func readModuleLog(folder *os.Root, spec moduleLogFile) (ModuleLogEntry, error) {
	entry := ModuleLogEntry{Name: spec.name}
	file, err := folder.Open(spec.file)
	if os.IsNotExist(err) {
		return entry, nil
	}
	if err != nil {
		return entry, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return entry, err
	}
	if !info.Mode().IsRegular() {
		return entry, errors.New("log is not a regular file")
	}
	const limit = 64 << 10
	offset := max(int64(0), info.Size()-limit)
	data, err := io.ReadAll(io.NewSectionReader(file, offset, limit))
	if err != nil {
		return entry, err
	}
	entry.Available = true
	entry.Truncated = offset > 0
	if entry.Truncated {
		for len(data) > 0 && !utf8.RuneStart(data[0]) {
			data = data[1:]
		}
	}
	entry.Text = string(bytes.ToValidUTF8(data, []byte("?")))
	return entry, nil
}

// OpenModuleLogFolder opens only a validated, existing module directory.
func (a *App) OpenModuleLogFolder(id string) error {
	if _, err := moduleLogFiles(id); err != nil {
		return err
	}
	if a.dataDirectory == "" {
		return errors.New("diagnostics directory unavailable")
	}
	root, err := os.OpenRoot(a.dataDirectory)
	if err != nil {
		return err
	}
	defer root.Close()
	folder, err := root.OpenRoot(moduleLogDirectory(id))
	if err != nil {
		return err
	}
	folder.Close()
	directory, err := filepath.Abs(filepath.Join(a.dataDirectory, moduleLogDirectory(id)))
	if err != nil {
		return err
	}
	windowsDir, err := windows.GetWindowsDirectory()
	if err != nil {
		return err
	}
	command := exec.Command(filepath.Join(windowsDir, "explorer.exe"), directory)
	if err = command.Start(); err != nil {
		return err
	}
	go func() {
		if err := command.Wait(); err != nil {
			fmt.Fprintln(os.Stderr, "log folder:", err)
		}
	}()
	return nil
}
