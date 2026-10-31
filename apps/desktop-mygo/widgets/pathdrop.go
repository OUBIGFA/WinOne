package widgets

import (
	"os"
	"path/filepath"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// DropKind is what a drop area takes: files, folders or either.
type DropKind int

const (
	DropAny DropKind = iota
	DropFiles
	DropFolders
)

// CheckDroppedPaths checks paths dropped from File Explorer on an area
// before the page takes them, as React's usePathDrop (ui/path-drop.ts) did
// with the previous window's InspectDroppedPaths: at least one local path,
// only one unless multiple, each an absolute path to an existing file or
// folder of the kind the area takes. It reads the file system: call it off
// the UI thread, or through DropPaths.
func CheckDroppedPaths(paths []string, kind DropKind, multiple bool) ([]string, error) {
	if len(paths) == 0 || len(paths) > 1024 {
		return nil, core.Errorf(i18n.T("fileDrop.localOnly"))
	}
	if !multiple && len(paths) != 1 {
		return nil, core.Errorf(i18n.T("fileDrop.single"))
	}
	checked := make([]string, 0, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, core.Errorf(i18n.T("fileDrop.absolute"))
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		directory := info.IsDir()
		switch {
		case !directory && !info.Mode().IsRegular():
			return nil, core.Errorf(i18n.T("fileDrop.localOnly"))
		case kind == DropFolders && !directory:
			return nil, core.Errorf(i18n.T("fileDrop.folderOnly"))
		case kind == DropFiles && directory:
			return nil, core.Errorf(i18n.T("fileDrop.fileOnly"))
		}
		checked = append(checked, path)
	}
	return checked, nil
}

// DropPaths checks dropped paths off the UI thread, then gives the page
// the paths or the reason they were refused, on the UI thread.
func DropPaths(env *core.Env, paths []string, kind DropKind, multiple bool, done func(paths []string, err error)) {
	core.Go(env, func() ([]string, error) { return CheckDroppedPaths(paths, kind, multiple) }, done)
}
