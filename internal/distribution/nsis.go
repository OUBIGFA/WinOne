package distribution

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
)

// NSISFileMacros consumes an already verified package manifest. The independent
// path checks also prevent accidental preprocessor/variable interpolation.
// Each macro requires a guard function and a failure label supplied by its
// caller. File failures stop immediately; directory cleanup preserves extras.
func NSISFileMacros(manifest Manifest) (string, error) {
	if manifest.Product != "WinOne" || manifest.Platform != "windows-x64" || manifest.SchemaVersion != 1 || len(manifest.Files) == 0 {
		return "", errors.New("unsupported installer manifest")
	}
	names := []string{}
	seen := map[string]bool{}
	directories := map[string]bool{}
	for _, file := range manifest.Files {
		key := strings.ToLower(file.Path)
		if !safeName(file.Path) || seen[key] || file.Path == "manifest.json" || (!allowed(file.Path) && file.Path != "LICENSE.txt" && file.Path != "README.txt") {
			return "", errors.New("invalid installer file path")
		}
		seen[key] = true
		names = append(names, file.Path)
		for directory := path.Dir(file.Path); directory != "."; directory = path.Dir(directory) {
			directories[directory] = true
		}
	}
	names = append(names, "manifest.json")
	sort.Strings(names)
	escape := func(name string) string { return strings.ReplaceAll(strings.ReplaceAll(name, "$", "$$"), "/", `\`) }
	var output strings.Builder
	guard := func() { output.WriteString("  Call ${GuardFunction}\r\n  IfErrors ${FailureLabel}\r\n") }
	checked := func(operation string) {
		guard()
		output.WriteString("  ClearErrors\r\n  " + operation + "\r\n  IfErrors ${FailureLabel}\r\n")
	}
	// File attributes alone do not reveal a junction in an ancestor directory.
	// Validate every parent before resolving file paths for install or removal.
	validateTargets := func() {
		parents := []string{"."}
		for directory := range directories {
			parents = append(parents, directory)
		}
		sort.Strings(parents)
		for _, directory := range parents {
			guard()
			parent := "$INSTDIR"
			if directory != "." {
				parent += `\` + escape(directory)
			}
			fmt.Fprintf(&output, "  StrCpy $0 \"%s\"\r\n", parent)
			output.WriteString("  System::Call 'kernel32::GetFileAttributesW(w r0) i.r1 ?e'\r\n  Pop $2\r\n  ${If} $1 == -1\r\n    ${If} $2 != 2\r\n    ${AndIf} $2 != 3\r\n      Goto ${FailureLabel}\r\n    ${EndIf}\r\n  ${Else}\r\n    IntOp $1 $1 & 0x410\r\n    ${If} $1 != 16\r\n      Goto ${FailureLabel}\r\n    ${EndIf}\r\n  ${EndIf}\r\n")
		}
		for _, name := range names {
			guard()
			fmt.Fprintf(&output, "  StrCpy $0 \"$INSTDIR\\%s\"\r\n", escape(name))
			// Do not let NSIS silently clear read-only flags or traverse a file
			// replaced with a directory/reparse point since installation.
			output.WriteString("  System::Call 'kernel32::GetFileAttributesW(w r0) i.r1'\r\n  ${If} $1 != -1\r\n    IntOp $1 $1 & 0x411\r\n    ${If} $1 != 0\r\n      Goto ${FailureLabel}\r\n    ${EndIf}\r\n  ${EndIf}\r\n")
		}
	}
	output.WriteString("; Generated from a verified WinOne package. Do not edit.\r\n!include LogicLib.nsh\r\n!macro WinOneInstallFiles GuardFunction FailureLabel\r\n")
	validateTargets()
	previous := ""
	for _, name := range names {
		directory := path.Dir(name)
		if directory != previous {
			destination := "$INSTDIR"
			if directory != "." {
				destination += `\` + escape(directory)
			}
			checked(fmt.Sprintf("SetOutPath \"%s\"", destination))
			previous = directory
		}
		// File's source is resolved at compile time and treats dollar signs
		// literally. Runtime destination/delete strings use NSIS escaping.
		checked(fmt.Sprintf("File \"${WINONE_PAYLOAD_DIR}\\%s\"", strings.ReplaceAll(name, "/", `\`)))
	}
	guard()
	output.WriteString("!macroend\r\n\r\n!macro WinOneRemoveFiles GuardFunction FailureLabel\r\n")
	validateTargets()
	// Probe the entire inventory before deleting its first entry. This catches
	// pre-existing sharing/permission failures without leaving an earlier prefix
	// deleted. Each actual Delete still checks errors: external state may change.
	for _, name := range names {
		guard()
		fmt.Fprintf(&output, "  StrCpy $0 \"$INSTDIR\\%s\"\r\n", escape(name))
		output.WriteString("  System::Call 'kernel32::CreateFileW(w r0, i 0x10000, i 7, p 0, i 3, i 0x80, p 0) p.r1 ?e'\r\n  Pop $2\r\n")
		output.WriteString("  ${If} $1 == -1\r\n    ${If} $2 != 2\r\n    ${AndIf} $2 != 3\r\n      Goto ${FailureLabel}\r\n    ${EndIf}\r\n  ${Else}\r\n    System::Call 'kernel32::CloseHandle(p r1)'\r\n  ${EndIf}\r\n")
	}
	for _, name := range names {
		checked(fmt.Sprintf("Delete \"$INSTDIR\\%s\"", escape(name)))
	}
	guard()
	output.WriteString("!macroend\r\n\r\n; Non-empty directories are intentionally preserved.\r\n!macro WinOneRemoveEmptyDirectories GuardFunction FailureLabel\r\n")
	dirs := make([]string, 0, len(directories))
	for directory := range directories {
		dirs = append(dirs, directory)
	}
	sort.Slice(dirs, func(i, j int) bool {
		a, b := strings.Count(dirs[i], "/"), strings.Count(dirs[j], "/")
		if a != b {
			return a > b
		}
		return dirs[i] > dirs[j]
	})
	for _, directory := range dirs {
		guard()
		fmt.Fprintf(&output, "  RMDir \"$INSTDIR\\%s\"\r\n", escape(directory))
	}
	guard()
	output.WriteString("  RMDir \"$INSTDIR\"\r\n")
	guard()
	output.WriteString("!macroend\r\n")
	return output.String(), nil
}
