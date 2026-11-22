package main

// Outbound media-path confinement.
//
// Today /api/send accepts any absolute path in media_path and the bridge
// happily reads it via os.ReadFile. That makes the bridge a generic
// arbitrary-file-read primitive for any local caller (after auth) — and
// for prompt-injection attacks that reach an MCP-driven agent — because
// the file is then sent out as a WhatsApp document to an attacker-chosen
// recipient.
//
// We confine media_path to a configurable allow-list of root directories.
// Default: ~/.local/share/whatsapp-mcp/outbox (created on first use).
// Override: colon-separated absolute paths in WHATSAPP_MEDIA_ROOTS.
//
// The check runs twice. First lexically, on the cleaned request path,
// before any filesystem call: a path outside the roots is rejected without
// being looked at, with one fixed error, so the endpoint cannot be used to
// probe which files exist. Then again after symlinks are resolved, so
// dropping a symlink inside outbox/ that points at /etc/passwd does not
// bypass the guard.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultOutboxSubpath is appended to the user's home directory when no
// override is configured. ~/.local/share is the XDG default for
// user-specific data files; using a project subdirectory keeps the bridge
// out of the way of unrelated tools.
const defaultOutboxSubpath = ".local/share/whatsapp-mcp/outbox"

// errMediaPathOutsideRoots is the one answer for every media_path that is
// not inside an allowed root, whether it fails the lexical check or only
// the check after symlink resolution. It deliberately carries no path and
// no filesystem error: anything more would tell the caller whether a file
// outside the roots exists.
var errMediaPathOutsideRoots = errors.New(
	"media_path is not inside an allowed media root " +
		"(set WHATSAPP_MEDIA_ROOTS to allow more directories)",
)

// mediaRoot is one allowed directory, in the two spellings a request may
// use for it: as configured, and with symlinks resolved. They differ when
// the root is, or sits below, a symlink (macOS: /var -> /private/var).
type mediaRoot struct {
	configured string // absolute and cleaned, as configured
	canonical  string // configured with symlinks resolved
}

func (r mediaRoot) String() string {
	if r.configured == r.canonical {
		return r.configured
	}
	return r.configured + " (-> " + r.canonical + ")"
}

// newMediaRoot builds the allow-list entry for an absolute directory path.
func newMediaRoot(dir string) mediaRoot {
	configured := filepath.Clean(dir)
	canonical, err := canonicalizePath(configured)
	if err != nil {
		canonical = configured
	}
	return mediaRoot{configured: configured, canonical: canonical}
}

// validatedMediaPath is an outbound media path that validateMediaPath has
// confined to the allowed roots. sendWhatsAppMessage takes this type rather
// than a string so a request path cannot reach os.ReadFile unchecked.
// The zero value means "no media". Only validateMediaPath may fill in path.
type validatedMediaPath struct {
	path string // canonical (symlink-resolved) path of a regular file, or ""
}

// resolveMediaRoots returns the list of allowed absolute root directories
// for outbound media. Each root keeps its configured spelling and, where
// the directory exists, its filepath.EvalSymlinks form, so the prefix
// checks are meaningful even if the user pointed the env var at a
// symlinked location.
func resolveMediaRoots() ([]mediaRoot, error) {
	if env := strings.TrimSpace(os.Getenv("WHATSAPP_MEDIA_ROOTS")); env != "" {
		var roots []mediaRoot
		for _, raw := range strings.Split(env, string(os.PathListSeparator)) {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			if !filepath.IsAbs(raw) {
				return nil, fmt.Errorf("WHATSAPP_MEDIA_ROOTS entries must be absolute paths, got %q", raw)
			}
			roots = append(roots, newMediaRoot(raw))
		}
		if len(roots) == 0 {
			return nil, errors.New("WHATSAPP_MEDIA_ROOTS is set but contains no valid entries")
		}
		return roots, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine home directory: %w", err)
	}
	def := filepath.Join(home, defaultOutboxSubpath)
	if mkErr := os.MkdirAll(def, 0o700); mkErr != nil {
		return nil, fmt.Errorf("create default outbox %q: %w", def, mkErr)
	}
	return []mediaRoot{newMediaRoot(def)}, nil
}

// canonicalizePath returns an absolute, symlink-resolved path. EvalSymlinks
// requires the path to exist; for newly-created paths the caller must
// ensure existence first (see resolveMediaRoots).
func canonicalizePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// Path doesn't exist yet (or is unreadable). Fall back to Abs;
		// validateMediaPath will run EvalSymlinks again at request time
		// when the file is expected to exist.
		return abs, nil
	}
	return resolved, nil
}

