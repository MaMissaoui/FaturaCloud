// Command seed-demo populates a FaturaCloud "demo" organization with 18
// months (configurable) of realistic, daily business activity — invoices,
// purchase orders, goods receipts, vendor bills, sales orders, deliveries,
// payments — by driving the real REST API exactly like a browser would (a
// logged-in session, the CSRF header, real server-side validation and GL
// posting). The intent is a demo dataset a person can click through to
// validate a workflow or a new feature against something that looks like a
// real, aged business — not a handful of hand-crafted fixture rows.
//
// Usage (against a running server, e.g. `go run . &` or the Docker image):
//
//	go run ./cmd/seed-demo \
//	  --base-url http://localhost:8080 \
//	  --admin-email admin@fatura.cloud --admin-password admin \
//	  --org-name "Demo Organization" --months 18 --volume busy --seed 20260909
//
// See cmd/seed-demo/README.md for the full flag reference, what gets
// created, expected runtime, and how to extend this with a new generator.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

func main() {
	cfg := parseFlags()

	c, err := NewClient(cfg.BaseURL)
	if err != nil {
		log.Fatalf("seed-demo: %v", err)
	}
	if err := c.Login(cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Fatalf("seed-demo: login as %s: %v\n(is the server running at %s? are --admin-email/--admin-password correct?)",
			cfg.AdminEmail, err, cfg.BaseURL)
	}

	s := NewSeeder(c, cfg)
	if err := s.Run(); err != nil {
		log.Fatalf("seed-demo: %v", err)
	}
}

// Config is every knob seed-demo exposes. Keeping it one flat struct (rather
// than threading a dozen individual flag values through every function)
// means a new generator can read whatever it needs from cfg without a
// signature change elsewhere.
type Config struct {
	BaseURL       string
	AdminEmail    string
	AdminPassword string
	OrgName       string
	Months        int
	EndDate       time.Time
	Seed          uint64
	Volume        string
	Reset         bool
	DryRun        bool
	ProgressEvery int
}

func parseFlags() Config {
	var cfg Config
	var endDateStr, volumeStr string

	flag.StringVar(&cfg.BaseURL, "base-url", envOr("SEED_BASE_URL", "http://localhost:8080"), "FaturaCloud server base URL")
	flag.StringVar(&cfg.AdminEmail, "admin-email", envOr("SEED_ADMIN_EMAIL", envOr("ADMIN_EMAIL", "admin@fatura.cloud")), "platform admin login email (creating the org grants it admin membership)")
	flag.StringVar(&cfg.AdminPassword, "admin-password", envOr("SEED_ADMIN_PASSWORD", envOr("ADMIN_PASSWORD", "admin")), "platform admin login password")
	flag.StringVar(&cfg.OrgName, "org-name", "Demo Organization", "name of the organization to create/reset and seed")
	flag.IntVar(&cfg.Months, "months", 18, "how many months of daily history to generate, ending at --end-date")
	flag.StringVar(&endDateStr, "end-date", time.Now().Format("2006-01-02"), "last day of the simulated range, YYYY-MM-DD (default: today)")
	flag.Uint64Var(&cfg.Seed, "seed", 20260101, "RNG seed — the same seed always reproduces the same dataset")
	flag.StringVar(&volumeStr, "volume", "busy", `data volume profile: "small" or "busy"`)
	flag.BoolVar(&cfg.Reset, "reset", false, "if an organization named --org-name already exists, delete it first and recreate from scratch")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "log the plan (day-by-day activity counts) without calling the API at all")
	flag.IntVar(&cfg.ProgressEvery, "progress-every", 20, "log a progress line every N simulated days (0 disables)")
	flag.Parse()

	end, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed-demo: --end-date must be YYYY-MM-DD: %v\n", err)
		os.Exit(2)
	}
	cfg.EndDate = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

	if _, ok := volumeProfiles[volumeStr]; !ok {
		fmt.Fprintf(os.Stderr, "seed-demo: --volume must be one of: small, busy\n")
		os.Exit(2)
	}
	cfg.Volume = volumeStr

	if cfg.Months < 1 {
		fmt.Fprintf(os.Stderr, "seed-demo: --months must be >= 1\n")
		os.Exit(2)
	}

	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
