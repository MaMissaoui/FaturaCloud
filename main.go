package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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
	// "fatura-cloud healthcheck" is the Docker HEALTHCHECK: the runtime image
	// has no curl or wget, so the binary asks itself (audit F181).
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.Getenv("PORT")))
	}

	restrictFileModes()
	setupLogging(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT"))
	slog.Info("FaturaCloud starting", "version", version)

	dbPath := dbFilePath()
	slog.Info("Initializing database", "path", dbPath)

	database, err := db.NewDatabase(dbPath)
	if err != nil {
		fatal("Failed to initialize database", "err", err)
	}
	defer database.Close()
	slog.Info("Database initialized")

	backupDir := backupDirPath(dbPath)
	if err := ensureBackupDir(backupDir); err != nil {
		fatal("Failed to create backup directory", "err", err)
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
			fatal("JWT_SECRET must be set to a strong secret in production — refusing to start with the default")
		}
		slog.Warn("JWT_SECRET is unset — using the insecure default. Set JWT_SECRET before deploying.")
		jwtSecret = defaultJWTSecret
	} else if isProduction && len(jwtSecret) < 32 {
		fatal("JWT_SECRET must be at least 32 characters in production — refusing to start with a weak secret")
	}
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		adminEmail = defaultAdminEmail
	}
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		if isProduction {
			fatal("ADMIN_PASSWORD must be set in production — refusing to start with the default password")
		}
		slog.Warn("ADMIN_PASSWORD is unset — using the insecure default. Set ADMIN_PASSWORD before deploying.")
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
		slog.Info("OIDC SSO enabled", "issuer", oidcCfg.IssuerURL, "admin_group", oidcCfg.AdminGroup)
	}

	trustedProxies := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))

	// SIGTERM (docker stop) or Ctrl-C starts a graceful shutdown: the
	// server stops accepting, in-flight requests finish, the background
	// loops stop and the database closes (audit F181).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux, background := api.NewRouterContext(ctx, database, dbPath, backupDir, jwtSecret, version, oidcCfg, trustedProxies)
	api.EnsureFirstAdmin(database, adminEmail, adminPassword)
	if err := database.SeedAccountingDefaultsForAllOrganizations(); err != nil {
		slog.Error("SeedAccountingDefaultsForAllOrganizations", "err", err)
	}
	if err := database.SeedInventoryAccountingDefaultsForAllOrganizations(); err != nil {
		slog.Error("SeedInventoryAccountingDefaultsForAllOrganizations", "err", err)
	}
	if err := database.SeedImportCostAccountForAllOrganizations(); err != nil {
		slog.Error("SeedImportCostAccountForAllOrganizations", "err", err)
	}
	if err := database.SeedDefaultPaymentTermsForAllOrganizations(); err != nil {
		slog.Error("SeedDefaultPaymentTermsForAllOrganizations", "err", err)
	}
	if err := database.SeedDefaultUnitsOfMeasureForAllOrganizations(); err != nil {
		slog.Error("SeedDefaultUnitsOfMeasureForAllOrganizations", "err", err)
	}

	// Serve embedded frontend from dist/ with SPA fallback to index.html.
	distFS, err := fs.Sub(assets, "dist")
	if err != nil {
		fatal("Failed to sub dist", "err", err)
	}
	spa, err := spaHandler(distFS)
	if err != nil {
		fatal("Failed to load the frontend", "err", err)
	}
	mux.Handle("/", spa)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf(":%s", port)
	slog.Info("FaturaCloud listening", "addr", addr)
	// Explicit timeouts guard against slow-client (slowloris) resource exhaustion.
	// WriteTimeout is generous because backup/restore stream large database files.
	srv := &http.Server{
		Addr:              addr,
		Handler:           serverHandler(mux, trustedProxies),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		slog.Info("Shutting down")
		// Within docker stop's grace period (stop_grace_period in compose);
		// an export or backup still running after it is cut off.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("Shutdown", "err", err)
		}
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fatal("Server error", "err", err)
	}
	<-shutdownDone
	background.Wait()
	slog.Info("Stopped")
}