// validateMediaPath checks whether mediaPath points at a regular file
// inside one of the allowed roots. On success the result holds the
// canonical path, so callers read from the resolved file (defends against
// TOCTOU between validation and ReadFile in degenerate cases).
//
// Nothing here touches the filesystem until the cleaned path has passed
// the lexical prefix check below. Keep that check inline: it is what makes
// "outside the roots" indistinguishable for existing and missing files,
// and what static analysis recognizes as the sanitizer for the path.
func validateMediaPath(mediaPath string, allowedRoots []mediaRoot) (validatedMediaPath, error) {
	if mediaPath == "" {
		return validatedMediaPath{}, errors.New("media_path is empty")
	}
	if !filepath.IsAbs(mediaPath) {
		return validatedMediaPath{}, fmt.Errorf("media_path must be absolute, got %q", mediaPath)
	}

	// Clean folds "." and ".." away, so <root>/../secret is compared as
	// what it names, not as something that merely starts with <root>.
	p := filepath.Clean(mediaPath)
	sep := string(os.PathSeparator)

	for _, root := range allowedRoots {
		for _, dir := range []string{root.configured, root.canonical} {
			if p == dir {
				// A root is a directory. Say so without a filesystem call.
				return validatedMediaPath{}, fmt.Errorf("media_path is not a regular file: %q", p)
			}
			// The trailing separator is what stops root /tmp/x from
			// accepting /tmp/xy/file.
			if strings.HasPrefix(p, strings.TrimSuffix(dir, sep)+sep) {
				return resolveMediaPathInRoots(p, allowedRoots)
			}
		}
	}
	return validatedMediaPath{}, errMediaPathOutsideRoots
}

// resolveMediaPathInRoots finishes validation for a cleaned path that is
// lexically inside an allowed root (validateMediaPath is the only caller):
// it resolves symlinks, requires the result to still be inside a root, and
// requires a regular file. Errors about a path inside a root keep their
// filesystem cause; a symlink leading out of the roots gets the same fixed
// error as any other outside path.
func resolveMediaPathInRoots(p string, allowedRoots []mediaRoot) (validatedMediaPath, error) {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return validatedMediaPath{}, fmt.Errorf("resolve media_path: %w", err)
	}

	contained := false
	for _, root := range allowedRoots {
		if pathHasPrefix(resolved, root.canonical) {
			contained = true
			break
		}
	}
	if !contained {
		return validatedMediaPath{}, errMediaPathOutsideRoots
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return validatedMediaPath{}, fmt.Errorf("stat media_path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return validatedMediaPath{}, fmt.Errorf("media_path is not a regular file: %q", resolved)
	}
	return validatedMediaPath{path: resolved}, nil
}

// pathHasPrefix returns true when child is the same path as parent, or a
// strict descendant. Plain string-prefix matching is unsafe ("/foo/bar"
// would match "/foo/barbaz"), so we require either exact match or an
// explicit separator after parent.
func pathHasPrefix(child, parent string) bool {
	if child == parent {
		return true
	}
	if !strings.HasPrefix(child, parent) {
		return false
	}
	rest := child[len(parent):]
	return strings.HasPrefix(rest, string(os.PathSeparator))
}

// outboundFileName returns the name a RECIPIENT should see for an outbound
// document, given a local media path.
//
// This is not cosmetic. The document filename travels to the recipient inside
// the message, so every character of the path we put here is published. The
// previous code took everything after the last forward slash:
//
//	mediaPath[strings.LastIndex(mediaPath, "/")+1:]
//
// On Windows that leaks the entire absolute path. The path handed to the send
// routine has already been through filepath.EvalSymlinks (see the /api/send
// handler), which normalises separators to backslash, so LastIndex for "/"
// returns -1, the slice starts at 0, and the recipient sees something like
// C:\Users\<name>\.local\share\whatsapp-mcp\outbox\file.zip -- username and
// home layout included.
//
// filepath.Base alone is not enough either: it only honours the separator of
// the platform it runs on, so a Windows-style path handled by a Linux or macOS
// build would still come through whole. We split on BOTH separators regardless
// of GOOS, which is what a filename crossing machines requires.
//
// If the path ends in a separator, or is otherwise nameless, we fall back to a
// neutral constant rather than sending an empty filename.
func outboundFileName(mediaPath string) string {
	name := mediaPath
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	// Drop a Windows drive prefix such as "C:" left behind by a bare
	// "C:file.zip" style path, which has no separator at all. Do not strip
	// arbitrary colons: names such as "draft:v2.pdf" are valid on POSIX.
	if len(mediaPath) >= 2 && mediaPath[1] == ':' &&
		((mediaPath[0] >= 'A' && mediaPath[0] <= 'Z') || (mediaPath[0] >= 'a' && mediaPath[0] <= 'z')) &&
		len(name) >= 2 && name[1] == ':' &&
		((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')) {
		name = name[2:]
	}
	if name == "" {
		return "file"
	}
	return name
}
