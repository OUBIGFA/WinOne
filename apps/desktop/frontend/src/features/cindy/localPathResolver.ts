// Pure path helpers reused from Cindy 69661422, Apache-2.0. Host owns file I/O.
/**
 * localPathResolver
 * ---------------------------------------------------------------------------
 * Pure-string utilities to classify a markdown link / inline @-chip ref and
 * resolve it to an absolute filesystem path or a `file://` URL.
 *
 * Used by:
 *   - F1 (MarkdownRenderer): split <a href="..."> into external / image-local
 *     / text-local and route to openExternal / ImageLightbox / TextLightbox.
 *   - F2 (UserMessage): turn an inline @-chip ref ("src/App.tsx") into the
 *     absolute path that TextLightbox needs to read.
 *
 * Design notes:
 *   - Sandbox-safe: no `node:path`, no `node:fs`, no IPC. The renderer layer
 *     can't import Node modules, so all path manipulation is pure regex /
 *     string slicing.
 *   - No fs.exists check. If the file is missing, the lightbox's own onError
 *     toast handles the failure.
 *   - Cross-platform: handles Windows backslashes AND POSIX slashes in both
 *     the `cwd` (session.workingDir) and the `href`. The decision uses the
 *     cwd's separator style.
 */

import { stripTrailingPathSeparators } from './pathText';

/** Image extension whitelist. Case-insensitive. Allows `?v=2`-style suffix. */
const IMAGE_EXT_RE = /\.(png|jpe?g|gif|webp|svg|bmp|ico)(\?.*)?$/i;
/** 3D model extension whitelist. .glb/.gltf open the in-app ModelLightbox;
 *  .fbx is classified as model too but activates as reveal-in-Finder (no
 *  in-app FBX preview, and openPath would hit macOS Preview's misleading
 *  "damaged file" dialog). Must be checked BEFORE the text fallback in
 *  classifyMarkdownHref — .gltf is JSON underneath and would otherwise be
 *  swallowed by 'text-local'. */
export const MODEL_EXT_RE = /\.(glb|gltf|fbx)(\?.*)?$/i;
/** Has any 1-10 char file extension (e.g. `.tsx`, `.md`, `.txt`). */
const HAS_EXT_RE = /\.[a-z0-9]{1,10}(\?.*)?$/i;
/** Windows absolute path: `C:\foo` or `C:/foo`. */
const WIN_ABS_RE = /^[A-Za-z]:[\\/]/;
/** POSIX absolute path that ends in a file extension (excludes bare `/etc`). */
const POSIX_ABS_PATH_RE = /^\/[^/\s][^\s]*\.[a-z0-9]{1,10}(\?[^\s]*)?$/i;
/** Relative path: at least one separator + ends in extension + no whitespace + no colon. */
const REL_PATH_WITH_SEP_AND_EXT_RE = /^[^\s:]*[\\/][^\s:]+\.[a-z0-9]{1,10}(\?[^\s]*)?$/i;
/** Any URL scheme (http://, https://, file://, ftp://, ...). */
const URL_SCHEME_RE = /^[a-z][a-z0-9+.-]*:\/\//i;
/** Trailing path separator — distinguishes a directory ref from a file ref. */
const TRAILING_SEP_RE = /[\\/]$/;

export type LocalHrefKind =
  | 'image-local'
  | 'model-local'
  | 'text-local'
  | 'external'
  | 'directory';
export interface KnownLocalFileRef {
  name: string;
  path: string;
}

/**
 * Detect whether a path-shaped string refers to a directory rather than a file.
 *
 * Heuristic: trailing `/` or `\`. Markdown / chat content that wants to point
 * at a folder almost always writes `src/components/`, `./docs/`, `C:\Users\`,
 * etc. We don't (and can't, in renderer) stat the filesystem, so the trailing
 * separator is the only reliable signal we have.
 *
 * Used by both `looksLikeFilePath` (to reject directory shapes upstream) and
 * `classifyMarkdownHref` (to surface the 'directory' kind so callers can
 * choose not to open a file lightbox on a folder click).
 *
 * Pure function. Never throws. No IO.
 */
export function looksLikeDirectoryPath(text: string): boolean {
  if (!text) return false;
  return TRAILING_SEP_RE.test(text);
}

/**
 * Classify a markdown `<a href>` for routing.
 *
 * Decision order (first hit wins):
 *   1. empty / undefined        → 'external' (defensive — caller no-ops)
 *   2. ^https?://               → 'external'
 *   3. ^file:// (then byExt of remainder)
 *   4. POSIX absolute (`^/`)    → byExt
 *   5. Windows absolute         → byExt
 *   6. contains `/` or `\`      → byExt
 *   7. has file extension       → byExt (bare `App.tsx` etc.)
 *   8. otherwise                → 'external' (plain word like `hello`)
 *
 * byExt: IMAGE_EXT_RE → 'image-local', else 'text-local'.
 *
 * Pure function. Never throws. No IO.
 */
