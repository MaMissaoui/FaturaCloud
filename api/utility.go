package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
	_ "modernc.org/sqlite"
)

// ── Types ────────────────────────────────────────────────────────────────────

type BackupEntry struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupConfig struct {
	Enabled       bool `json:"enabled"`
	ScheduleHour  int  `json:"scheduleHour"`
	RetentionDays int  `json:"retentionDays"`
}

// ── Version ──────────────────────────────────────────────────────────────────

func (h *handler) getVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": h.version})
}

// ── Backup config ─────────────────────────────────────────────────────────────

func (h *handler) getBackupConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.readBackupConfig())
}

func (h *handler) setBackupConfig(w http.ResponseWriter, r *http.Request) {
	var cfg BackupConfig
	if err := decodeJSON(w, r, &cfg); err != nil {
		return
	}
	if cfg.RetentionDays < 1 {
		cfg.RetentionDays = 1
	}
	if cfg.ScheduleHour < 0 || cfg.ScheduleHour > 23 {
		cfg.ScheduleHour = 0
	}
	if err := h.writeBackupConfig(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *handler) readBackupConfig() BackupConfig {
	cfg := BackupConfig{Enabled: false, ScheduleHour: 2, RetentionDays: 30}
	data, err := os.ReadFile(filepath.Join(h.backupDir, "config.json"))
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func (h *handler) writeBackupConfig(cfg BackupConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(h.backupDir, "config.json"), data, 0600)
}

// ── Backup list ───────────────────────────────────────────────────────────────

func (h *handler) listBackups(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.backupDir)
	if err != nil {
		writeJSON(w, http.StatusOK, []BackupEntry{})
		return
	}
	var result []BackupEntry
	for _, e := range entries {
		if e.IsDir() || e.Name() == "config.json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, BackupEntry{
			Name:      e.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if result == nil {
		result = []BackupEntry{}
	}
	writeJSON(w, http.StatusOK, result)
}

// ── Trigger / download ────────────────────────────────────────────────────────

func (h *handler) triggerBackup(w http.ResponseWriter, r *http.Request) {
	filename := fmt.Sprintf("fatura-backup-%s.db", time.Now().Format("2006-01-02T15-04-05"))
	dst := filepath.Join(h.backupDir, filename)

	if err := h.db.Backup(dst); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("backup failed: %v", err))
		return
	}

	f, err := os.Open(dst)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read backup file")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeContent(w, r, filename, time.Now(), f)
}

// ── Restore from stored backup ─────────────────────────────────────────────────

func (h *handler) restoreNamedBackup(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	if name == "" || name == "." || name == "config.json" {
		writeError(w, http.StatusBadRequest, "invalid backup name")
		return
	}
	src := filepath.Join(h.backupDir, name)
	if _, err := os.Stat(src); err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	h.swapDatabase(w, src)
}

// ── Upload restore ────────────────────────────────────────────────────────────

func (h *handler) restoreDatabase(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("database")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing database file")
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "fatura-restore-*.db")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp file")
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save upload")
		return
	}
	tmp.Close()

	h.swapDatabase(w, tmp.Name())
}

// ── Shared DB-swap logic ──────────────────────────────────────────────────────

// validateRestoreCandidate opens srcPath as a plain SQLite file (no pragmas,
// no migrations — it must not mutate a stored backup or a not-yet-committed
// upload) and runs a quick integrity check, so a garbage upload is rejected
// before the live database is ever touched.
func validateRestoreCandidate(srcPath string) error {
	conn, err := sql.Open("sqlite", srcPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer conn.Close()

	var result string
	if err := conn.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("not a sqlite database: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}

	var count int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'users'`,
	).Scan(&count); err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("missing users table — doesn't look like a FaturaCloud database")
	}
	return nil
}

func (h *handler) swapDatabase(w http.ResponseWriter, srcPath string) {
	if err := validateRestoreCandidate(srcPath); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("not a valid FaturaCloud database: %v", err))
		return
	}

	safetyPath := h.dbPath + ".safety"

	h.dbMu.Lock()
	defer h.dbMu.Unlock()

	if _, statErr := os.Stat(h.dbPath); statErr == nil {
		if err := h.db.Backup(safetyPath); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("safety backup: %v", err))
			return
		}
	}

	if err := h.db.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("close db: %v", err))
		return
	}
	h.db = nil

	_ = os.Remove(h.dbPath + "-wal")
	_ = os.Remove(h.dbPath + "-shm")

	if err := copyFile(srcPath, h.dbPath); err != nil {
		if !h.recoverFromSafety(safetyPath) {
			log.Fatalf("restore failed (%v) and rollback to the pre-restore backup also failed — refusing to keep running with no usable database", err)
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("restore copy failed, rolled back to the pre-restore database: %v", err))
		return
	}

	database, err := db.NewDatabase(h.dbPath)
	if err != nil {
		if !h.recoverFromSafety(safetyPath) {
			log.Fatalf("restored database failed to open (%v) and rollback to the pre-restore backup also failed — refusing to keep running with no usable database", err)
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("restored database failed to open, rolled back to the pre-restore database: %v", err))
		return
	}
	h.db = database
	_ = os.Remove(safetyPath)

	writeJSON(w, http.StatusOK, map[string]string{"message": "Database restored successfully"})
}

// recoverFromSafety restores the pre-restore safety backup over h.dbPath and
// reopens it, used when a restore attempt fails partway through with the live
// database already closed. Reports whether recovery succeeded — h.db is left
// nil only when it didn't (no safety backup existed, e.g. on a fresh install
// with no prior database, or the rollback copy/reopen itself failed).
func (h *handler) recoverFromSafety(safetyPath string) bool {
	if _, err := os.Stat(safetyPath); err != nil {
		return false
	}
	if err := copyFile(safetyPath, h.dbPath); err != nil {
		return false
	}
	database, err := db.NewDatabase(h.dbPath)
	if err != nil {
		return false
	}
	h.db = database
	_ = os.Remove(safetyPath)
	return true
}

// ── Scheduler ─────────────────────────────────────────────────────────────────

func (h *handler) runScheduler() {
	for {
		time.Sleep(1 * time.Minute)

		cfg := h.readBackupConfig()
		if !cfg.Enabled {
			continue
		}

		now := time.Now().UTC()
		if now.Hour() != cfg.ScheduleHour {
			continue
		}

		todayName := fmt.Sprintf("fatura-auto-%s.db", now.Format("2006-01-02"))
		if _, err := os.Stat(filepath.Join(h.backupDir, todayName)); err == nil {
			continue
		}

		h.dbMu.RLock()
		if h.db == nil {
			h.dbMu.RUnlock()
			continue
		}
		err := h.db.Backup(filepath.Join(h.backupDir, todayName))
		h.dbMu.RUnlock()
		if err != nil {
			continue
		}

		h.applyRetention(cfg.RetentionDays)
	}
}

func (h *handler) applyRetention(days int) {
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	entries, err := os.ReadDir(h.backupDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "config.json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(h.backupDir, e.Name()))
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
