package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/api"
	"github.com/MaMissaoui/fatura-cloud/db/dbtest"
)

// TestServerHandlerKeepsActivityHistory guards where compression sits (audit
// F171): the activity history learns a created document's id from the
// first bytes of the response, captured inside api.RequestLogger. A create
// sent through the whole chain with Accept-Encoding: gzip must come back
// compressed and still be recorded against its document.
func TestServerHandlerKeepsActivityHistory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sqlite.db")
	database := dbtest.NewDatabase(t, dbPath)
	t.Cleanup(func() { database.Close() })

	mux := api.NewRouter(database, dbPath, filepath.Join(dir, "backups"), "test-secret-test-secret-test-secret", "test", api.OIDCConfig{}, nil)
	api.EnsureFirstAdmin(database, "admin@example.test", "correct-horse-battery")
	h := serverHandler(mux, nil)

	send := func(method, path, cookie string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Protection", "1")
		req.Header.Set("Accept-Encoding", "gzip")
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder, v any) {
		t.Helper()
		var r io.Reader = rec.Body
		if rec.Header().Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(rec.Body)
			if err != nil {
				t.Fatalf("gzip reader: %v", err)
			}
			r = zr
		}
		if err := json.NewDecoder(r).Decode(v); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}

	login := send(http.MethodPost, "/api/auth/login", "", map[string]string{
		"email": "admin@example.test", "password": "correct-horse-battery",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	var cookie string
	for _, c := range login.Result().Cookies() {
		if c.Name == "fc_token" {
			cookie = c.Name + "=" + c.Value
		}
	}
	if cookie == "" {
		t.Fatal("login set no fc_token cookie")
	}

	orgRec := send(http.MethodPost, "/api/organizations", cookie, map[string]any{"name": "Gzip Org"})
	if orgRec.Code != http.StatusCreated {
		t.Fatalf("create organization: %d", orgRec.Code)
	}
	var org struct {
		ID string `json:"id"`
	}
	decode(orgRec, &org)

	// A long street pushes the client's JSON well past the compression
	// threshold, so this create really is answered gzipped.
	rec := send(http.MethodPost, "/api/clients", cookie, map[string]any{
		"organizationId": org.ID, "name": "Gzip Client", "street": strings.Repeat("Avenue Habib Bourguiba ", 80),
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create client: %d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q; the test needs a compressed create", rec.Header().Get("Content-Encoding"))
	}
	var client struct {
		ID string `json:"id"`
	}
	decode(rec, &client)
	if client.ID == "" {
		t.Fatal("client response carries no id")
	}

	var entityID string
	if err := database.DB.Get(&entityID, `SELECT entityId FROM audit_events WHERE method = 'POST' AND resource = 'clients' ORDER BY createdAt DESC LIMIT 1`); err != nil {
		t.Fatalf("read activity history: %v", err)
	}
	if entityID != client.ID {
		t.Fatalf("activity history entityId = %q, want the created client %q", entityID, client.ID)
	}
}
