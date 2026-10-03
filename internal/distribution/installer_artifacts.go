package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"winone/internal/configjson"
)

// InstallerArtifactFile records one relative executable name and exact content.
type InstallerArtifactFile struct {
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

// InstallerArtifacts binds the independently exported uninstaller to its setup
// and canonical package manifest. A missing test classification is invalid.
type InstallerArtifacts struct {
	SchemaVersion         int                   `json:"schemaVersion"`
	Version               string                `json:"version"`
	Platform              string                `json:"platform"`
	PackageManifestSHA256 string                `json:"packageManifestSHA256"`
	TestBuild             *bool                 `json:"testBuild"`
	Installer             InstallerArtifactFile `json:"installer"`
	Uninstaller           InstallerArtifactFile `json:"uninstaller"`
}

// VerifyInstallerArtifacts checks consistency, not publisher identity. The
// updater must authenticate release metadata before using these executable files.
func VerifyInstallerArtifacts(recordPath, referencePackage string, allowTest bool) (InstallerArtifacts, error) {
	var record InstallerArtifacts
	manifest, err := Verify(referencePackage)
	if err != nil {
		return record, err
	}
	root, err := openUpgradeRoot(filepath.Dir(recordPath))
	if err != nil {
		return record, err
	}
	defer root.Close()
	input, err := openInstalledFile(root, filepath.Base(recordPath))
	if err != nil {
		return record, err
	}
	data, readErr := io.ReadAll(io.LimitReader(input, (64<<10)+1))
	if err = errors.Join(readErr, input.Close()); err != nil {
		return record, err
	}
	if len(data) > 64<<10 {
		return record, errors.New("installer artifact record too large")
	}
	if err = configjson.DecodeObject(data, &record); err != nil {
		return record, err
	}
	if record.SchemaVersion != 1 || record.Version != manifest.Version || record.Platform != manifest.Platform || record.TestBuild == nil {
		return record, errors.New("installer artifact identity mismatch")
	}
	if *record.TestBuild && !allowTest {
		return record, errors.New("test installer artifacts are not allowed")
	}
	// This record format accepts the canonical manifest emitted by Build,
	// rather than a hash for unrelated or differently serialized metadata.
	canonical, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return record, err
	}
	sum := sha256.Sum256(canonical)
	if record.PackageManifestSHA256 != hex.EncodeToString(sum[:]) {
		return record, errors.New("installer artifacts belong to a different package")
	}
	prefix := "WinOne-" + manifest.Version + "-" + manifest.Platform
	if *record.TestBuild {
		prefix += "-test"
	}
	for _, item := range []struct {
		file  InstallerArtifactFile
		name  string
		limit int64
	}{
		{record.Installer, prefix + "-setup.exe", maxTotalBytes}, {record.Uninstaller, prefix + "-uninstall.exe", 16 << 20},
	} {
		if item.file.FileName != item.name || item.file.Size <= 0 || item.file.Size > item.limit {
			return record, errors.New("invalid installer artifact file")
		}
		file, err := openInstalledFile(root, item.file.FileName)
		if err != nil {
			return record, err
		}
		digest := sha256.New()
		size, readErr := io.Copy(digest, io.LimitReader(file, item.file.Size+1))
		if err = errors.Join(readErr, file.Close()); err != nil {
			return record, err
		}
		if size != item.file.Size || hex.EncodeToString(digest.Sum(nil)) != item.file.SHA256 {
			return record, errors.New("installer artifact content mismatch")
		}
	}
	return record, nil
}
