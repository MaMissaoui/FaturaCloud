package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// newTestDistFS builds an in-memory embed.FS-like filesystem mirroring a
// minimal dist/ output: an index.html and a nested asset with no index of
// its own, so directory-listing behavior can be exercised.
func newTestDistFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       {Data: []byte("<html>spa shell</html>")},
		"assets/app.js":    {Data: []byte("console.log('app')")},
		"assets/sub/x.txt": {Data: []byte("nested")},
	}
}

func TestSpaHandler_ServesRealFile(t *testing.T) {
	h := mustSPAHandler(t, newTestDistFS())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/assets/app.js", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("got status %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "console.log") {
		t.Fatalf("expected asset content, got %q", rr.Body.String())
	}
}

func TestSpaHandler_UnknownPathFallsBackToIndex(t *testing.T) {
	h := mustSPAHandler(t, newTestDistFS())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/clients/abc123", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("got status %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "spa shell") {
		t.Fatalf("expected index.html fallback, got %q", rr.Body.String())
	}
}

// TestSpaHandler_UnmatchedAPIPathReturnsJSON404 covers F16: unmatched
// /api/* paths must not silently 200 with the SPA shell — that breaks any
// client (including our own fetch wrapper) expecting JSON on failure.
func TestSpaHandler_UnmatchedAPIPathReturnsJSON404(t *testing.T) {
	h := mustSPAHandler(t, newTestDistFS())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/nope", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Fatalf("got status %d, want 404", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("got Content-Type %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body isn't JSON: %v (%q)", err, rr.Body.String())
	}
	if body["error"] == "" {
		t.Fatalf("expected an error message, got %v", body)
	}
}

// TestSpaHandler_DirectoryPathFallsBackToIndex covers F16: a directory with
// no index.html of its own (e.g. /assets/) must not render a listing. Under
// assets/ the answer is a 404 (audit F171: nothing there may stand in for
// an asset), elsewhere the SPA shell.
func TestSpaHandler_DirectoryPathFallsBackToIndex(t *testing.T) {
	h := mustSPAHandler(t, newTestDistFS())

	for path, wantShell := range map[string]bool{"/assets": true, "/assets/": true, "/assets/sub": false, "/assets/sub/": false} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", path, nil)
			h.ServeHTTP(rr, req)
			if strings.Contains(rr.Body.String(), "x.txt") {
				t.Fatalf("listed the directory: %q", rr.Body.String())
			}
			if wantShell && (rr.Code != 200 || !strings.Contains(rr.Body.String(), "spa shell")) {
				t.Fatalf("got %d %q, want the index.html fallback", rr.Code, rr.Body.String())
			}
			if !wantShell && rr.Code != 404 {
				t.Fatalf("got status %d, want 404", rr.Code)
			}
		})
	}
}

func mustSPAHandler(t *testing.T, fsys fstest.MapFS) http.Handler {
	t.Helper()
	h, err := spaHandler(fsys)
	if err != nil {
		t.Fatalf("spaHandler: %v", err)
	}
	return h
}

func TestSecurityHeaders_IncludesCSP(t *testing.T) {
	h := securityHeaders(mustSPAHandler(t, newTestDistFS()), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rr, req)
	csp := rr.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("expected a Content-Security-Policy header")
	}
	for _, want := range []string{"default-src 'self'", "script-src 'self';", "worker-src 'self';", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP %q missing expected directive %q", csp, want)
		}
	}
	// What the removed in-browser PDF stack needed (audit F180).
	for _, gone := range []string{"wasm-unsafe-eval", "connect-src 'self' data:", "blob: *.sentry", "worker-src 'self' blob:"} {
		if strings.Contains(csp, gone) {
			t.Fatalf("CSP %q still allows %q", csp, gone)
		}
	}
}

func TestBackupDirPermissions(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "sqlite.db")
	backupDir := backupDirPath(dbPath)
	if err := ensureBackupDir(backupDir); err != nil {
		t.Fatalf("ensureBackupDir: %v", err)
	}
	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("stat backup dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0750 {
		t.Fatalf("backup dir mode = %o, want 0750", perm)
	}
}

// TestBackupDirPermissionsFixAnExistingDirectory pins audit F179: a 0700
// directory left by an install from before #512 is opened up to the group
// too, not only a newly created one.
func TestBackupDirPermissionsFixAnExistingDirectory(t *testing.T) {
	backupDir := filepath.Join(t.TempDir(), "backups")
	if err := os.Mkdir(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := ensureBackupDir(backupDir); err != nil {
		t.Fatalf("ensureBackupDir: %v", err)
	}
	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0750 {
		t.Fatalf("existing backup dir mode = %o, want 0750", perm)
	}
}

// TestHealthcheckCommand: the Docker HEALTHCHECK command exits 0 only when
// the server answers /api/healthz with 200 (audit F181).
func TestHealthcheckCommand(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]

	if code := healthcheck(port); code != 0 {
		t.Fatalf("healthy server: exit %d, want 0", code)
	}
	status = http.StatusServiceUnavailable
	if code := healthcheck(port); code != 1 {
		t.Fatalf("unhealthy server: exit %d, want 1", code)
	}
	srv.Close()
	if code := healthcheck(port); code != 1 {
		t.Fatalf("no server: exit %d, want 1", code)
	}
}
