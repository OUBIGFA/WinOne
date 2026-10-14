package startupprobe

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"winone/internal/catalog"
)

// ProfileSnapshot binds only canonical/control data. It contains no runtime
// objects and never creates or upgrades a profile. The target Host separately
// validates the signed packages referenced by Registry.
type ProfileSnapshot struct {
	Configuration      []byte
	ModuleState        []byte
	Registry           []byte
	Reclamation        []byte
	RuntimeRegistry    []byte
	RuntimeReclamation []byte
}

// A legacy target can keep using the original configuration-only contract for
// builtin profiles. Optional data requires the new explicit profile contract;
// it is never silently accepted through a less capable validator.
func (p ProfileSnapshot) RequiresPackageCheck() bool {
	if p.Registry != nil || p.Reclamation != nil || p.RuntimeRegistry != nil || p.RuntimeReclamation != nil {
		return true
	}
	var data struct {
		Committed struct {
			Values          map[string]json.RawMessage
			ModuleEnabled   map[string]bool
			RetainedModules map[string]json.RawMessage
		}
	}
	if json.Unmarshal(p.Configuration, &data) != nil {
		return false
	}
	for id := range data.Committed.Values {
		if catalog.ValidOptionalID(id) {
			return true
		}
	}
	for id := range data.Committed.ModuleEnabled {
		if catalog.ValidOptionalID(id) {
			return true
		}
	}
	for id := range data.Committed.RetainedModules {
		if catalog.ValidOptionalID(id) {
			return true
		}
	}
	return false
}

func ReadProfileSnapshot(directory string) (ProfileSnapshot, error) {
	var result ProfileSnapshot
	if !filepath.IsAbs(directory) {
		return result, errors.New("profile probe requires an absolute directory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return result, err
	}
	defer root.Close()
	read := func(name string, limit int64) ([]byte, error) {
		info, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > limit {
			return nil, errors.New("profile control file has an invalid type or size")
		}
		file, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
		err = errors.Join(readErr, file.Close())
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > limit {
			return nil, errors.New("profile control file exceeds size limit")
		}
		return data, nil
	}
	for _, file := range []struct {
		name   string
		limit  int64
		target *[]byte
	}{
		{"configuration.json", 8 << 20, &result.Configuration},
		{"module-state.json", 1 << 20, &result.ModuleState},
		{"packages/.winone-module-registry.json", 1 << 20, &result.Registry},
		{"packages/.winone-module-reclaim.json", 1 << 20, &result.Reclamation},
		{"packages/shared-runtimes/.winone-runtime-registry.json", 1 << 20, &result.RuntimeRegistry},
		{"packages/shared-runtimes/.winone-module-reclaim.json", 1 << 20, &result.RuntimeReclamation},
	} {
		data, err := read(file.name, file.limit)
		if err != nil {
			return result, err
		}
		*file.target = data
	}
	return result, nil
}

func (p ProfileSnapshot) Digest() string {
	sum := sha256.New()
	files := [][]byte{p.Configuration, p.ModuleState, p.Registry, p.Reclamation}
	// Preserve the existing probe digest for profiles without shared storage,
	// including upgrades initiated by a previous installed Host.
	if p.RuntimeRegistry != nil || p.RuntimeReclamation != nil {
		files = append(files, p.RuntimeRegistry, p.RuntimeReclamation)
	}
	for _, data := range files {
		present := byte(1)
		if data == nil {
			present = 0
		}
		_, _ = sum.Write([]byte{present})
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(data)))
		_, _ = sum.Write(size[:])
		_, _ = sum.Write(data)
	}
	return hex.EncodeToString(sum.Sum(nil))
}
