package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestAuditSnapshotNeedsASignedSession pins audit F182: the activity
// history reads a document's "before" only for a request carrying a session
// cookie this server signed; an anonymous change request goes straight to
// its 401.
func TestAuditSnapshotNeedsASignedSession(t *testing.T) {
	t.Parallel()
	_, database, _, _ := newTestRouter(t)
	h := &handler{db: database, jwtSecret: testRestoreJWTSecret}

	req := func(token string) *http.Request {
		r := httptest.NewRequest(http.MethodPut, "/api/clients/x", nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
		}
		return r
	}
	if h.hasSignedSessionToken(req("")) {
		t.Error("no cookie counted as a session")
	}
	if h.hasSignedSessionToken(req("not-a-jwt")) {
		t.Error("garbage counted as a session")
	}
	if !h.hasSignedSessionToken(req(mintSessionToken(t, "u1", 0, time.Minute, time.Minute))) {
		t.Error("a valid token wasn't recognized")
	}
	if h.hasSignedSessionToken(req(mintSessionToken(t, "u1", 0, tokenLifetime+time.Minute, tokenLifetime+time.Minute))) {
		t.Error("an expired token counted as a session")
	}
	forged := &handler{db: database, jwtSecret: "some-other-secret-some-other-secret"}
	if forged.hasSignedSessionToken(req(mintSessionToken(t, "u1", 0, time.Minute, time.Minute))) {
		t.Error("a token signed with another secret counted as a session")
	}
}
