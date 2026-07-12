package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
	"github.com/golang-jwt/jwt/v5"
)

const testRestoreJWTSecret = "test-jwt-secret-at-least-32-characters-long"

func strPtr(s string) *string { return &s }

// newTestRouter builds a full router (real middleware chain — withDB, auth,
// adminOnly) plus the underlying *db.Database, so restore behavior can be
// exercised end-to-end instead of by calling handler methods directly.
func newTestRouter(t *testing.T) (mux http.Handler, database *db.Database, dbPath, backupDir string) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "test.db")
	backupDir = filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}
	database, err := db.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("new database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	mux = NewRouter(database, dbPath, backupDir, testRestoreJWTSecret, "test", OIDCConfig{}, nil)
	return mux, database, dbPath, backupDir
}

// seedUser inserts a user row directly (bypassing the API) so tests can mint
// a JWT for a real, known user ID — required now that authMiddleware
// re-checks isActive against the database on every request (F5).
func seedUser(t *testing.T, database *db.Database, id, role string, isActive int) {
	t.Helper()
	_, err := database.DB.Exec(
		`INSERT INTO users (id, email, passwordHash, displayName, role, isActive) VALUES (?, ?, ?, ?, ?, ?)`,
		id, id+"@test.local", "unused-hash", id, role, isActive,
	)
	if err != nil {
		t.Fatalf("seed user %q: %v", id, err)
	}
}

func mintTestJWT(t *testing.T, userID, role string) string {
	t.Helper()
	claims := Claims{
		UserID:   userID,
		Role:     role,
		Provider: "local",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testRestoreJWTSecret))
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	return tok
}

func multipartDatabaseUpload(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, err := w.CreateFormFile("database", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, w.FormDataContentType()
}

// TestRestoreDatabase_RejectsInvalidFile confirms a garbage upload is
// rejected before the live database is ever touched (F2) — the app must
// keep serving existing data afterward instead of being left with a closed
// or nil h.db.
func TestRestoreDatabase_RejectsInvalidFile(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-admin", "admin", 1)
	token := mintTestJWT(t, "test-admin", "admin")

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}

	body, contentType := multipartDatabaseUpload(t, "bad.db", []byte("definitely not a sqlite database"))
	req := httptest.NewRequest(http.MethodPost, "/api/restore", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid restore upload, got %d: %s", rec.Code, rec.Body.String())
	}

	orgs, err := database.GetOrganizations()
	if err != nil {
		t.Fatalf("GetOrganizations after rejected restore: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("expected existing data to survive a rejected restore, got %d orgs", len(orgs))
	}
}

// TestRestoreDatabase_ConcurrentWithReads exercises a real restore racing a
// burst of ordinary reads through the actual router middleware chain — it
// must pass under `go test -race` to prove withDB (api/router.go) actually
// serializes handler access to h.db against swapDatabase's write lock (F1).
func TestRestoreDatabase_ConcurrentWithReads(t *testing.T) {
	mux, database, dbPath, _ := newTestRouter(t)
	seedUser(t, database, "test-admin", "admin", 1)
	token := mintTestJWT(t, "test-admin", "admin")

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}

	validBackupPath := filepath.Join(filepath.Dir(dbPath), "good-backup.db")
	if err := database.Backup(validBackupPath); err != nil {
		t.Fatalf("create valid backup: %v", err)
	}
	backupBytes, err := os.ReadFile(validBackupPath)
	if err != nil {
		t.Fatalf("read valid backup: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				req := httptest.NewRequest(http.MethodGet, "/api/organizations", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Errorf("GET /api/organizations during restore: got %d: %s", rec.Code, rec.Body.String())
					return
				}
			}
		}()
	}

	body, contentType := multipartDatabaseUpload(t, "good-backup.db", backupBytes)
	req := httptest.NewRequest(http.MethodPost, "/api/restore", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected restore to succeed, got %d: %s", rec.Code, rec.Body.String())
	}

	close(stop)
	wg.Wait()
}
