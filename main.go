package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MaMissaoui/fatura-cloud/api"
	"github.com/MaMissaoui/fatura-cloud/db"
)

const defaultJWTSecret = "dev-secret-change-me-in-production"
const defaultAdminEmail = "admin@fatura.cloud"
const defaultAdminPassword = "admin"

// version is injected at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

//go:embed all:dist
var assets embed.FS

func main() {
	dbPath := dbFilePath()
	log.Printf("Initializing database at: %s", dbPath)

	database, err := db.NewDatabase(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	log.Println("Database initialized successfully")

	backupDir := backupDirPath(dbPath)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Fatalf("Failed to create backup directory: %v", err)
	}

	// Treat the presence of the /data volume as "this is a real deployment" (Docker).
	// In that case, refuse to start with insecure defaults so a production instance
	// can never silently run with a forgeable JWT secret or the well-known admin password.
	isProduction := false
	if _, err := os.Stat("/data"); err == nil {
		isProduction = true
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" || jwtSecret == defaultJWTSecret {
		if isProduction {
			log.Fatal("JWT_SECRET must be set to a strong secret in production — refusing to start with the default")
		}
		log.Println("WARNING: JWT_SECRET is unset — using the insecure default. Set JWT_SECRET before deploying.")
		jwtSecret = defaultJWTSecret
	} else if isProduction && len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters in production — refusing to start with a weak secret")
	}
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		adminEmail = defaultAdminEmail
	}
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		if isProduction {
			log.Fatal("ADMIN_PASSWORD must be set in production — refusing to start with the default password")
		}
		log.Println("WARNING: ADMIN_PASSWORD is unset — using the insecure default. Set ADMIN_PASSWORD before deploying.")
		adminPassword = defaultAdminPassword
	}

	oidcCfg := api.OIDCConfig{
		IssuerURL:    os.Getenv("OIDC_ISSUER_URL"),
		ClientID:     os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("OIDC_REDIRECT_URL"),
		Scopes:       oidcScopes(os.Getenv("OIDC_SCOPES")),
		EmailClaim:   envOrDefault("OIDC_EMAIL_CLAIM", "email"),
		NameClaim:    envOrDefault("OIDC_NAME_CLAIM", "name"),
		GroupsClaim:  envOrDefault("OIDC_GROUPS_CLAIM", "groups"),
		AdminGroup:   envOrDefault("OIDC_ADMIN_GROUP", "admins"),
	}
	if oidcCfg.IssuerURL != "" {
		log.Printf("OIDC SSO enabled — issuer %s, admin group %q", oidcCfg.IssuerURL, oidcCfg.AdminGroup)
	}

	mux := api.NewRouter(database, dbPath, backupDir, jwtSecret, version, oidcCfg)
	api.EnsureFirstAdmin(database, adminEmail, adminPassword)

	// Serve embedded frontend from dist/ with SPA fallback to index.html.
	distFS, err := fs.Sub(assets, "dist")
	if err != nil {
		log.Fatalf("Failed to sub dist: %v", err)
	}
	mux.Handle("/", spaHandler(distFS))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf(":%s", port)
	log.Printf("FaturaCloud listening on %s", addr)
	// Explicit timeouts guard against slow-client (slowloris) resource exhaustion.
	// WriteTimeout is generous because backup/restore stream large database files.
	srv := &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// securityHeaders sets baseline defensive headers on every response, API and
// embedded frontend alike.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// spaHandler serves static files from the embedded FS and falls back to
// index.html for any path that doesn't resolve to a real file — letting
// React Router handle client-side navigation.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the requested path in the embedded FS.
		f, err := fsys.Open(r.URL.Path[1:]) // strip leading "/"
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// Not found — serve index.html so React Router takes over.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// oidcScopes parses a space-separated OIDC_SCOPES override, falling back to
// the standard set needed for email + group-based role mapping.
func oidcScopes(raw string) []string {
	if raw == "" {
		return []string{"openid", "profile", "email", "groups"}
	}
	return strings.Fields(raw)
}

func dbFilePath() string {
	// In Docker the data volume is mounted at /data.
	if _, err := os.Stat("/data"); err == nil {
		return "/data/sqlite.db"
	}

	// Local development: use the platform config dir.
	dataDir, err := os.UserConfigDir()
	if err != nil {
		dataDir, _ = os.UserHomeDir()
	}
	appDir := filepath.Join(dataDir, "FaturaCloud")
	_ = os.MkdirAll(appDir, 0755)
	return filepath.Join(appDir, "sqlite.db")
}

func backupDirPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "backups")
}
