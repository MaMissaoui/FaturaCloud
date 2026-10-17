package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// mintSessionToken signs a token issued issuedAgo ago for a session that
// began sessionAgo ago.
func mintSessionToken(t *testing.T, userID string, tokenVersion int, issuedAgo, sessionAgo time.Duration) string {
	t.Helper()
	issued := time.Now().Add(-issuedAgo)
	claims := Claims{
		UserID:       userID,
		Provider:     "local",
		TokenVersion: tokenVersion,
		SessionStart: time.Now().Add(-sessionAgo).Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Audience:  jwt.ClaimStrings{jwtAudience},
			ExpiresAt: jwt.NewNumericDate(issued.Add(tokenLifetime)),
			IssuedAt:  jwt.NewNumericDate(issued),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testRestoreJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// TestSessionRenewal pins audit F175: an active session past the renewal
// age gets a fresh cookie carrying the same session start and token
// version; a young token isn't renewed, a session past maxSessionAge isn't
// extended, and an expired token still answers 401.
func TestSessionRenewal(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "clerk", "user", 1)
	database.DB.MustExec(`UPDATE users SET tokenVersion = 3 WHERE id = 'clerk'`)

	me := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	renewed := func(rec *httptest.ResponseRecorder) *Claims {
		for _, c := range rec.Result().Cookies() {
			if c.Name == authCookieName && c.Value != "" {
				claims := &Claims{}
				if _, err := jwt.ParseWithClaims(c.Value, claims, func(*jwt.Token) (any, error) {
					return []byte(testRestoreJWTSecret), nil
				}); err != nil {
					t.Fatalf("renewed token doesn't parse: %v", err)
				}
				return claims
			}
		}
		return nil
	}

	rec := me(mintSessionToken(t, "clerk", 3, 13*time.Hour, 2*24*time.Hour))
	if rec.Code != http.StatusOK {
		t.Fatalf("old-but-valid token: %d", rec.Code)
	}
	claims := renewed(rec)
	if claims == nil {
		t.Fatal("a token older than tokenRenewAfter was not renewed")
	}
	if claims.TokenVersion != 3 || claims.UserID != "clerk" {
		t.Fatalf("renewed claims = %+v, want the same user and token version", claims)
	}
	if want := time.Now().Add(-2 * 24 * time.Hour).Unix(); claims.SessionStart < want-5 || claims.SessionStart > want+5 {
		t.Fatalf("renewed session start %d, want the original %d", claims.SessionStart, want)
	}
	if time.Until(claims.ExpiresAt.Time) < tokenLifetime-time.Minute {
		t.Fatalf("renewed token expires in %v, want a full lifetime", time.Until(claims.ExpiresAt.Time))
	}

	if c := renewed(me(mintSessionToken(t, "clerk", 3, time.Hour, time.Hour))); c != nil {
		t.Fatal("a young token was renewed")
	}
	if c := renewed(me(mintSessionToken(t, "clerk", 3, 13*time.Hour, maxSessionAge+time.Hour))); c != nil {
		t.Fatal("a session past maxSessionAge was extended")
	}
	if rec := me(mintSessionToken(t, "clerk", 3, tokenLifetime+time.Minute, tokenLifetime+time.Minute)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired token: %d, want 401", rec.Code)
	}
	// A revoked session (logout bumped the version) is refused, not renewed.
	if rec := me(mintSessionToken(t, "clerk", 2, 13*time.Hour, time.Hour*13)); rec.Code != http.StatusUnauthorized || renewed(rec) != nil {
		t.Fatalf("revoked token: %d, want 401 and no renewal", rec.Code)
	}
}
