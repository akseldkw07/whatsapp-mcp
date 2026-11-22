package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// helper: create a temporary file inside dir and return its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// helper: allow-list entries for dirs, as resolveMediaRoots would build them.
func rootsOf(dirs ...string) []mediaRoot {
	roots := make([]mediaRoot, 0, len(dirs))
	for _, dir := range dirs {
		roots = append(roots, newMediaRoot(dir))
	}
	return roots
}

func TestPathHasPrefix(t *testing.T) {
	sep := string(os.PathSeparator)
	cases := []struct {
		child, parent string
		want          bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b" + sep + "c", "/a/b", true},
		{"/a/bbb", "/a/b", false}, // sibling-prefix attack
		{"/a", "/a/b", false},     // shorter
		{"", "/a", false},
	}
	for _, tc := range cases {
		t.Run(tc.child+"|"+tc.parent, func(t *testing.T) {
			if got := pathHasPrefix(tc.child, tc.parent); got != tc.want {
				t.Fatalf("pathHasPrefix(%q,%q) = %v, want %v", tc.child, tc.parent, got, tc.want)
			}
		})
	}
}

func TestValidateMediaPathAcceptsFileInsideRoot(t *testing.T) {
	root := t.TempDir()
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval root: %v", err)
	}
	f := writeFile(t, root, "ok.txt", "hello")

	got, err := validateMediaPath(f, rootsOf(root))
	if err != nil {
		t.Fatalf("expected accept, got error: %v", err)
	}
	resolvedFile, _ := filepath.EvalSymlinks(f)
	if got.path != resolvedFile {
		t.Fatalf("expected resolved path %q, got %q", resolvedFile, got.path)
	}

	// The canonical spelling of the same file is accepted too (macOS: /var
	// is a symlink to /private/var, and t.TempDir() hands out the former).
	if _, err := validateMediaPath(filepath.Join(resolvedRoot, "ok.txt"), rootsOf(root)); err != nil {
		t.Fatalf("expected canonical spelling to be accepted, got error: %v", err)
	}
}

func TestValidateMediaPathRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	f := writeFile(t, other, "elsewhere.txt", "nope")

	if _, err := validateMediaPath(f, rootsOf(root)); err == nil {
		t.Fatal("expected error for path outside root, got nil")
	}
}

// A caller must not be able to learn whether a file outside the roots
// exists: a missing path and an existing file get the identical error, and
// the error names neither.
func TestValidateMediaPathOutsideRootIsNotAnExistenceOracle(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	existing := writeFile(t, other, "elsewhere.txt", "nope")
	missing := filepath.Join(other, "no-such-file.txt")

	_, existingErr := validateMediaPath(existing, rootsOf(root))
	_, missingErr := validateMediaPath(missing, rootsOf(root))
	if existingErr == nil || missingErr == nil {
		t.Fatalf("expected both rejected, got existing=%v missing=%v", existingErr, missingErr)
	}
	if existingErr.Error() != missingErr.Error() {
		t.Fatalf("errors differ, which reveals existence:\n existing: %v\n missing:  %v", existingErr, missingErr)
	}
	if !errors.Is(existingErr, errMediaPathOutsideRoots) || !errors.Is(missingErr, errMediaPathOutsideRoots) {
		t.Fatalf("expected errMediaPathOutsideRoots, got existing=%v missing=%v", existingErr, missingErr)
	}
	if strings.Contains(existingErr.Error(), other) {
		t.Fatalf("error echoes the rejected path: %v", existingErr)
	}
}

func TestValidateMediaPathRejectsDotDotEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "outbox")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, parent, "secret", "PRETEND-SECRET")

	escape := root + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "secret"
	if _, err := validateMediaPath(escape, rootsOf(root)); !errors.Is(err, errMediaPathOutsideRoots) {
		t.Fatalf("expected errMediaPathOutsideRoots for %q, got: %v", escape, err)
	}
}

