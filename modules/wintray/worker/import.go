package worker

import "winone/modules/wintray/preferences"

// Preserve the worker API while sharing the read-only importer with Host.
type ImportPreview = preferences.ImportPreview

func PreviewImport(path string) (ImportPreview, error) {
	return preferences.PreviewImport(path)
}
