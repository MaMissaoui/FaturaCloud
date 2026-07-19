package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/netip"
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
	if err := os.MkdirAll(backupDir, 0700); err != nil {
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

	trustedProxies := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))

	mux := api.NewRouter(database, dbPath, backupDir, jwtSecret, version, oidcCfg, trustedProxies)
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

// contentSecurityPolicy is same-origin by design (no CDNs). Ant Design and
// @react-pdf need inline styles; @react-pdf renders PDFs via a blob: worker,
// compiles a WebAssembly font-shaping engine loaded from a data: URI (hence
// 'wasm-unsafe-eval' and connect-src data:), and its in-app PDFViewer fetches
// its own rendered output back via an XHR to a blob: URL (hence connect-src
// blob:) — all three confirmed against an actual invoice PDF preview/download,
// not just copied from a generic starting policy.
const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"font-src 'self' data:; connect-src 'self' data: blob: *.sentry.io; " +
	"worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'self'; " +
	"form-action 'self'; object-src 'none'"

// securityHeaders sets baseline defensive headers on every response, API and
// embedded frontend alike.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		// HSTS only on requests that actually arrived over HTTPS. TLS is
		// terminated upstream (reverse proxy) in every real deployment, so
		// we key off X-Forwarded-Proto; sending it unconditionally would be
		// wrong for plain-HTTP LAN deployments. 2 years + includeSubDomains,
		// no preload (that's a standing commitment the operator should opt
		// into deliberately, not a default).
		if requestIsHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// requestIsHTTPS reports whether the original client request was HTTPS,
// accounting for TLS termination upstream (mirrors api.isHTTPS, which is
// unexported).
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// spaHandler serves static files from the embedded FS and falls back to
// index.html for any path that doesn't resolve to a real file — letting
// React Router handle client-side navigation. Unmatched /api/* paths (no
// registered handler matched them) get a JSON 404 instead of index.html, and
// directory paths fall through to index.html instead of a listing.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONNotFound(w)
			return
		}
		// Try to open the requested path in the embedded FS.
		f, err := fsys.Open(r.URL.Path[1:]) // strip leading "/"
		if err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Not found, or a directory with no index.html of its own — serve the
		// SPA shell so React Router takes over instead of listing contents.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

func writeJSONNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":"not found"}`))
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

// parseTrustedProxies parses a comma/space-separated list of IPs or CIDRs
// (e.g. "172.20.0.0/16, 10.0.0.5") naming reverse proxies allowed to set
// X-Forwarded-For. Left empty (the default), the login rate limiter always
// keys on the direct TCP peer — safe with no reverse proxy in front, but
// every client behind one then shares a single bucket. Only set this to a
// proxy that is the sole path to the app (an exposed app port alongside a
// trusted-but-bypassable proxy lets a client set X-Forwarded-For directly
// and dodge rate limiting entirely).
func parseTrustedProxies(raw string) []netip.Prefix {
	if raw == "" {
		return nil
	}
	var prefixes []netip.Prefix
	for field := range strings.FieldsSeq(strings.ReplaceAll(raw, ",", " ")) {
		entry := field
		if !strings.Contains(entry, "/") {
			addr, err := netip.ParseAddr(entry)
			if err != nil {
				log.Printf("ignoring invalid TRUSTED_PROXIES entry %q: %v", field, err)
				continue
			}
			bits := 32
			if addr.Is6() {
				bits = 128
			}
			entry = fmt.Sprintf("%s/%d", addr, bits)
		}
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			log.Printf("ignoring invalid TRUSTED_PROXIES entry %q: %v", field, err)
			continue
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes
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
