package model

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// The path helpers port Cindy's localPathResolver.ts (69661422, Apache-2.0)
// and WinOne's chat-file-navigation.tsx target(): pure string rules that
// decide whether a reply's link or inline code names a local file, and
// which one. The Host stats and opens the result; nothing here touches the
// file system.

var (
	imageExt           = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|webp|svg|bmp|ico)(\?.*)?$`)
	modelExt           = regexp.MustCompile(`(?i)\.(glb|gltf|fbx)(\?.*)?$`)
	hasExt             = regexp.MustCompile(`(?i)\.[a-z0-9]{1,10}(\?.*)?$`)
	windowsAbsolute    = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	posixAbsoluteFile  = regexp.MustCompile(`(?i)^/[^/\s]\S*\.[a-z0-9]{1,10}(\?\S*)?$`)
	relativeFile       = regexp.MustCompile(`(?i)^[^\s:]*[\\/][^\s:]+\.[a-z0-9]{1,10}(\?\S*)?$`)
	urlScheme          = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://`)
	anyScheme          = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*:`)
	windowsFileURLPath = regexp.MustCompile(`^/[A-Za-z]:[\\/]`)
	// coordinates are a viewer's line and column after a path: GitHub's
	// #L12C3 and the path:12:3 agents write.
	coordinates = regexp.MustCompile(`(?:#L|:)([1-9]\d*)(?:(?:C|:)([1-9]\d*))?(?:-L?\d+)?$`)
)

// Href kinds, as classifyMarkdownHref names them.
const (
	HrefExternal  = "external"
	HrefDirectory = "directory"
	HrefImage     = "image-local"
	HrefModel     = "model-local"
	HrefText      = "text-local"
)