/**
 * Decide whether an inline-code text node should be treated as a clickable
 * filesystem path. Stricter than `classifyMarkdownHref`: false positives here
 * are costly because inline code commonly contains identifiers / commands /
 * flags (`useState`, `npm run build`, `--filter desktop`) that would turn
 * into broken click targets if we matched too liberally.
 *
 * Match policy (any one is enough):
 *   1. Windows absolute path (`C:\foo`, `D:/bar`) — drive letter is unambiguous
 *   2. POSIX absolute with file extension (`/Users/me/foo.md`)
 *      — bare `/etc` or `/dev/null` (no dotted extension) does NOT match
 *   3. Relative path with separator + extension + no whitespace + no colon
 *      (`src/App.tsx`, `./foo.md`, `../sib/foo.json`)
 *      — bare `package.json` (no separator) does NOT match (too ambiguous to
 *      know which one in the workspace)
 *
 * URL schemes (`http://`, `file://`, `git+ssh://`, ...) are always rejected;
 * they're URLs, not local paths, even though some resolve to files.
 *
 * Cross-platform: works the same on Windows, macOS, and Linux. A Mac user
 * clicking a Windows-absolute path will get a "file not found" toast from
 * TextLightbox — same failure mode as a markdown link to that path.
 *
 * Pure function. Never throws. No IO.
 */
export function looksLikeFilePath(text: string): boolean {
  if (!text) return false;
  if (text.includes('\n')) return false;
  if (URL_SCHEME_RE.test(text)) return false;
  // Directory shapes (trailing `/` or `\`) are not files; reject early so
  // inline-code chips like `src/components/` or `C:\Users\` don't become
  // TextLightbox triggers. The Windows absolute branch below would otherwise
  // accept `C:\Users\` because WIN_ABS_RE only checks the prefix.
  if (looksLikeDirectoryPath(text)) return false;
  if (WIN_ABS_RE.test(text)) return true;
  if (POSIX_ABS_PATH_RE.test(text)) return true;
  if (REL_PATH_WITH_SEP_AND_EXT_RE.test(text)) return true;
  return false;
}

