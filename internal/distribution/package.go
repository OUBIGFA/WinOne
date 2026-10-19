// Package distribution defines the checked runtime payload shared by local ZIP
// packaging and the future installer. Hashes detect damage, not publisher trust.
package distribution

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
	"winone/internal/configjson"
)

const maxFileBytes = int64(256 << 20)
const maxTotalBytes = int64(2 << 30)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$`)
var culturePattern = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[a-zA-Z0-9]{2,8})*$`)
var devicePattern = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9¹²³]|LPT[1-9¹²³])$`)
var requiredFiles = []string{"WinOne.exe", "WinOne.Host.exe", "WinOne.WinTray.exe", "always-on-top/WinOne.AlwaysOnTop.exe", "always-on-top/WinOne.AlwaysOnTop.Native.dll", "capture/WinOne.Capture.exe", "capture/WinOne.Capture.Regions.exe", "capture/WinOne.Capture.dll", "capture/WinOne.Capture.deps.json", "capture/WinOne.Capture.runtimeconfig.json", "capture/coreclr.dll", "capture/System.Private.CoreLib.dll", "capture/Licenses/ShareX_license.txt"}

type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	Product       string `json:"product"`
	Version       string `json:"version"`
	Platform      string `json:"platform"`
	EntryPoint    string `json:"entryPoint"`
	Files         []File `json:"files"`
}
type input struct {
	name, source string
	data         []byte
}

func safeName(name string) bool {
	if name == "" || len(name) > 1024 || !utf8.ValidString(name) || strings.ContainsAny(name, `\:<>"|?*`) || strings.HasPrefix(name, "/") || path.Clean(name) != name || name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return false
	}
	for _, r := range name {
		if r < 32 {
			return false
		}
	}
	for _, part := range strings.Split(name, "/") {
		if strings.TrimRight(part, " .") != part || devicePattern.MatchString(strings.TrimRight(strings.Split(part, ".")[0], " ")) {
			return false
		}
	}
	return true
}

// ValidPayloadPath shares the Windows-safe canonical path rules with module
// packages. It validates a name only, not filesystem containment or symlinks.
func ValidPayloadPath(name string) bool { return safeName(name) }

func PackageName(version string) (string, error) {
	if !versionPattern.MatchString(version) {
		return "", errors.New("invalid package version")
	}
	return "WinOne-" + version + "-windows-x64.zip", nil
}
func allowed(name string) bool {
	// Accept only the named result runtime; older verified 0.0.1 payloads
	// remain valid for upgrade/rollback without this newly added component.
	if name == "shared/ui/WinOne.Results.exe" || name == "shared/ui/WinOne.Diagrams.exe" || name == "selection/WinOne.Selection.exe" || name == "WinOne.WinTray.Recovery.exe" {
		return true
	}
	// Both architectures are shipped beside WinTray. Older verified bundles
	// remain readable for rollback; the current worker verifies all four files.
	switch name {
	case "native/x64/WinTray-Bridge.exe", "native/x64/WinTray-ShellBridge.dll", "native/x86/WinTray-Bridge.exe", "native/x86/WinTray-ShellBridge.dll":
		return true
	}
	if name == "capture/recording-runtime.json" { return true }
	if textRuntimeAllowed[name] {
		return true
	}
	for _, file := range strokeRuntimeFiles {
		if name == file {
			return true
		}
	}
	for _, required := range requiredFiles {
		if name == required {
			return true
		}
	}
	parts := strings.Split(name, "/")
	if len(parts) == 2 && parts[0] == "licenses" && strings.HasSuffix(parts[1], ".txt") {
		return true
	}
	if parts[0] != "capture" {
		return false
	}
	if len(parts) == 2 {
		return strings.HasSuffix(parts[1], ".dll") || parts[1] == "createdump.exe"
	}
	if len(parts) == 3 {
		if parts[1] == "Licenses" {
			return strings.HasSuffix(parts[2], ".txt")
		}
		return culturePattern.MatchString(parts[1]) && strings.HasSuffix(parts[2], ".resources.dll")
	}
	return false
}

