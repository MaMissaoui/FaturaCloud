package main

import (
	"encoding/json"
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
	h := spaHandler(newTestDistFS())
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
	h := spaHandler(newTestDistFS())
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
	h := spaHandler(newTestDistFS())
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
// no index.html of its own (e.g. /assets/) must not render a listing.
func TestSpaHandler_DirectoryPathFallsBackToIndex(t *testing.T) {
	h := spaHandler(newTestDistFS())

	for _, path := range []string{"/assets", "/assets/", "/assets/sub", "/assets/sub/"} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", path, nil)
			h.ServeHTTP(rr, req)
			if rr.Code != 200 {
				t.Fatalf("got status %d, want 200", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "spa shell") {
				t.Fatalf("expected index.html fallback (no directory listing), got %q", rr.Body.String())
			}
		})
	}
}

func TestSecurityHeaders_IncludesCSP(t *testing.T) {
	h := securityHeaders(spaHandler(newTestDistFS()))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rr, req)
	csp := rr.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("expected a Content-Security-Policy header")
	}
	for _, want := range []string{"default-src 'self'", "worker-src 'self' blob:", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP %q missing expected directive %q", csp, want)
		}
	}
}

func TestBackupDirPermissions(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "sqlite.db")
	backupDir := backupDirPath(dbPath)
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("stat backup dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Fatalf("backup dir mode = %o, want 0700", perm)
	}
}