func TestValidateMediaPathRejectsSiblingWithSharedPrefix(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "x")
	sibling := filepath.Join(parent, "xy")
	for _, dir := range []string{root, sibling} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	f := writeFile(t, sibling, "file", "nope")

	if _, err := validateMediaPath(f, rootsOf(root)); !errors.Is(err, errMediaPathOutsideRoots) {
		t.Fatalf("root %q must not accept %q, got: %v", root, f, err)
	}
}

func TestValidateMediaPathAcceptsFileUnderSymlinkedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; covered by Unix CI")
	}
	parent := t.TempDir()
	realDir := filepath.Join(parent, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	linkRoot := filepath.Join(parent, "outbox")
	if err := os.Symlink(realDir, linkRoot); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	writeFile(t, realDir, "ok.txt", "hello")
	want, _ := filepath.EvalSymlinks(filepath.Join(realDir, "ok.txt"))

	// Addressed through the configured (symlinked) root and by its canonical
	// path: both are the same allowed file.
	for _, in := range []string{filepath.Join(linkRoot, "ok.txt"), want} {
		got, err := validateMediaPath(in, rootsOf(linkRoot))
		if err != nil {
			t.Fatalf("expected %q to be accepted, got error: %v", in, err)
		}
		if got.path != want {
			t.Fatalf("validateMediaPath(%q) = %q, want %q", in, got.path, want)
		}
	}
}

func TestValidateMediaPathKeepsErrorsForPathsInsideRoot(t *testing.T) {
	root := t.TempDir()

	_, err := validateMediaPath(filepath.Join(root, "missing.txt"), rootsOf(root))
	if err == nil || errors.Is(err, errMediaPathOutsideRoots) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the not-exist error for a missing file inside the root, got: %v", err)
	}

	_, err = validateMediaPath(root, rootsOf(root))
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected a not-a-regular-file error for the root itself, got: %v", err)
	}
}

func TestValidateMediaPathRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; covered by Unix CI")
	}
	root := t.TempDir()
	secrets := t.TempDir()
	target := writeFile(t, secrets, "id_rsa", "PRETEND-PRIVATE-KEY")

	// Drop a symlink inside the allowed root that points at a file outside.
	link := filepath.Join(root, "stolen")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := validateMediaPath(link, rootsOf(root))
	if err == nil {
		t.Fatal("expected symlink escape to be rejected, got nil error")
	} else if !errors.Is(err, errMediaPathOutsideRoots) {
		t.Fatalf("expected confinement error, got: %v", err)
	}
	if strings.Contains(err.Error(), secrets) {
		t.Fatalf("error echoes where the symlink points: %v", err)
	}
}

func TestValidateMediaPathRejectsRelative(t *testing.T) {
	if _, err := validateMediaPath("not/absolute.txt", rootsOf("/tmp")); err == nil {
		t.Fatal("expected error for relative path")
	}
}

func TestValidateMediaPathRejectsDirectory(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "sub")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := validateMediaPath(subdir, rootsOf(root)); err == nil {
		t.Fatal("expected error for directory, got nil")
	} else if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected a not-a-regular-file error, got: %v", err)
	}
}

func TestResolveMediaRootsRejectsRelativeEnv(t *testing.T) {
	t.Setenv("WHATSAPP_MEDIA_ROOTS", "relative/path")
	if _, err := resolveMediaRoots(); err == nil {
		t.Fatal("expected error for relative path in env, got nil")
	}
}

func TestResolveMediaRootsAcceptsEnvList(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	t.Setenv("WHATSAPP_MEDIA_ROOTS", a+string(os.PathListSeparator)+b)

	roots, err := resolveMediaRoots()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}
}

// Each root keeps the spelling it was configured with next to its
// symlink-resolved form, so a request may use either.
func TestResolveMediaRootsKeepsConfiguredAndCanonicalForms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; covered by Unix CI")
	}
	parent := t.TempDir()
	realDir := filepath.Join(parent, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	linkRoot := filepath.Join(parent, "outbox")
	if err := os.Symlink(realDir, linkRoot); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	t.Setenv("WHATSAPP_MEDIA_ROOTS", linkRoot+string(os.PathSeparator))

	roots, err := resolveMediaRoots()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantCanonical, _ := filepath.EvalSymlinks(realDir)
	if len(roots) != 1 || roots[0].configured != linkRoot || roots[0].canonical != wantCanonical {
		t.Fatalf("roots = %+v, want configured %q and canonical %q", roots, linkRoot, wantCanonical)
	}
}