// Only dependency inventories evolve independently of the package format.
// Keep root notes/configuration and unknown module roots outside upgrade scope.
func allowedHistorical(name string) bool {
	return allowed(name) || name == "LICENSE.txt" || name == "README.txt" ||
		strings.HasPrefix(name, "capture/Ocr/") || strings.HasPrefix(name, "shared/ai/") || strings.HasPrefix(name, "shared/chat/")
}

func inputs(directory, license string) ([]input, error) {
	var result []input
	seen := map[string]bool{}
	err := filepath.WalkDir(directory, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("package source contains a link")
		}
		relative, err := filepath.Rel(directory, filePath)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if entry.IsDir() {
			if name != "." {
				top := strings.Split(name, "/")[0]
				if top != "capture" && top != "always-on-top" && top != "licenses" && top != "shared" && top != "selection" && top != "stroke" && top != "native" {
					return fmt.Errorf("unexpected directory: %s", name)
				}
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("not a regular file: %s", name)
		}
		if name == "build-info.json" {
			return nil
		}
		if strings.HasPrefix(name, "capture/") && strings.HasSuffix(name, ".pdb") {
			return nil
		}
		if !safeName(name) || !allowed(name) {
			return fmt.Errorf("unexpected runtime file: %s", name)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate path: %s", name)
		}
		seen[key] = true
		result = append(result, input{name: name, source: filePath})
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, required := range requiredForPayload(seen) {
		if !seen[strings.ToLower(required)] {
			return nil, fmt.Errorf("required file missing: %s", required)
		}
	}
	info, err := os.Lstat(license)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("license must be a regular file")
	}
	result = append(result, input{name: "LICENSE.txt", source: license})
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result, nil
}

