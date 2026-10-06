package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// These tests swap slog's default logger, so none of them is parallel: the
// package's parallel tests only start once every sequential test is done.

// captureLog points slog's default logger at a buffer (JSON, debug level)
// for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// logLine returns the "request" line logged for request id, failing if there
// isn't exactly one.
func logLine(t *testing.T, buf *bytes.Buffer, id string) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var line map[string]any
		if json.Unmarshal([]byte(raw), &line) == nil && line["msg"] == "request" && line["request_id"] == id {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one log line for request %s, got %d in:\n%s", id, len(found), buf.String())
	}
	return found[0]
}

func serveLogged(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	RequestLogger(h, nil).ServeHTTP(rec, req)
	return rec
}

func TestRequestLogLineCarriesRouteAndUser(t *testing.T) {
	buf := captureLog(t)
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "log-user", "user", 1)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	authRequest(req, mintTestJWT(t, "log-user", "user"))
	rec := serveLogged(mux, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	id := rec.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("no X-Request-ID header")
	}
	line := logLine(t, buf, id)
	for key, want := range map[string]any{
		"level":   "DEBUG", // a successful read
		"route":   "GET /api/auth/me",
		"status":  float64(200),
		"user":    "log-user@test.local",
		"user_id": "log-user",
	} {
		if line[key] != want {
			t.Errorf("%s = %v, want %v", key, line[key], want)
		}
	}
}

// An organization route's line names the organization and the caller's role
// in it; a section the role doesn't have is a 403, logged as a warning.
func TestRequestLogLineCarriesOrganizationAndRole(t *testing.T) {
	buf := captureLog(t)
	mux, database, _, _ := newTestRouter(t)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-log"}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedUser(t, database, "cashier", "user", 1)
	if _, err := database.AddOrganizationUser("org-log", "cashier", "cashbook"); err != nil {
		t.Fatalf("AddOrganizationUser: %v", err)
	}
	token := mintTestJWT(t, "cashier", "user")

	for _, c := range []struct {
		path, level string
		status      float64
	}{
		{"/api/organizations/org-log/clients", "DEBUG", 200},
		{"/api/organizations/org-log/invoices/outstanding", "WARN", 403},
	} {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		authRequest(req, token)
		rec := serveLogged(mux, req)
		line := logLine(t, buf, rec.Header().Get("X-Request-ID"))
		if line["org"] != "org-log" || line["role"] != "cashbook" || line["level"] != c.level || line["status"] != c.status {
			t.Errorf("%s: line = %v, want org-log/cashbook at %s with %v", c.path, line, c.level, c.status)
		}
	}
}

func TestInternalErrorIsLoggedWithItsReference(t *testing.T) {
	buf := captureLog(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/things/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeInternalError(w, errors.New("disk on fire"))
	})
	rec := serveLogged(mux, httptest.NewRequest(http.MethodGet, "/api/things/42", nil))

	id := rec.Header().Get("X-Request-ID")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "ref "+id) {
		t.Fatalf("got %d %s, want a 500 quoting ref %s", rec.Code, rec.Body, id)
	}
	if strings.Contains(rec.Body.String(), "disk on fire") {
		t.Errorf("the error leaked to the client: %s", rec.Body)
	}
	line := logLine(t, buf, id)
	if line["level"] != "ERROR" || line["err"] != "disk on fire" || line["route"] != "GET /api/things/{id}" {
		t.Errorf("line = %v, want ERROR with err and the route pattern", line)
	}
}

func TestPanicBecomesALoggedInternalError(t *testing.T) {
	buf := captureLog(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("kaboom")
	})
	rec := serveLogged(mux, httptest.NewRequest(http.MethodPost, "/api/boom", nil))

	id := rec.Header().Get("X-Request-ID")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "ref "+id) {
		t.Fatalf("got %d %s, want a 500 quoting ref %s", rec.Code, rec.Body, id)
	}
	line := logLine(t, buf, id)
	if line["level"] != "ERROR" || line["err"] != "panic: kaboom" {
		t.Errorf("line = %v, want ERROR panic: kaboom", line)
	}
	if stack, _ := line["stack"].(string); !strings.Contains(stack, "request_log_test.go") {
		t.Errorf("stack doesn't point at the panicking handler: %q", stack)
	}
}

