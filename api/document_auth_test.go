package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// documentCreateRoutes is every core document domain named in issue #152
// (invoices, orders, deliveries, purchase orders, inbound deliveries,
// incoming invoices, payments) — each registered in router.go through the
// plain protected() wrapper (auth(csrf(...))). A route accidentally wired
// without that wrapper wouldn't surface any other way in this suite, since a
// missing auth check only differs from a working one once a request reaches
// the handler underneath — an empty/garbage body with no valid session must
// still 401 here, not 400/409/201 from the handler.
var documentCreateRoutes = []struct {
	name   string
	method string
	path   string
}{
	{"invoices", http.MethodPost, "/api/invoices"},
	{"orders", http.MethodPost, "/api/orders"},
	{"deliveries", http.MethodPost, "/api/deliveries"},
	{"purchase orders", http.MethodPost, "/api/purchase-orders"},
	{"inbound deliveries", http.MethodPost, "/api/inbound-deliveries"},
	{"incoming invoices", http.MethodPost, "/api/incoming-invoices"},
	{"payments", http.MethodPost, "/api/payments"},
}

// TestDocumentRoutes_RequireAuth is the no-cookie-at-all case.
func TestDocumentRoutes_RequireAuth(t *testing.T) {
	t.Parallel()
	mux, _, _, _ := newTestRouter(t)

	for _, rt := range documentCreateRoutes {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with no auth cookie: expected 401, got %d: %s", rt.method, rt.path, rec.Code, rec.Body.String())
		}
	}
}

// TestDocumentRoutes_RejectDeactivatedUser confirms authMiddleware's
// re-derived isActive check (never trusted from the JWT — see
// api/middleware.go) applies uniformly across these routes too: a token
// minted for a user who's since been deactivated must still 401, not just
// on the generic route auth_middleware_test.go already covers.
func TestDocumentRoutes_RejectDeactivatedUser(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "inactive-user", "user", 0)
	token := mintTestJWT(t, "inactive-user", "user")

	for _, rt := range documentCreateRoutes {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		authRequest(req, token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a deactivated user's token: expected 401, got %d: %s", rt.method, rt.path, rec.Code, rec.Body.String())
		}
	}
}