export function classifyMarkdownHref(href: string | undefined): LocalHrefKind {
  if (!href) return 'external';
  if (/^https?:\/\//i.test(href)) return 'external';
  // In-document anchors (`#section`, `#v1.2`) are scroll targets, not paths.
  // A dotted fragment like `#v1.2` would otherwise trip HAS_EXT_RE below and
  // misclassify as 'text-local'. Routing belongs to the renderer's anchor
  // scroll handler, so surface as 'external' and let it fall through there.
  if (href.startsWith('#')) return 'external';
  // Any URL scheme other than file:// is a non-local target — http(s), and our
  // internal deep links (cindy://session/<uuid>, xdt-audio://...). file://
  // is the one scheme that maps to a local path and is unwrapped below. This
  // mirrors the URL_SCHEME_RE rejection in looksLikeFilePath so the two
  // predicates stay symmetric: without it a session deep link (contains `/`,
  // no extension) slips into the local-file branch and the renderer opens a
  // missing-file lightbox instead of navigating.
  if (URL_SCHEME_RE.test(href) && !href.startsWith('file://')) return 'external';

  let probe = href;
  if (probe.startsWith('file://')) probe = probe.slice(7);

  const looksLocal =
    probe.startsWith('/') ||
    WIN_ABS_RE.test(probe) ||
    probe.includes('/') ||
    probe.includes('\\') ||
    HAS_EXT_RE.test(probe);

  if (!looksLocal) return 'external';
  // A trailing separator on a local-shaped path means the author is pointing
  // at a folder, not a file. Surface as its own kind so renderers can opt out
  // of file-preview behavior (a TextLightbox on a directory makes no sense
  // and the IPC read would just fail with EISDIR).
  if (looksLikeDirectoryPath(probe)) return 'directory';
  if (IMAGE_EXT_RE.test(probe)) return 'image-local';
  // Model check must precede the text fallback: .gltf is a JSON file and
  // would otherwise classify as 'text-local'.
  if (MODEL_EXT_RE.test(probe)) return 'model-local';
  return 'text-local';
}

function safeDecodeURIComponent(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function stripQueryAndHash(value: string): string {
  const hashIdx = value.indexOf('#');
  const beforeHash = hashIdx >= 0 ? value.slice(0, hashIdx) : value;
  const queryIdx = beforeHash.indexOf('?');
  return queryIdx >= 0 ? beforeHash.slice(0, queryIdx) : beforeHash;
}

function basenameFromPathLike(value: string): string {
  const slash = Math.max(value.lastIndexOf('/'), value.lastIndexOf('\\'));
  return slash >= 0 ? value.slice(slash + 1) : value;
}

function normalizeKnownFileHref(href: string): string {
  const strippedScheme = href.startsWith('file://') ? href.slice(7) : href;
  const decoded = safeDecodeURIComponent(stripQueryAndHash(strippedScheme.trim()));
  return basenameFromPathLike(decoded);
}

/**
 * Resolve a model-authored markdown link against files the user attached in
 * this chat session. LLMs often cite an uploaded document as
 * `[name.docx](name.docx)` because the prompt exposes the original filename,
 * while the real file lives outside the session working directory. In that
 * case, resolve by unique attachment basename before falling back to cwd-based
 * path resolution.
 *
 * Matching is case-insensitive. Windows / macOS-default filesystems are
 * case-insensitive, and LLMs frequently lowercase the basename when authoring
 * the link (e.g. uploaded `Report.docx` cited as `report.docx`). Linux pays a
 * tiny false-positive risk in exchange — the candidate set is the current
 * session's uploads only, and unique-match still applies, so accidental
 * collisions are negligible.
 *
 * Ambiguous duplicate names intentionally return null so callers keep the
 * existing workspace resolver behavior instead of opening the wrong file.
 */
export function resolveKnownLocalFileHref(
  href: string,
  files: readonly KnownLocalFileRef[] | undefined,
): string | null {
  if (!href || !files || files.length === 0) return null;
  const wanted = normalizeKnownFileHref(href).toLowerCase();
  if (!wanted) return null;

  const matches = files.filter((file) => {
    const name = normalizeKnownFileHref(file.name).toLowerCase();
    const pathName = normalizeKnownFileHref(file.path).toLowerCase();
    return wanted === name || wanted === pathName;
  });

  if (matches.length !== 1) return null;
  return matches[0].path;
}

/**
 * Resolve a markdown href / @-chip ref to an absolute filesystem path.
 *
 * Behavior:
 *   - `file://...`                    → decode + strip leading slash on Win
 *   - POSIX absolute (`^/`)           → return as-is
 *   - Windows absolute (`^[A-Za-z]:`) → return as-is
 *   - relative                        → joinPath(cwd, href)
 *       * cwd contains `\` → Windows-style join (use `\`, normalize href `/` → `\`)
 *       * else             → POSIX-style join (use `/`)
 *       * trailing separator on cwd is trimmed before joining
 *
 * No fs.exists check; downstream lightbox handles missing files.
 *
 * @param href markdown href (may have `file://` prefix) OR @-chip ref (no `@`)
 * @param cwd  session.workingDir, expected to be an absolute path
 */
export function resolveLocalPath(href: string, cwd: string): string {
  if (href.startsWith('file://')) {
    // safeDecode: filenames with a literal `%` ("100%完成.md") form invalid
    // escape sequences — a bare decodeURIComponent throws URIError and, since
    // remote sessions call this synchronously during render, took down the
    // whole route with react-router's error page. Fall back to the raw path.
    let p = safeDecodeURIComponent(href.slice(7));
    // Windows file URL: 'file:///C:/x.png' → after slice(7): '/C:/x.png'
    // → strip leading '/' → 'C:/x.png'
    if (/^\/[A-Za-z]:[\\/]/.test(p)) p = p.slice(1);
    return p;
  }
  if (href.startsWith('/')) return href;
  if (WIN_ABS_RE.test(href)) return href;
  // UNC share (`\\server\share\...`) is already absolute on Windows — joining
  // it onto the cwd would produce garbage.
  if (href.startsWith('\\\\')) return href;

  const isWin = cwd.includes('\\');
  const sep = isWin ? '\\' : '/';
  const trimmedCwd = stripTrailingPathSeparators(cwd);
  const normalizedHref = isWin ? href.replace(/\//g, '\\') : href;
  return `${trimmedCwd}${sep}${normalizedHref}`;
}

/**
 * Absolutize an agent tool-input file path (Read/Edit/Write `file_path`,
 * Codex file_change target) against the session working directory.
 *
 * Why: the agent runtime resolves a relative path against the session cwd —
 * and some models DO emit relative paths even though the tool spec asks for
 * absolute ones — while every renderer file IPC (`text-file:read-preview`,
 * `shell:show-item-in-folder`, `xdt-file://`) requires an absolute path.
 * Mirroring the runtime semantics here makes the tool's file chip open the
 * same file the agent actually read/wrote.
 *
 *   - absolute (POSIX `/`, drive letter, UNC `\\`) → returned untouched
 *   - relative + known workingDir → resolveLocalPath join (cwd's separator
 *     style; for remote sessions the caller passes the REMOTE workingDir,
 *     which is the cwd the remote agent resolved against)
 *   - relative + empty workingDir (component used outside the chat stream)
 *     → returned untouched, preserving the downstream
 *     "Path must be absolute" error UX instead of producing a garbage path
 */
export function resolveToolFilePath(rawPath: string, workingDir: string): string {
  if (!rawPath || !workingDir) return rawPath;
  return resolveLocalPath(rawPath, workingDir);
}