// Build refuses unknown payload files and existing output. Interrupted output
// stays named .partial; publication occurs only after reading the archive back.
func Build(directory, license, destination, version string) (err error) {
	if !versionPattern.MatchString(version) {
		return errors.New("invalid package version")
	}
	files, err := inputs(directory, license)
	if err != nil {
		return err
	}
	build, err := ReadBuildInfo(directory)
	if err != nil {
		return err
	}
	if build.Version != version {
		return errors.New("package version does not match the verified build")
	}
	expected := map[string]File{}
	for _, record := range build.Files {
		if _, duplicate := expected[record.Path]; duplicate {
			return errors.New("duplicate build inventory path")
		}
		expected[record.Path] = record
	}
	if len(expected) != len(files) {
		return errors.New("build contents changed; rebuild before packaging")
	}
	files = append(files, input{name: "README.txt", data: []byte("WinOne " + version + "\nWindows 11 x64\n\n运行 WinOne.exe。需要 Microsoft Edge WebView2 Runtime。\n设置保存在 %LOCALAPPDATA%\\WinOne。\n\nRun WinOne.exe. Microsoft Edge WebView2 Runtime is required.\nSettings are stored in %LOCALAPPDATA%\\WinOne.\n")})
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	if len(files) >= 8192 {
		return errors.New("too many package files")
	}
	if _, err = os.Stat(destination); !os.IsNotExist(err) {
		if err == nil {
			return errors.New("package already exists")
		}
		return err
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	partial := destination + ".partial"
	output, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	archive := zip.NewWriter(output)
	manifest := newManifest(version)
	var total int64
	for _, file := range files {
		entry, err := writeFile(archive, file)
		if err != nil {
			archive.Close()
			return err
		}
		if file.name != "README.txt" && expected[file.name] != entry {
			archive.Close()
			return fmt.Errorf("build file changed: %s", file.name)
		}
		total += entry.Size
		if total > maxTotalBytes {
			archive.Close()
			return errors.New("package exceeds size limit")
		}
		manifest.Files = append(manifest.Files, entry)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if _, err = writeFile(archive, input{name: "manifest.json", data: data}); err != nil {
		return err
	}
	if err = archive.Close(); err != nil {
		return err
	}
	if err = output.Sync(); err != nil {
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	if _, err = Verify(partial); err != nil {
		return err
	}
	// Link publishes without replacing another writer's output; removing our
	// temporary link does not remove the now-published archive contents.
	if err = os.Link(partial, destination); err != nil {
		return err
	}
	return os.Remove(partial)
}

func writeFile(archive *zip.Writer, file input) (File, error) {
	header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
	header.SetMode(0644)
	header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return File{}, err
	}
	return hashInput(file, writer)
}

func hashInput(file input, writer io.Writer) (File, error) {
	var source io.Reader = bytes.NewReader(file.data)
	if file.source != "" {
		stream, err := os.Open(file.source)
		if err != nil {
			return File{}, err
		}
		defer stream.Close()
		info, err := stream.Stat()
		if err != nil {
			return File{}, err
		}
		if !info.Mode().IsRegular() || info.Size() > maxFileBytes {
			return File{}, errors.New("invalid runtime file")
		}
		source = stream
	}
	digest := sha256.New()
	size, err := io.Copy(io.MultiWriter(writer, digest), io.LimitReader(source, maxFileBytes+1))
	if err != nil {
		return File{}, err
	}
	if size > maxFileBytes {
		return File{}, errors.New("file exceeds size limit")
	}
	return File{file.name, size, hex.EncodeToString(digest.Sum(nil))}, nil
}

func Verify(filename string) (Manifest, error) { return verifyPackage(filename, false) }

// Historical OCR/AI/Chat inventories describe the exact old distribution.
// Dependency upgrades rename files and add requirements: those current-build
// rules cannot invalidate a hash-verified source/rollback package. New packages
// still go through Verify, Build and the current complete runtime inventories.
func verifyHistoricalPackage(filename string) (Manifest, error) { return verifyPackage(filename, true) }

func verifyPackage(filename string, historical bool) (Manifest, error) {
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return Manifest{}, err
	}
	defer archive.Close()
	if len(archive.File) > 8192 {
		return Manifest{}, errors.New("too many package entries")
	}
	entries := map[string]*zip.File{}
	seen := map[string]bool{}
	for _, entry := range archive.File {
		key := strings.ToLower(entry.Name)
		if !safeName(entry.Name) || seen[key] || !entry.Mode().IsRegular() {
			return Manifest{}, errors.New("invalid or duplicate package path")
		}
		entries[entry.Name] = entry
		seen[key] = true
	}
	index := entries["manifest.json"]
	if index == nil || index.UncompressedSize64 > 1<<20 {
		return Manifest{}, errors.New("package manifest missing or too large")
	}
	reader, err := index.Open()
	if err != nil {
		return Manifest{}, err
	}
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	reader.Close()
	if err != nil || len(data) > 1<<20 {
		return Manifest{}, errors.New("invalid package manifest")
	}
	var manifest Manifest
	if err = configjson.DecodeObject(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.SchemaVersion != 1 || manifest.Product != "WinOne" || manifest.Platform != "windows-x64" || manifest.EntryPoint != "WinOne.exe" || !versionPattern.MatchString(manifest.Version) {
		return Manifest{}, errors.New("unsupported package manifest")
	}
	if len(manifest.Files)+1 != len(entries) {
		return Manifest{}, errors.New("package inventory mismatch")
	}
	checked := map[string]bool{}
	var total int64
	for _, record := range manifest.Files {
		entry := entries[record.Path]
		pathAllowed := allowed(record.Path) || record.Path == "LICENSE.txt" || record.Path == "README.txt"
		if historical {
			pathAllowed = allowedHistorical(record.Path)
		}
		if entry == nil || checked[record.Path] || record.Path == "manifest.json" || !pathAllowed || record.Size < 0 || record.Size > maxFileBytes || entry.UncompressedSize64 != uint64(record.Size) {
			return Manifest{}, errors.New("invalid package inventory")
		}
		total += record.Size
		if total > maxTotalBytes {
			return Manifest{}, errors.New("package exceeds size limit")
		}
		stream, err := entry.Open()
		if err != nil {
			return Manifest{}, err
		}
		digest := sha256.New()
		size, err := io.Copy(digest, io.LimitReader(stream, record.Size+1))
		stream.Close()
		if err != nil || size != record.Size || hex.EncodeToString(digest.Sum(nil)) != record.SHA256 {
			return Manifest{}, fmt.Errorf("package checksum mismatch: %s", record.Path)
		}
		checked[record.Path] = true
	}
	required := requiredFiles
	if !historical {
		required = requiredForPayload(checked)
	}
	for _, required := range append(append([]string{}, required...), "LICENSE.txt", "README.txt") {
		if !checked[required] {
			return Manifest{}, fmt.Errorf("required file missing: %s", required)
		}
	}
	return manifest, nil
}