// shutdownTimeout bounds how long in-flight requests get to finish.
const shutdownTimeout = 25 * time.Second

// healthcheck asks the running server's /api/healthz and returns the exit
// code Docker's HEALTHCHECK expects: 0 healthy, 1 not.
func healthcheck(port string) int {
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/api/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}

// serverHandler is the whole chain every request goes through: compression
// outermost (it must stay outside api.RequestLogger — see
// compressAPIResponses), then the request log, then the security headers and
// the router.
func serverHandler(mux http.Handler, trustedProxies []netip.Prefix) http.Handler {
	return compressAPIResponses(api.RequestLogger(securityHeaders(mux, trustedProxies), trustedProxies))
}

// contentSecurityPolicy is same-origin by design (no CDNs). Ant Design needs
// inline styles; images may be data: (the organization logo) or blob: URIs,
// and browser errors go to Sentry. The in-browser PDF stack that once needed
// 'wasm-unsafe-eval', data:/blob: fetches and blob: workers was removed on
// 2026-09-08 (every PDF is rendered server-side now), so the policy no
// longer allows them (audit F180).
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"font-src 'self' data:; connect-src 'self' *.sentry.io; " +
	"worker-src 'self'; frame-ancestors 'none'; base-uri 'self'; " +
	"form-action 'self'; object-src 'none'"

// securityHeaders sets baseline defensive headers on every response, API and
// embedded frontend alike.
func securityHeaders(next http.Handler, trustedProxies []netip.Prefix) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		// HSTS only on requests that actually arrived over HTTPS. TLS is
		// terminated upstream (reverse proxy) in every real deployment, so
		// we key off X-Forwarded-Proto — but F64 (2026-08-13 audit): only
		// when it's set by a TRUSTED_PROXIES peer (api.IsHTTPS), the same
		// trust boundary already drawn for X-Forwarded-For. Before this
		// fix, any peer reaching the app directly could set the header
		// itself and force HSTS on a plain-HTTP deployment, locking
		// clients out. 2 years + includeSubDomains, no preload (that's a
		// standing commitment the operator should opt into deliberately,
		// not a default).
		if api.IsHTTPS(r, trustedProxies) {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
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
				slog.Warn("ignoring invalid TRUSTED_PROXIES entry", "entry", field, "err", err)
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
			slog.Warn("ignoring invalid TRUSTED_PROXIES entry", "entry", field, "err", err)
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

// backupDirMode keeps the backups directory closed to "other" (F15) but
// traversable by the group, for the same ACL-mask reason as
// db.BackupFileMode — a 0700 directory would lock a backup user out of
// every snapshot inside it.
const backupDirMode os.FileMode = 0750

// ensureBackupDir creates the backups directory, and adds backupDirMode's
// bits to an existing one: os.MkdirAll leaves an existing directory's mode
// alone, so an install upgraded from before #512 kept a 0700 directory the
// group couldn't enter (audit F179). Bits are only added, never removed — a
// directory an operator opened further (production's is 0775) keeps that. A
// directory owned by someone else (a host mount) can't be changed and is
// left as it is.
func ensureBackupDir(dir string) error {
	if err := os.MkdirAll(dir, backupDirMode); err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if want := info.Mode().Perm() | backupDirMode; want != info.Mode().Perm() {
		if err := os.Chmod(dir, want); err != nil && !errors.Is(err, fs.ErrPermission) {
			return err
		}
	}
	return nil
}

func backupDirPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "backups")
}

// setupLogging makes slog's default logger the server's single log output:
// LOG_LEVEL is debug, info (default), warn or error; LOG_FORMAT is text
// (default) or json. Anything still written through the standard log package
// goes through the same handler at info level.
func setupLogging(level, format string) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(strings.TrimSpace(level))); err != nil || level == "" {
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler = slog.NewTextHandler(os.Stderr, opts)
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
	if level != "" && !strings.EqualFold(strings.TrimSpace(level), lvl.String()) {
		slog.Warn("ignoring invalid LOG_LEVEL, using info", "value", level)
	}
}

// fatal logs an error and exits, the slog counterpart of log.Fatal.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}