// Bodies, query strings, cookies and passwords never reach the log.
func TestRequestLogLeavesOutPrivateData(t *testing.T) {
	buf := captureLog(t)
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "private-user", "user", 1)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me?q=SECRET-CIN-12345", nil)
	authRequest(req, mintTestJWT(t, "private-user", "user"))
	serveLogged(mux, req)

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email":"nobody@test.local","password":"SECRET-PASSWORD"}`))
	login.Header.Set(csrfHeaderName, "1")
	login.Header.Set("Content-Type", "application/json")
	rec := serveLogged(mux, login)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want 401", rec.Code)
	}

	out := buf.String()
	for _, secret := range []string{"SECRET-CIN-12345", "SECRET-PASSWORD", mintTestJWT(t, "private-user", "user")} {
		if strings.Contains(out, secret) {
			t.Errorf("log contains %q:\n%s", secret, out)
		}
	}
	line := logLine(t, buf, rec.Header().Get("X-Request-ID"))
	if line["level"] != "WARN" || line["event"] != "login_failed" || line["login_email"] != "nobody@test.local" {
		t.Errorf("failed login line = %v, want WARN login_failed with the email", line)
	}

	// The login route is open to anyone: a huge "email" must not fill the
	// log (and rotate the evidence out of it).
	huge := strings.Repeat("x", 100_000) + "@test.local"
	long := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email":"`+huge+`","password":"wrong"}`))
	long.Header.Set(csrfHeaderName, "1")
	long.Header.Set("Content-Type", "application/json")
	rec = serveLogged(mux, long)
	if got, _ := logLine(t, buf, rec.Header().Get("X-Request-ID"))["login_email"].(string); len(got) > 260 {
		t.Errorf("logged login_email is %d bytes, want it capped", len(got))
	}
}

func TestStaticFilesAreNotLogged(t *testing.T) {
	buf := captureLog(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	rec := serveLogged(h, httptest.NewRequest(http.MethodGet, "/assets/index.js", nil))
	if rec.Header().Get("X-Request-ID") != "" || buf.Len() != 0 {
		t.Errorf("a static file was logged: %q", buf.String())
	}
}

func TestClientErrorReportIsLoggedWithoutTheQuery(t *testing.T) {
	buf := captureLog(t)
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "browser-user", "user", 1)
	token := mintTestJWT(t, "browser-user", "user")

	report := func() *httptest.ResponseRecorder {
		body := `{"name":"TypeError","message":"x is undefined","stack":"at Invoices (index.tsx:12)",` +
			`"source":"window.error","url":"https://fatura.example/cash-book?q=SECRET-PHONE","release":"v9.9.9"}`
		req := httptest.NewRequest(http.MethodPost, "/api/client-errors", strings.NewReader(body))
		authRequest(req, token)
		req.Header.Set("Content-Type", "application/json")
		return serveLogged(mux, req)
	}
	rec := report()
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	line := logLine(t, buf, rec.Header().Get("X-Request-ID"))
	for key, want := range map[string]any{
		"level":   "ERROR",
		"event":   "client_error",
		"error":   "TypeError: x is undefined",
		"page":    "/cash-book",
		"release": "v9.9.9",
		"user":    "browser-user@test.local",
	} {
		if line[key] != want {
			t.Errorf("%s = %v, want %v", key, line[key], want)
		}
	}
	if strings.Contains(buf.String(), "SECRET-PHONE") {
		t.Error("the page's query string reached the log")
	}
}

func TestClientErrorReportsAreLimitedPerUser(t *testing.T) {
	now := time.Now()
	for i := 0; i < clientErrorLimit; i++ {
		if !allowClientError("limit-user", now) {
			t.Fatalf("report %d refused, want %d allowed", i+1, clientErrorLimit)
		}
	}
	if allowClientError("limit-user", now) {
		t.Error("report past the limit allowed")
	}
	if !allowClientError("other-user", now) {
		t.Error("another user's report refused")
	}
	if !allowClientError("limit-user", now.Add(clientErrorWindow)) {
		t.Error("report in a new window refused")
	}
}
