package distribution

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"winone/internal/configjson"
)

func newManifest(version string) Manifest {
	return Manifest{SchemaVersion: 1, Product: "WinOne", Version: version, Platform: "windows-x64", EntryPoint: "WinOne.exe"}
}

// RecordBuild is called only after the build script's checks pass. This is an
// inventory of that build, not a signature or independent test attestation.
func RecordBuild(directory, license, version string) error {
	if !versionPattern.MatchString(version) {
		return errors.New("invalid build version")
	}
	files, err := inputs(directory, license)
	if err != nil {
		return err
	}
	manifest := newManifest(version)
	for _, file := range files {
		record, err := hashInput(file, io.Discard)
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, record)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("build inventory too large")
	}
	file, err := os.CreateTemp(directory, ".build-info-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(directory, "build-info.json"))
}

func ReadBuildInfo(directory string) (Manifest, error) {
	path := filepath.Join(directory, "build-info.json")
	info, err := os.Lstat(path)
	if err != nil {
		return Manifest{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return Manifest{}, errors.New("invalid build inventory file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > 1<<20 {
		return Manifest{}, errors.New("build inventory too large")
	}
	var manifest Manifest
	if err = configjson.DecodeObject(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.SchemaVersion != 1 || manifest.Product != "WinOne" || manifest.Platform != "windows-x64" || manifest.EntryPoint != "WinOne.exe" || !versionPattern.MatchString(manifest.Version) || len(manifest.Files) == 0 {
		return Manifest{}, errors.New("unsupported build inventory")
	}
	return manifest, nil
}
