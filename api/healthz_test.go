package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db/dbtest"
)

// TestHealthz: the container health check answers without a session while
// the database works, and 503 once it doesn't (audit F181).
func TestHealthz(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: %d %s", rec.Code, rec.Body.String())
	}
	_ = database.Close()
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("healthz with the database closed: %d, want 503", rec.Code)
	}
}

// TestBackgroundLoopsStopWithTheirContext: graceful shutdown cancels the
// context and waits for the loops; they must return promptly rather than
// sleep out their interval (audit F181).
func TestBackgroundLoopsStopWithTheirContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database := dbtest.NewDatabase(t, dbPath)
	t.Cleanup(func() { _ = database.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	_, background := NewRouterContext(ctx, database, dbPath, filepath.Join(dir, "backups"), testRestoreJWTSecret, "test", OIDCConfig{}, nil)
	cancel()
	done := make(chan struct{})
	go func() {
		background.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("background loops still running 5s after their context ended")
	}
}