// /api/send answers a media_path outside the roots the same way whether or
// not the file exists, and never reaches the send path.
func TestSendHandler_MediaPathOutsideRoots_UniformRejection(t *testing.T) {
	const token = "supersecrettoken1234567890abcdef"
	root := t.TempDir()
	other := t.TempDir()
	existing := writeFile(t, other, "elsewhere.txt", "nope")
	missing := filepath.Join(other, "no-such-file.txt")
	handler := newRESTMux(newTestClient(&mockLIDStore{}), newTestMessageStore(t), 8080, token, rootsOf(root))

	bodies := map[string]string{}
	for name, mediaPath := range map[string]string{"existing": existing, "missing": missing} {
		payload, err := json.Marshal(map[string]string{"recipient": "15551234567", "media_path": mediaPath})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/send", strings.NewReader(string(payload)))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusForbidden {
			t.Fatalf("%s: expected 403, got %d (%s)", name, resp.Code, resp.Body.String())
		}
		bodies[name] = resp.Body.String()
	}
	if bodies["existing"] != bodies["missing"] {
		t.Fatalf("responses differ, which reveals existence:\n existing: %s\n missing:  %s", bodies["existing"], bodies["missing"])
	}
	if !strings.Contains(bodies["existing"], errMediaPathOutsideRoots.Error()) {
		t.Fatalf("expected the confinement error in the response, got: %s", bodies["existing"])
	}
}

func TestOutboundFileName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "windows absolute path, the reported leak",
			in:   `C:\Users\someone\.local\share\whatsapp-mcp\outbox\brief.zip`,
			want: "brief.zip",
		},
		{
			name: "windows path with spaces and parentheses",
			in:   `C:\Users\someone\outbox\2026-08-24 pitch (v19).mp4`,
			want: "2026-08-24 pitch (v19).mp4",
		},
		{
			name: "posix absolute path",
			in:   "/home/someone/.local/share/whatsapp-mcp/outbox/brief.zip",
			want: "brief.zip",
		},
		{
			name: "posix filename preserves colon",
			in:   "/home/someone/outbox/draft:v2.pdf",
			want: "draft:v2.pdf",
		},
		{
			name: "posix filename resembling a drive prefix is preserved",
			in:   "/home/someone/outbox/C:notes.pdf",
			want: "C:notes.pdf",
		},
		{
			name: "posix filename preserves surrounding spaces",
			in:   "/home/someone/outbox/ draft v2.pdf ",
			want: " draft v2.pdf ",
		},
		{
			name: "mixed separators, as a hand-written path often is",
			in:   `C:/Users/someone\outbox/brief.zip`,
			want: "brief.zip",
		},
		{
			name: "bare name, already safe",
			in:   "brief.zip",
			want: "brief.zip",
		},
		{
			name: "bare posix filename preserves colon",
			in:   "draft:v2.pdf",
			want: "draft:v2.pdf",
		},
		{
			name: "drive-relative path with no separator at all",
			in:   `C:brief.zip`,
			want: "brief.zip",
		},
		{
			name: "trailing separator yields a neutral fallback, never empty",
			in:   `C:\Users\someone\outbox\`,
			want: "file",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := outboundFileName(tc.in)
			if got != tc.want {
				t.Fatalf("outboundFileName(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// The real invariant: nothing that identifies the machine or the
			// user survives into the name, while valid filename characters stay intact.
			for _, leak := range []string{"Users", "home", "someone", `\`, "/"} {
				if strings.Contains(got, leak) {
					t.Fatalf("outboundFileName(%q) = %q, leaks %q", tc.in, got, leak)
				}
			}
		})
	}
}