// LooksLikeDirectoryPath reports a path written as a folder: with a
// trailing separator.
func LooksLikeDirectoryPath(text string) bool {
	return strings.HasSuffix(text, "/") || strings.HasSuffix(text, `\`)
}

// LooksLikeFilePath reports inline code that is unambiguously a file
// path: a Windows absolute path, a POSIX absolute path with an extension,
// or a relative path with a separator and an extension. Identifiers,
// commands and bare names stay code.
func LooksLikeFilePath(text string) bool {
	if text == "" || strings.Contains(text, "\n") || urlScheme.MatchString(text) || LooksLikeDirectoryPath(text) {
		return false
	}
	return windowsAbsolute.MatchString(text) || posixAbsoluteFile.MatchString(text) || relativeFile.MatchString(text)
}

// ClassifyHref routes a Markdown link: web and in-document links are
// external; file:// and path-shaped links are local, a folder, an image, a
// 3D model or text.
func ClassifyHref(href string) string {
	if href == "" || strings.HasPrefix(href, "#") {
		return HrefExternal
	}
	lower := strings.ToLower(href)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return HrefExternal
	}
	if urlScheme.MatchString(href) && !strings.HasPrefix(href, "file://") {
		return HrefExternal
	}
	probe := strings.TrimPrefix(href, "file://")
	local := strings.HasPrefix(probe, "/") || windowsAbsolute.MatchString(probe) || strings.ContainsAny(probe, `/\`) || hasExt.MatchString(probe)
	switch {
	case !local:
		return HrefExternal
	case LooksLikeDirectoryPath(probe):
		return HrefDirectory
	case imageExt.MatchString(probe):
		return HrefImage
	case modelExt.MatchString(probe):
		return HrefModel
	}
	return HrefText
}

func decodeComponent(value string) string {
	if decoded, err := url.PathUnescape(value); err == nil {
		return decoded
	}
	return value
}

func stripTrailingSeparators(value string) string { return strings.TrimRight(value, `/\`) }

// ResolveLocalPath makes a link or reference absolute: file:// URLs are
// decoded, absolute and UNC paths kept, relative ones joined to cwd in its
// separator style.
func ResolveLocalPath(href, cwd string) string {
	if strings.HasPrefix(href, "file://") {
		path := decodeComponent(href[len("file://"):])
		if windowsFileURLPath.MatchString(path) {
			path = path[1:]
		}
		return path
	}
	if strings.HasPrefix(href, "/") || windowsAbsolute.MatchString(href) || strings.HasPrefix(href, `\\`) {
		return href
	}
	sep := "/"
	if strings.Contains(cwd, `\`) {
		sep = `\`
		href = strings.ReplaceAll(href, "/", `\`)
	}
	return stripTrailingSeparators(cwd) + sep + href
}

// ResolveToolFilePath makes a tool's file argument absolute against the
// conversation's folder, as the agent resolved it; without a folder it
// stays as written.
func ResolveToolFilePath(path, cwd string) string {
	if path == "" || cwd == "" {
		return path
	}
	return ResolveLocalPath(path, cwd)
}

func basename(value string) string {
	value = stripTrailingSeparators(value)
	if i := strings.LastIndexAny(value, `/\`); i >= 0 {
		return value[i+1:]
	}
	return value
}

func knownName(href string) string {
	href = strings.TrimPrefix(strings.TrimSpace(href), "file://")
	if i := strings.IndexByte(href, '#'); i >= 0 {
		href = href[:i]
	}
	if i := strings.IndexByte(href, '?'); i >= 0 {
		href = href[:i]
	}
	return strings.ToLower(basename(decodeComponent(href)))
}

// ResolveKnownFile finds the one attachment of the conversation a bare
// file name cites, case-insensitively, as models cite uploads by name; ""
// when none or several match.
func ResolveKnownFile(href string, files []Attachment) string {
	wanted := knownName(href)
	if wanted == "" {
		return ""
	}
	match := ""
	for _, f := range files {
		if knownName(f.Name) == wanted || knownName(f.Path) == wanted {
			if match != "" {
				return ""
			}
			match = f.Path
		}
	}
	return match
}

// LinkTarget is where a reply's link or inline path leads.
type LinkTarget struct {
	// Kind is one of the Href kinds.
	Kind         string
	Path         string
	Line, Column int
}

// Local reports a target in the file system.
func (t LinkTarget) Local() bool { return t.Kind != HrefExternal }

// TargetOf resolves href, with an optional :line[:column] or #L12C3, as
// the React window's chat-file-navigation target(): a bare name resolves
// to the conversation's attachment of that name first; explicit paths and
// web addresses never do.
func TargetOf(href, cwd string, files []Attachment) LinkTarget {
	var t LinkTarget
	raw := href
	if m := coordinates.FindStringSubmatchIndex(href); m != nil {
		raw = href[:m[0]]
		t.Line, _ = strconv.Atoi(href[m[2]:m[3]])
		if m[4] >= 0 {
			t.Column, _ = strconv.Atoi(href[m[4]:m[5]])
		}
	}
	decoded := raw
	if !strings.HasPrefix(raw, "file://") {
		decoded = decodeComponent(raw)
	}
	if !strings.ContainsAny(decoded, `/\`) && !anyScheme.MatchString(decoded) {
		if known := ResolveKnownFile(decoded, files); known != "" {
			t.Kind, t.Path = HrefText, known
			return t
		}
	}
	t.Kind = ClassifyHref(decoded)
	if strings.HasPrefix(decoded, "file://") {
		t.Path = ResolveLocalPath(decoded, cwd)
	} else {
		t.Path = ResolveToolFilePath(decoded, cwd)
	}
	return t
}

// InlinePathCandidate reports inline code that may name a local file or
// folder, to confirm with the Host before it becomes a file chip.
func InlinePathCandidate(text, cwd string, files []Attachment) (LinkTarget, bool) {
	if text == "" || strings.ContainsAny(text, "\r\n") {
		return LinkTarget{}, false
	}
	t := TargetOf(text, cwd, files)
	if !t.Local() {
		return t, false
	}
	bare := coordinates.ReplaceAllString(text, "")
	return t, LooksLikeFilePath(bare) || LooksLikeDirectoryPath(text) || strings.HasPrefix(strings.ToLower(text), "file://")
}
