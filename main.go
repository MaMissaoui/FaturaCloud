package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

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

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = defaultJWTSecret
	}
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		adminEmail = defaultAdminEmail
	}
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = defaultAdminPassword
	}

	mux := api.NewRouter(database, dbPath, backupDir, jwtSecret, version)
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
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
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
